package bubbles

import (
	"github.com/gotk3/gotk3/gtk"
)

type AudioBubble struct {
	*baseBubble
}

func NewAudioBubble(isSelf bool) (*AudioBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 10)
	if err != nil {
		return nil, err
	}

	playButton, _ := gtk.ButtonNewFromIconName("media-playback-start", gtk.ICON_SIZE_BUTTON)
	slider, _ := gtk.ScaleNewWithRange(gtk.ORIENTATION_HORIZONTAL, 0, 100, 1)
	slider.SetSizeRequest(150, -1)

	box.PackStart(playButton, false, false, 5)
	box.PackStart(slider, true, true, 5)

	base, err := newBaseBubble("", box, isSelf, true, "", "", nil)
	if err != nil {
		return nil, err
	}

	return &AudioBubble{baseBubble: base}, nil
}
