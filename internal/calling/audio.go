package calling

import (
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/purpshell/meowcaller"
)

const (
	SampleRate   = meowcaller.SampleRate   // 16000 Hz
	FrameSamples = meowcaller.FrameSamples // 960 samples = 60ms
	numChannels  = 1
)

// MicSource captures audio from the default microphone and produces float32 PCM frames.
type MicSource struct {
	ctx    *malgo.AllocatedContext
	dev    *malgo.Device
	frames chan []int16
	muted  atomic.Bool
	closed atomic.Bool
	once   sync.Once
}

// NewMicSource starts capturing from the default microphone.
func NewMicSource() (*MicSource, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = numChannels
	cfg.SampleRate = SampleRate
	cfg.Alsa.NoMMap = 1

	frames := make(chan []int16, 32)
	var acc []int16

	onData := func(_, in []byte, count uint32) {
		for i := 0; i+1 < len(in); i += 2 {
			acc = append(acc, int16(binary.LittleEndian.Uint16(in[i:])))
		}
		for len(acc) >= FrameSamples {
			f := make([]int16, FrameSamples)
			copy(f, acc[:FrameSamples])
			acc = acc[FrameSamples:]
			select {
			case frames <- f:
			default: // drop frame if consumer is behind to maintain real-time
			}
		}
	}

	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: onData})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}

	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}

	return &MicSource{
		ctx:    ctx,
		dev:    dev,
		frames: frames,
	}, nil
}

// ReadFrame returns the next 60ms frame. If muted, returns clean silence.
func (m *MicSource) ReadFrame() ([]float32, error) {
	if m.closed.Load() {
		return nil, nil
	}
	pcm, ok := <-m.frames
	if !ok {
		return nil, nil
	}
	if m.muted.Load() {
		return make([]float32, FrameSamples), nil
	}
	return pcmToFloat(pcm), nil
}

// SetMute toggles mute state.
func (m *MicSource) SetMute(muted bool) {
	m.muted.Store(muted)
}

// IsMuted returns true if muted.
func (m *MicSource) IsMuted() bool {
	return m.muted.Load()
}

// Close stops capture and releases the audio hardware.
func (m *MicSource) Close() error {
	m.once.Do(func() {
		m.closed.Store(true)
		if m.dev != nil {
			_ = m.dev.Stop()
			m.dev.Uninit()
		}
		if m.ctx != nil {
			_ = m.ctx.Uninit()
			m.ctx.Free()
		}
	})
	return nil
}

// SpeakerSink plays received float32 PCM frames to the default speaker.
type SpeakerSink struct {
	ctx    *malgo.AllocatedContext
	dev    *malgo.Device
	in     chan []int16
	done   chan struct{}
	closed atomic.Bool
	once   sync.Once
}

// NewSpeakerSink initializes the playback device.
func NewSpeakerSink() (*SpeakerSink, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = numChannels
	cfg.SampleRate = SampleRate

	in := make(chan []int16, 64)
	var (
		mu  sync.Mutex
		buf []int16
	)
	done := make(chan struct{})

	go func() {
		for f := range in {
			mu.Lock()
			buf = append(buf, f...)
			mu.Unlock()
		}
		close(done)
	}()

	onData := func(out, _ []byte, count uint32) {
		need := int(count)
		mu.Lock()
		n := min(need, len(buf))
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint16(out[i*2:], uint16(buf[i]))
		}
		buf = buf[n:]
		mu.Unlock()
		// Zero-fill underrun
		for i := n * 2; i < need*2; i++ {
			out[i] = 0
		}
	}

	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: onData})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}

	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}

	return &SpeakerSink{
		ctx:  ctx,
		dev:  dev,
		in:   in,
		done: done,
	}, nil
}

// WriteFrame queues a float32 frame for audio playback.
func (s *SpeakerSink) WriteFrame(frame []float32) error {
	if s.closed.Load() {
		return nil
	}
	s.in <- floatToPCM(frame)
	return nil
}

// Close drains and releases playback resources.
func (s *SpeakerSink) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		close(s.in)
		<-s.done
		if s.dev != nil {
			_ = s.dev.Stop()
			s.dev.Uninit()
		}
		if s.ctx != nil {
			_ = s.ctx.Uninit()
			s.ctx.Free()
		}
	})
	return nil
}

// TonePlayer generates synthetic ringtones and ringback sounds directly to a SpeakerSink.
type TonePlayer struct {
	sink   *SpeakerSink
	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

// NewTonePlayer creates a tone player that streams tones to the speaker.
func NewTonePlayer(sink *SpeakerSink) *TonePlayer {
	return &TonePlayer{
		sink:   sink,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// PlayRingback plays standard outgoing ringback tone: 440Hz + 480Hz beep for 1.5s, pause 2s.
func (tp *TonePlayer) PlayRingback() {
	go tp.loop(func(t float64) float32 {
		cycle := math.Mod(t, 3.5)
		if cycle < 1.5 {
			s1 := math.Sin(2 * math.Pi * 440 * t)
			s2 := math.Sin(2 * math.Pi * 480 * t)
			return float32((s1 + s2) * 0.15)
		}
		return 0
	})
}

// PlayRingtone plays incoming melodic chime: alternating 523Hz (C5) and 659Hz (E5).
func (tp *TonePlayer) PlayRingtone() {
	go tp.loop(func(t float64) float32 {
		cycle := math.Mod(t, 2.5)
		if cycle < 0.4 {
			return float32(math.Sin(2*math.Pi*523.25*t) * 0.25)
		} else if cycle >= 0.5 && cycle < 0.9 {
			return float32(math.Sin(2*math.Pi*659.25*t) * 0.25)
		} else if cycle >= 1.0 && cycle < 1.4 {
			return float32(math.Sin(2*math.Pi*783.99*t) * 0.25)
		}
		return 0
	})
}

func (tp *TonePlayer) loop(synth func(t float64) float32) {
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	defer close(tp.doneCh)

	var sampleIndex int64
	frame := make([]float32, FrameSamples)

	for {
		select {
		case <-tp.stopCh:
			return
		case <-ticker.C:
			for i := 0; i < FrameSamples; i++ {
				t := float64(sampleIndex+int64(i)) / float64(SampleRate)
				frame[i] = synth(t)
			}
			sampleIndex += FrameSamples
			if tp.sink != nil {
				_ = tp.sink.WriteFrame(frame)
			}
		}
	}
}

// Stop terminates tone playback.
func (tp *TonePlayer) Stop() {
	tp.once.Do(func() {
		close(tp.stopCh)
		<-tp.doneCh
	})
}

// pcmToFloat converts int16 to [-1, 1] float32.
func pcmToFloat(pcm []int16) []float32 {
	out := make([]float32, len(pcm))
	for i, s := range pcm {
		out[i] = float32(s) / 32768.0
	}
	return out
}

// floatToPCM converts [-1, 1] float32 to clamped int16.
func floatToPCM(f []float32) []int16 {
	out := make([]int16, len(f))
	for i, s := range f {
		v := s * 32768.0
		if v > 32767.0 {
			v = 32767.0
		} else if v < -32768.0 {
			v = -32768.0
		}
		out[i] = int16(v)
	}
	return out
}
