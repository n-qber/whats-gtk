package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type StickerBubble struct {
	box *gtk.Box
}

func NewStickerBubble(pixbuf *gdk.Pixbuf, isSelf bool) (*StickerBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}

	// Stickers are usually smaller, let's scale if needed
	image, _ := gtk.ImageNewFromPixbuf(pixbuf)

	if isSelf {
		box.PackEnd(image, false, false, 10)
	} else {
		box.PackStart(image, false, false, 10)
	}

	return &StickerBubble{box: box}, nil
}

func (b *StickerBubble) Widget() gtk.IWidget {
	return b.box
}
