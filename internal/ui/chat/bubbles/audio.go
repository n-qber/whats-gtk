package bubbles

import (
	"github.com/gotk3/gotk3/gtk"
)

type AudioBubble struct {
	box *gtk.Box
}

func NewAudioBubble(isSelf bool) (*AudioBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 10)
	if err != nil {
		return nil, err
	}

	playButton, _ := gtk.ButtonNewFromIconName("media-playback-start", gtk.ICON_SIZE_BUTTON)
	slider, _ := gtk.ScaleNewWithRange(gtk.ORIENTATION_HORIZONTAL, 0, 100, 1)
	slider.SetSizeRequest(150, -1)

	if isSelf {
		box.PackEnd(playButton, false, false, 5)
		box.PackEnd(slider, true, true, 5)
	} else {
		box.PackStart(playButton, false, false, 5)
		box.PackStart(slider, true, true, 5)
	}

	return &AudioBubble{box: box}, nil
}

func (b *AudioBubble) Widget() gtk.IWidget {
	return b.box
}
