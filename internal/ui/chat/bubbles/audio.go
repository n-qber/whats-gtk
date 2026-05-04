package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type AudioBubble struct {
	*baseBubble
	OnDownloadRequest func()
	OnPlayRequest     func()
	OnStopRequest     func()
	audioPath         string
	playButton        *gtk.Button
	isPlaying         bool
}

func NewAudioBubble(name string, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*AudioBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 10)
	if err != nil {
		return nil, err
	}

	playButton, _ := gtk.ButtonNew()
	playButton.SetRelief(gtk.RELIEF_NONE)
	pbCtx, _ := playButton.GetStyleContext()
	pbCtx.AddClass("audio-play-button")
	
	playImg, _ := gtk.ImageNewFromIconName("media-playback-start", gtk.ICON_SIZE_LARGE_TOOLBAR)
	playButton.SetImage(playImg)
	playButton.SetAlwaysShowImage(true)

	slider, _ := gtk.ScaleNewWithRange(gtk.ORIENTATION_HORIZONTAL, 0, 100, 1)
	slider.SetDrawValue(false)
	sCtx, _ := slider.GetStyleContext()
	sCtx.AddClass("audio-slider")
	slider.SetSizeRequest(200, -1)

	box.PackStart(playButton, false, false, 0)
	box.PackStart(slider, true, true, 5)

	base, err := newBaseBubble(name, "[Audio]", box, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	ab := &AudioBubble{baseBubble: base, playButton: playButton}
	
	playButton.Connect("clicked", func() {
		if ab.audioPath == "" {
			if ab.OnDownloadRequest != nil {
				ab.OnDownloadRequest()
			}
		} else {
			if ab.isPlaying {
				if ab.OnStopRequest != nil {
					ab.OnStopRequest()
				}
			} else {
				if ab.OnPlayRequest != nil {
					ab.OnPlayRequest()
				}
			}
		}
	})

	return ab, nil
}

func (ab *AudioBubble) AudioPath() string {
	return ab.audioPath
}

func (ab *AudioBubble) SetAudioPath(path string) {
	ab.audioPath = path
}

func (ab *AudioBubble) SetPlaying(playing bool) {
	ab.isPlaying = playing
	icon := "media-playback-start"
	if playing {
		icon = "media-playback-pause"
	}
	img, _ := gtk.ImageNewFromIconName(icon, gtk.ICON_SIZE_LARGE_TOOLBAR)
	ab.playButton.SetImage(img)
}

