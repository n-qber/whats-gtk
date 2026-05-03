package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type ImageBubble struct {
	*baseBubble
	image             *gtk.Image
	OnDownloadRequest func()
}

func NewImageBubble(name string, pixbuf *gdk.Pixbuf, thumb *gdk.Pixbuf, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*ImageBubble, error) {
	image, err := gtk.ImageNew()
	if err != nil {
		return nil, err
	}

	displayPixbuf := pixbuf
	isThumbnail := false
	if displayPixbuf == nil && thumb != nil {
		displayPixbuf = thumb
		isThumbnail = true
	}

	if displayPixbuf != nil {
		w := displayPixbuf.GetWidth(); h := displayPixbuf.GetHeight()
		if w > 300 { h = int(float64(h) * 300.0 / float64(w)); w = 300 }
		resized, _ := displayPixbuf.ScaleSimple(w, h, gdk.INTERP_BILINEAR)
		image.SetFromPixbuf(resized)
	} else {
		image.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
		image.SetSizeRequest(200, 150)
		isThumbnail = true
	}

	iCtx, _ := image.GetStyleContext()
	iCtx.AddClass("message-image")
	if isThumbnail {
		iCtx.AddClass("message-image-thumbnail")
	}

	eventBox, _ := gtk.EventBoxNew()
	eventBox.Add(image)

	base, err := newBaseBubble(name, eventBox, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	ib := &ImageBubble{baseBubble: base, image: image}

	eventBox.Connect("button-press-event", func() {
		if ib.OnDownloadRequest != nil {
			ib.OnDownloadRequest()
		}
	})

	return ib, nil
}

func (ib *ImageBubble) UpdateImage(pixbuf *gdk.Pixbuf) {
	if pixbuf != nil {
		w := pixbuf.GetWidth(); h := pixbuf.GetHeight()
		if w > 300 { h = int(float64(h) * 300.0 / float64(w)); w = 300 }
		resized, _ := pixbuf.ScaleSimple(w, h, gdk.INTERP_BILINEAR)
		ib.image.SetFromPixbuf(resized)
		
		iCtx, _ := ib.image.GetStyleContext()
		iCtx.RemoveClass("message-image-thumbnail")
	}
}
