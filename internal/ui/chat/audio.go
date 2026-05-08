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
	OnStop      func()
	currentPath string
	isTemp      bool
}

func NewAudioPlayer() *AudioPlayer {
	return &AudioPlayer{}
}

func (ap *AudioPlayer) Play(path string, onStop func()) error {
	ap.Stop()

	var streamer beep.StreamSeekCloser
	var format beep.Format
	var f *os.File
	var err error
	var isTempWav bool

	decode := func(p string) error {
		f, err = os.Open(p)
		if err != nil {
			return err
		}

		ext := strings.ToLower(filepath.Ext(p))
		switch ext {
		case ".ogg":
			streamer, format, err = vorbis.Decode(f)
		case ".mp3":
			streamer, format, err = mp3.Decode(f)
		case ".wav":
			streamer, format, err = wav.Decode(f)
		default:
			streamer, format, err = vorbis.Decode(f)
			if err != nil {
				f.Seek(0, 0)
				streamer, format, err = mp3.Decode(f)
			}
			if err != nil {
				f.Seek(0, 0)
				streamer, format, err = wav.Decode(f)
			}
		}
		return err
	}

	err = decode(path)
	if err != nil {
		if f != nil {
			f.Close()
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
					if f != nil {
						f.Close()
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
	ap.currentPath = path
	ap.isTemp = isTempWav

	if !ap.initialized {
		err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
		if err != nil {
			streamer.Close()
			f.Close()
			ap.cleanup()
			return err
		}
		ap.initialized = true
		ap.sampleRate = format.SampleRate
	}

	resampled := beep.Resample(4, format.SampleRate, ap.sampleRate, streamer)
	ap.ctrl = &beep.Ctrl{Streamer: beep.Seq(resampled, beep.Callback(func() {
		streamer.Close()
		f.Close()
		ap.cleanup()
		if ap.OnStop != nil {
			ap.OnStop()
		}
	})), Paused: false}

	speaker.Play(ap.ctrl)

	return nil
}

func (ap *AudioPlayer) Stop() {
	if ap.ctrl != nil {
		speaker.Clear()
		ap.ctrl = nil
		ap.cleanup()
		if ap.OnStop != nil {
			ap.OnStop()
			ap.OnStop = nil
		}
	}
}

func (ap *AudioPlayer) cleanup() {
	if ap.isTemp && ap.currentPath != "" {
		os.Remove(ap.currentPath)
		ap.isTemp = false
		ap.currentPath = ""
	}
}
