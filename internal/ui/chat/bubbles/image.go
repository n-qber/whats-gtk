package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type ImageBubble struct {
	box *gtk.Box
}

func NewImageBubble(pixbuf *gdk.Pixbuf, isSelf bool) (*ImageBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}

	image, err := gtk.ImageNewFromPixbuf(pixbuf)
	if err != nil {
		return nil, err
	}

	if isSelf {
		box.PackEnd(image, false, false, 10)
	} else {
		box.PackStart(image, false, false, 10)
	}

	return &ImageBubble{box: box}, nil
}

func (b *ImageBubble) Widget() gtk.IWidget {
	return b.box
}
