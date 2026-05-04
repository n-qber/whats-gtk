package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

type Bubble interface {
	Widget() gtk.IWidget
	UpdateAvatar(pixbuf *gdk.Pixbuf)
	UpdateImage(pixbuf *gdk.Pixbuf)
	SetStatus(status string)
	SetReactions(reactions []string)
	SetQuotedMessage(id, sender, content string)
	IsSelf() bool
	Sender() string
	Content() string
	SetOnQuotedClick(f func(id string))
}

type baseBubble struct {
	Box            *gtk.Box
	BubbleBox      *gtk.Box
	QuotedBox      *gtk.Box
	QuotedEventBox *gtk.EventBox
	StatusLabel    *gtk.Label
	AvatarImg      *gtk.Image
	ReactionsBox   *gtk.Box
	isSelf         bool
	sender         string
	content        string
	quotedID       string
	onQuotedClick  func(id string)
}

func (b *baseBubble) Sender() string  { return b.sender }
func (b *baseBubble) Content() string { return b.content }
func (b *baseBubble) SetOnQuotedClick(f func(id string)) { b.onQuotedClick = f }

func newBaseBubble(name string, contentText string, content gtk.IWidget, isSelf bool, hasBubble bool, status string, time string, avatar *gdk.Pixbuf) (*baseBubble, error) {
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

	quotedEventBox, _ := gtk.EventBoxNew()
	quotedBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	quotedEventBox.Add(quotedBox)
	quotedEventBox.Hide()
	bubbleBox.PackStart(quotedEventBox, false, false, 0)

	// Use Overlay for content and status
	overlay, _ := gtk.OverlayNew()
	
	contentBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	contentBox.PackStart(content, false, false, 0)
	overlay.Add(contentBox)

	statusBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	sbc, _ := statusBox.GetStyleContext()
	sbc.AddClass("status-overlay")
	statusBox.SetHAlign(gtk.ALIGN_END)
	statusBox.SetVAlign(gtk.ALIGN_END)
	
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
	}

	overlay.AddOverlay(statusBox)
	bubbleBox.PackStart(overlay, false, false, 0)

	reactionsBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	reactionsBox.SetNoShowAll(true)
	reactionsBox.SetHAlign(gtk.ALIGN_END)
	rCtx, _ := reactionsBox.GetStyleContext()
	rCtx.AddClass("reactions-container")
	reactionsBox.Hide()
	bubbleBox.PackStart(reactionsBox, false, false, 0)

	if isSelf {
		if hasBubble {
			bCtx.AddClass("bubble-self")
		}
		alignmentBox.PackEnd(bubbleBox, false, false, 0)
	} else {
		if hasBubble {
			bCtx.AddClass("bubble-other")
		}
		alignmentBox.PackStart(bubbleBox, false, false, 0)
	}
	
	alignmentBox.ShowAll()
	quotedEventBox.Hide()

	bb := &baseBubble{
		Box:            alignmentBox,
		BubbleBox:      bubbleBox,
		QuotedBox:      quotedBox,
		QuotedEventBox: quotedEventBox,
		StatusLabel:    statusLabel,
		AvatarImg:      avatarImg,
		ReactionsBox:   reactionsBox,
		isSelf:         isSelf,
		sender:         name,
		content:        contentText,
	}

	quotedEventBox.Connect("button-press-event", func(eb *gtk.EventBox, event *gdk.Event) bool {
		if bb.onQuotedClick != nil && bb.quotedID != "" {
			bb.onQuotedClick(bb.quotedID)
			return true
		}
		return false
	})

	return bb, nil
}

func (b *baseBubble) SetQuotedMessage(id, sender, content string) {
	b.quotedID = id
	if id == "" {
		glib.IdleAdd(func() {
			qCtx, _ := b.QuotedBox.GetStyleContext()
			qCtx.RemoveClass("quoted-message")
			b.QuotedEventBox.Hide()
		})
		return
	}

	glib.IdleAdd(func() {
		children := b.QuotedBox.GetChildren()
		children.Foreach(func(item interface{}) {
			b.QuotedBox.Remove(item.(gtk.IWidget))
		})

		qCtx, _ := b.QuotedBox.GetStyleContext()
		qCtx.AddClass("quoted-message")

		senderLabel, _ := gtk.LabelNew("")
		senderLabel.SetMarkup("<b>" + sender + "</b>")
		senderLabel.SetXAlign(0)
		sCtx, _ := senderLabel.GetStyleContext()
		sCtx.AddClass("quoted-sender")

		contentLabel, _ := gtk.LabelNew(content)
		contentLabel.SetXAlign(0)
		contentLabel.SetLineWrap(true)
		contentLabel.SetLineWrapMode(pango.WRAP_WORD_CHAR)
		contentLabel.SetMaxWidthChars(40)
		contentLabel.SetEllipsize(pango.ELLIPSIZE_END)
		contentLabel.SetLines(3)

		b.QuotedBox.PackStart(senderLabel, false, false, 0)
		b.QuotedBox.PackStart(contentLabel, false, false, 0)
		b.QuotedBox.ShowAll()
		b.QuotedEventBox.Show()
	})
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
