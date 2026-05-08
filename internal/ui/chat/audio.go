package chat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

type AudioPlayer struct {
	initialized bool
	sampleRate  beep.SampleRate
	ctrl        *beep.Ctrl
	streamer    beep.StreamSeekCloser
	format      beep.Format
	f           *os.File
	OnStop      func()
	OnProgress  func(current, total time.Duration)
	currentPath string
	isTemp      bool
	done        chan bool
}

func NewAudioPlayer() *AudioPlayer {
	return &AudioPlayer{
		done: make(chan bool),
	}
}

func (ap *AudioPlayer) Play(path string, onStop func(), onProgress func(c, t time.Duration)) error {
	ap.Stop()

	var isTempWav bool

	decode := func(p string) error {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		ap.f = f

		ext := strings.ToLower(filepath.Ext(p))
		switch ext {
		case ".ogg":
			ap.streamer, ap.format, err = vorbis.Decode(f)
		case ".mp3":
			ap.streamer, ap.format, err = mp3.Decode(f)
		case ".wav":
			ap.streamer, ap.format, err = wav.Decode(f)
		default:
			ap.streamer, ap.format, err = vorbis.Decode(f)
			if err != nil {
				f.Seek(0, 0)
				ap.streamer, ap.format, err = mp3.Decode(f)
			}
			if err != nil {
				f.Seek(0, 0)
				ap.streamer, ap.format, err = wav.Decode(f)
			}
		}
		return err
	}

	err := decode(path)
	if err != nil {
		if ap.f != nil {
			ap.f.Close()
			ap.f = nil
		}

		// Fallback to ffmpeg conversion
		if _, lookErr := exec.LookPath("ffmpeg"); lookErr == nil {
			tempWav := path + ".tmp.wav"
			cmd := exec.Command("ffmpeg", "-y", "-i", path, "-acodec", "pcm_s16le", "-ar", "44100", tempWav)
			if cmd.Run() == nil {
				err = decode(tempWav)
				if err == nil {
					isTempWav = true
					path = tempWav
				} else {
					if ap.f != nil {
						ap.f.Close()
						ap.f = nil
					}
					os.Remove(tempWav)
				}
			}
		} else {
			return fmt.Errorf("ffmpeg not found, cannot decode this audio format: %v", err)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to decode audio: %v", err)
	}

	ap.OnStop = onStop
	ap.OnProgress = onProgress
	ap.currentPath = path
	ap.isTemp = isTempWav

	if !ap.initialized {
		err = speaker.Init(ap.format.SampleRate, ap.format.SampleRate.N(time.Second/10))
		if err != nil {
			ap.streamer.Close()
			ap.f.Close()
			ap.f = nil
			ap.cleanup()
			return err
		}
		ap.initialized = true
		ap.sampleRate = ap.format.SampleRate
	}

	totalDuration := ap.format.SampleRate.D(ap.streamer.Len())

	resampled := beep.Resample(4, ap.format.SampleRate, ap.sampleRate, ap.streamer)
	ap.ctrl = &beep.Ctrl{Streamer: beep.Seq(resampled, beep.Callback(func() {
		ap.done <- true
		ap.Stop()
	})), Paused: false}

	speaker.Play(ap.ctrl)

	// Start progress reporter
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if ap.ctrl == nil || ap.ctrl.Paused {
					continue
				}
				if ap.OnProgress != nil {
					current := ap.format.SampleRate.D(ap.streamer.Position())
					ap.OnProgress(current, totalDuration)
				}
			case <-ap.done:
				return
			}
		}
	}()

	return nil
}

func (ap *AudioPlayer) Seek(percent float64) {
	if ap.streamer == nil {
		return
	}
	speaker.Lock()
	newPos := int(float64(ap.streamer.Len()) * percent / 100.0)
	if newPos < 0 {
		newPos = 0
	}
	if newPos >= ap.streamer.Len() {
		newPos = ap.streamer.Len() - 1
	}
	ap.streamer.Seek(newPos)
	speaker.Unlock()
}

func (ap *AudioPlayer) Stop() {
	if ap.ctrl != nil {
		select {
		case ap.done <- true:
		default:
		}
		speaker.Clear()
		ap.ctrl = nil
	}
	if ap.streamer != nil {
		ap.streamer.Close()
		ap.streamer = nil
	}
	if ap.f != nil {
		ap.f.Close()
		ap.f = nil
	}
	ap.cleanup()
	if ap.OnStop != nil {
		tmpStop := ap.OnStop
		ap.OnStop = nil
		tmpStop()
	}
}

func (ap *AudioPlayer) cleanup() {
	if ap.isTemp && ap.currentPath != "" {
		os.Remove(ap.currentPath)
		ap.isTemp = false
		ap.currentPath = ""
	}
}
