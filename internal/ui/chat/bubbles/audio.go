package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type AudioBubble struct {
	*baseBubble
	playButton        *gtk.Button
	playImg           *gtk.Image
	slider            *gtk.Scale
	audioPath         string
	isPlaying         bool
	isDownloading     bool
	isSeeking         bool
	OnDownloadRequest func()
	OnPlayRequest     func()
	OnStopRequest     func()
	OnSeekRequest     func(percent float64)
}

func NewAudioBubble(name string, isSelf bool, status, time string, avatar *gdk.Texture) (*AudioBubble, error) {
	box := gtk.NewBox(gtk.OrientationHorizontal, 10)
	
	playButton := gtk.NewButton()
	playButton.SetHasFrame(false)
	playButton.AddCSSClass("audio-play-button")
	
	playImg := gtk.NewImageFromIconName("folder-download-symbolic")
	playImg.SetPixelSize(32)
	playButton.SetChild(playImg)

	slider := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
	slider.SetDrawValue(false)
	slider.AddCSSClass("audio-slider")
	slider.SetHExpand(true)
	slider.SetSizeRequest(200, -1)

	box.Append(playButton)
	box.Append(slider)

	base, err := newBaseBubble(name, "[Audio]", box, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	ab := &AudioBubble{
		baseBubble: base,
		playButton: playButton,
		playImg:    playImg,
		slider:     slider,
	}

	playButton.ConnectClicked(func() {
		if ab.audioPath == "" {
			if !ab.isDownloading {
				ab.isDownloading = true
				ab.playImg.SetFromIconName("process-working-symbolic")
				if ab.OnDownloadRequest != nil {
					ab.OnDownloadRequest()
				}
			}
			return
		}
		
		if ab.isPlaying {
			if ab.OnStopRequest != nil {
				ab.OnStopRequest()
			}
		} else {
			if ab.OnPlayRequest != nil {
				ab.OnPlayRequest()
			}
		}
	})

	// Detect when user starts/ends dragging to avoid feedback loops
	press := gtk.NewGestureClick()
	press.ConnectPressed(func(n int, x, y float64) {
		ab.isSeeking = true
	})
	slider.AddController(press)

	slider.ConnectValueChanged(func() {
		if ab.isSeeking && ab.OnSeekRequest != nil {
			ab.OnSeekRequest(ab.slider.Value())
		}
	})

	// Use another gesture or signal to detect when dragging ends
	// In GTK4, we can use release or just check isSeeking
	release := gtk.NewGestureClick()
	release.ConnectReleased(func(n int, x, y float64) {
		ab.isSeeking = false
	})
	slider.AddController(release)

	return ab, nil
}

func (ab *AudioBubble) AudioPath() string { return ab.audioPath }

func (ab *AudioBubble) SetAudioPath(path string) {
	ab.audioPath = path
	ab.isDownloading = false
	if path != "" {
		ab.playImg.SetFromIconName("media-playback-start")
	} else {
		ab.playImg.SetFromIconName("folder-download-symbolic")
	}
}

func (ab *AudioBubble) SetPlaying(playing bool) {
	ab.isPlaying = playing
	icon := "media-playback-start"
	if playing {
		icon = "media-playback-pause"
	}
	ab.playImg.SetFromIconName(icon)
}

func (ab *AudioBubble) SetProgress(progress float64) {
	if !ab.isSeeking {
		ab.slider.SetValue(progress)
	}
}
