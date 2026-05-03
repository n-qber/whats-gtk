package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type Bubble interface {
	Widget() gtk.IWidget
	UpdateAvatar(pixbuf *gdk.Pixbuf)
	UpdateImage(pixbuf *gdk.Pixbuf)
	SetStatus(status string)
	SetReactions(reactions []string)
	IsSelf() bool
}

type baseBubble struct {
	Box          *gtk.Box
	BubbleBox    *gtk.Box
	StatusLabel  *gtk.Label
	AvatarImg    *gtk.Image
	ReactionsBox *gtk.Box
	isSelf       bool
}

func newBaseBubble(name string, content gtk.IWidget, isSelf bool, hasBubble bool, status string, time string, avatar *gdk.Pixbuf) (*baseBubble, error) {
	alignmentBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	if err != nil {
		return nil, err
	}

	var avatarImg *gtk.Image
	if !isSelf {
		avatarImg, _ = gtk.ImageNew()
		if avatar != nil {
			ApplyCircularAvatar(avatarImg, avatar, 36)
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
	bCtx, _ := bubbleBox.GetStyleContext()
	if hasBubble {
		bCtx.AddClass("message-bubble")
	}

	if name != "" && !isSelf {
		nameLabel, _ := gtk.LabelNew("")
		nameLabel.SetMarkup(name)
		nCtx, _ := nameLabel.GetStyleContext()
		nCtx.AddClass("message-sender-name")
		nameLabel.SetXAlign(0)
		bubbleBox.PackStart(nameLabel, false, false, 0)
	}

	bubbleBox.PackStart(content, false, false, 0)

	reactionsBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	reactionsBox.SetHAlign(gtk.ALIGN_END)
	rCtx, _ := reactionsBox.GetStyleContext()
	rCtx.AddClass("reactions-container")
	reactionsBox.SetNoShowAll(true)
	reactionsBox.Hide()
	bubbleBox.PackStart(reactionsBox, false, false, 0)

	statusBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	timeLabel, _ := gtk.LabelNew(time)
	tCtx, _ := timeLabel.GetStyleContext()
	tCtx.AddClass("message-time")
	statusBox.PackEnd(timeLabel, false, false, 0)

	var statusLabel *gtk.Label
	if isSelf {
		if hasBubble {
			bCtx.AddClass("bubble-self")
		}
		statusLabel, _ = gtk.LabelNew(getStatusIcon(status))
		sCtx, _ := statusLabel.GetStyleContext()
		sCtx.AddClass("receipt")
		applyStatusClass(sCtx, status)
		statusBox.PackEnd(statusLabel, false, false, 0)
		alignmentBox.PackEnd(bubbleBox, false, false, 0)
	} else {
		if hasBubble {
			bCtx.AddClass("bubble-other")
		}
		alignmentBox.PackStart(bubbleBox, false, false, 0)
	}
	
	bubbleBox.PackEnd(statusBox, false, false, 0)
	alignmentBox.ShowAll()

	return &baseBubble{
		Box:          alignmentBox,
		BubbleBox:    bubbleBox,
		StatusLabel:  statusLabel,
		AvatarImg:    avatarImg,
		ReactionsBox: reactionsBox,
		isSelf:       isSelf,
	}, nil
}

func (b *baseBubble) SetReactions(reactions []string) {
	glib.IdleAdd(func() {
		children := b.ReactionsBox.GetChildren()
		children.Foreach(func(item interface{}) {
			b.ReactionsBox.Remove(item.(gtk.IWidget))
		})

		if len(reactions) == 0 {
			b.ReactionsBox.Hide()
			return
		}

		b.ReactionsBox.Show()
		for _, r := range reactions {
			label, _ := gtk.LabelNew(r)
			lCtx, _ := label.GetStyleContext()
			lCtx.AddClass("reaction-badge")
			b.ReactionsBox.PackStart(label, false, false, 0)
		}
		b.ReactionsBox.ShowAll()
	})
}

func (b *baseBubble) UpdateAvatar(pixbuf *gdk.Pixbuf) {
	if b.AvatarImg != nil && pixbuf != nil {
		ApplyCircularAvatar(b.AvatarImg, pixbuf, 36)
	}
}

func (b *baseBubble) UpdateImage(pixbuf *gdk.Pixbuf) {}

func (b *baseBubble) SetStatus(status string) {
	if b.StatusLabel != nil {
		b.StatusLabel.SetText(getStatusIcon(status))
		sCtx, _ := b.StatusLabel.GetStyleContext()
		sCtx.RemoveClass("receipt-sent")
		sCtx.RemoveClass("receipt-delivered")
		sCtx.RemoveClass("receipt-read")
		sCtx.RemoveClass("receipt-pending")
		applyStatusClass(sCtx, status)
	}
}

func (b *baseBubble) Widget() gtk.IWidget { return b.Box }

func (b *baseBubble) IsSelf() bool { return b.isSelf }

func getStatusIcon(status string) string {
	switch status {
	case "read":
		return "✓✓"
	case "delivered":
		return "✓✓"
	case "sent":
		return "✓"
	case "pending":
		return "🕒"
	default:
		return ""
	}
}

func applyStatusClass(sCtx *gtk.StyleContext, status string) {
	switch status {
	case "read":
		sCtx.AddClass("receipt-read")
	case "delivered":
		sCtx.AddClass("receipt-delivered")
	case "sent":
		sCtx.AddClass("receipt-sent")
	case "pending":
		sCtx.AddClass("receipt-pending")
	}
}
