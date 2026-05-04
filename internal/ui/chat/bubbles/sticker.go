package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type StickerBubble struct {
	*baseBubble
	image             *gtk.Image
	OnDownloadRequest func()
}

func NewStickerBubble(name string, pixbuf *gdk.Pixbuf, thumb *gdk.Pixbuf, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*StickerBubble, error) {
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
		resized, _ := displayPixbuf.ScaleSimple(160, 160, gdk.INTERP_BILINEAR)
		image.SetFromPixbuf(resized)
	} else {
		image.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
		image.SetSizeRequest(160, 160)
		isThumbnail = true
	}

	if isThumbnail {
		iCtx, _ := image.GetStyleContext()
		iCtx.AddClass("message-sticker-thumbnail")
	}

	eventBox, _ := gtk.EventBoxNew()
	eventBox.Add(image)

	base, err := newBaseBubble(name, "[Sticker]", eventBox, isSelf, false, status, time, avatar)

	if err != nil {
		return nil, err
	}

	sb := &StickerBubble{baseBubble: base, image: image}

	eventBox.Connect("button-press-event", func() {
		if sb.OnDownloadRequest != nil {
			sb.OnDownloadRequest()
		}
	})

	return sb, nil
}

func (sb *StickerBubble) UpdateImage(pixbuf *gdk.Pixbuf) {
	if pixbuf != nil {
		resized, _ := pixbuf.ScaleSimple(160, 160, gdk.INTERP_BILINEAR)
		sb.image.SetFromPixbuf(resized)
		
		iCtx, _ := sb.image.GetStyleContext()
		iCtx.RemoveClass("message-sticker-thumbnail")
	}
}
