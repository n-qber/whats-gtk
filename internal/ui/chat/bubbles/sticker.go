package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type StickerBubble struct {
	box         *gtk.Box
	statusLabel *gtk.Label
	avatarImg   *gtk.Image
}

func NewStickerBubble(name string, pixbuf *gdk.Pixbuf, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*StickerBubble, error) {
	alignmentBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	if err != nil {
		return nil, err
	}

	var avatarImg *gtk.Image
	if !isSelf {
		avatarImg, _ = gtk.ImageNew()
		aCtx, _ := avatarImg.GetStyleContext()
		aCtx.AddClass("avatar")
		if avatar != nil {
			resized, _ := avatar.ScaleSimple(36, 36, gdk.INTERP_BILINEAR)
			avatarImg.SetFromPixbuf(resized)
		} else {
			avatarImg.SetSizeRequest(36, 36)
		}
		avatarImg.SetVAlign(gtk.ALIGN_START)
		if name == "" {
			avatarImg.SetOpacity(0)
		}
		alignmentBox.PackStart(avatarImg, false, false, 0)
	}

	bubbleBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	
	if name != "" && !isSelf {
		nameLabel, _ := gtk.LabelNew("")
		nameLabel.SetMarkup(name)
		nCtx, _ := nameLabel.GetStyleContext()
		nCtx.AddClass("message-sender-name")
		nameLabel.SetXAlign(0)
		bubbleBox.PackStart(nameLabel, false, false, 0)
	}

	image, _ := gtk.ImageNew()
	if pixbuf != nil {
		resized, _ := pixbuf.ScaleSimple(160, 160, gdk.INTERP_BILINEAR)
		image.SetFromPixbuf(resized)
	}
	bubbleBox.PackStart(image, false, false, 0)

	statusBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	timeLabel, _ := gtk.LabelNew(time)
	tCtx, _ := timeLabel.GetStyleContext()
	tCtx.AddClass("message-time")
	statusBox.PackEnd(timeLabel, false, false, 0)

	var statusLabel *gtk.Label
	if isSelf {
		statusLabel, _ = gtk.LabelNew(getStatusIcon(status))
		sCtx, _ := statusLabel.GetStyleContext()
		sCtx.AddClass("receipt")
		applyStatusClass(sCtx, status)
		statusBox.PackEnd(statusLabel, false, false, 0)
		alignmentBox.PackEnd(bubbleBox, false, false, 0)
	} else {
		alignmentBox.PackStart(bubbleBox, false, false, 0)
	}
	
	bubbleBox.PackEnd(statusBox, false, false, 0)
	return &StickerBubble{box: alignmentBox, statusLabel: statusLabel, avatarImg: avatarImg}, nil
}

func (b *StickerBubble) UpdateAvatar(pixbuf *gdk.Pixbuf) {
	if b.avatarImg != nil && pixbuf != nil {
		resized, _ := pixbuf.ScaleSimple(36, 36, gdk.INTERP_BILINEAR)
		b.avatarImg.SetFromPixbuf(resized)
	}
}

func (b *StickerBubble) SetStatus(status string) {
	if b.statusLabel != nil {
		b.statusLabel.SetText(getStatusIcon(status))
		sCtx, _ := b.statusLabel.GetStyleContext()
		sCtx.RemoveClass("receipt-sent")
		sCtx.RemoveClass("receipt-delivered")
		sCtx.RemoveClass("receipt-read")
		sCtx.RemoveClass("receipt-pending")
		applyStatusClass(sCtx, status)
	}
}

func (b *StickerBubble) Widget() gtk.IWidget { return b.box }
