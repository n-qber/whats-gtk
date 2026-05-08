package bubbles

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type Bubble interface {
	Widget() gtk.Widgetter
	UpdateAvatar(tex *gdk.Texture)
	UpdateImage(tex *gdk.Texture)
	UpdateDocument(path string)
	SetStatus(status string)
	SetReactions(reactions []string)
	SetQuotedMessage(id, sender, content string)
	IsSelf() bool
	Sender() string
	Content() string
	SetOnQuotedClick(f func(id string))
	SetOnReplyRequest(f func())
	SetOnReactionRequest(f func(emoji string))
}

type baseBubble struct {
	Box            *gtk.Box
	BubbleBox      *gtk.Box
	QuotedBox      *gtk.Box
	QuotedEventBox *gtk.GestureClick
	StatusLabel    *gtk.Label
	AvatarImg      *adw.Avatar
	ReactionsBox   *gtk.Box
	ReactionsBtn   *gtk.Button
	isSelf         bool
	sender         string
	content        string
	quotedID       string
	onQuotedClick  func(id string)
	onReplyRequest func()
	onReactionRequest func(emoji string)
}

func (b *baseBubble) Sender() string  { return b.sender }
func (b *baseBubble) Content() string { return b.content }
func (b *baseBubble) SetOnQuotedClick(f func(id string)) { b.onQuotedClick = f }
func (b *baseBubble) SetOnReplyRequest(f func()) { b.onReplyRequest = f }
func (b *baseBubble) SetOnReactionRequest(f func(emoji string)) { b.onReactionRequest = f }
func (b *baseBubble) IsSelf() bool { return b.isSelf }
func (b *baseBubble) Widget() gtk.Widgetter { return b.Box }

func newBaseBubble(name string, contentText string, content gtk.Widgetter, isSelf bool, hasBubble bool, status string, time string, avatar *gdk.Texture) (*baseBubble, error) {
	alignmentBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	
	var avatarImg *adw.Avatar
	if !isSelf {
		cleanName := name
		if idx := strings.Index(name, " <span"); idx != -1 {
			cleanName = name[:idx]
		}
		avatarImg = adw.NewAvatar(36, cleanName, true)
		if avatar != nil {
			avatarImg.SetCustomImage(avatar)
		}
		avatarImg.SetVAlign(gtk.AlignStart)
		if name == "" {
			avatarImg.SetOpacity(0)
		}
		alignmentBox.Append(avatarImg)
	}

	bubbleBox := gtk.NewBox(gtk.OrientationVertical, 2)
	if hasBubble {
		bubbleBox.AddCSSClass("message-bubble")
		if isSelf {
			bubbleBox.AddCSSClass("bubble-self")
			bubbleBox.SetHAlign(gtk.AlignEnd)
		} else {
			bubbleBox.AddCSSClass("bubble-other")
			bubbleBox.SetHAlign(gtk.AlignStart)
		}
	}

	if name != "" && !isSelf {
		nameLabel := gtk.NewLabel("")
		if strings.Contains(name, "<span") {
			nameLabel.SetMarkup(name)
		} else {
			nameLabel.SetText(name)
		}
		nameLabel.AddCSSClass("message-sender-name")
		nameLabel.SetXAlign(0)
		bubbleBox.Append(nameLabel)
	}

	quotedBox := gtk.NewBox(gtk.OrientationVertical, 2)
	quotedBox.SetFocusable(true)
	quotedBox.Hide()
	
	click := gtk.NewGestureClick()
	quotedBox.AddController(click)
	
	bubbleBox.Append(quotedBox)

	statusBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	statusBox.AddCSSClass("status-overlay")
	statusBox.SetHAlign(gtk.AlignEnd)
	statusBox.SetVAlign(gtk.AlignEnd)
	statusBox.SetCanTarget(false)
	
	timeLabel := gtk.NewLabel(time)
	timeLabel.AddCSSClass("message-time")
	statusBox.Append(timeLabel)

	var statusLabel *gtk.Label
	if isSelf {
		statusLabel = gtk.NewLabel(getStatusIcon(status))
		statusLabel.AddCSSClass("receipt")
		applyStatusClass(statusLabel, status)
		statusBox.Append(statusLabel)
	}

	isPhoto := contentText == "[Image]" || contentText == "[Video]"
	
	if isPhoto {
		overlay := gtk.NewOverlay()
		overlay.SetChild(content)
		statusBox.SetMarginEnd(6)
		statusBox.SetMarginBottom(4)
		overlay.AddOverlay(statusBox)
		bubbleBox.Append(overlay)
	} else {
		contentStatusBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
		contentStatusBox.SetVAlign(gtk.AlignEnd)
		if l, ok := content.(*gtk.Label); ok {
			l.SetHExpand(false)
			l.SetXAlign(0)
			l.SetWrap(true)
			l.SetWrapMode(pango.WrapWordChar)
		}
		contentStatusBox.Append(content)
		contentStatusBox.Append(statusBox)
		bubbleBox.Append(contentStatusBox)
	}

	reactionsBox := gtk.NewBox(gtk.OrientationHorizontal, 2)
	reactionsBox.SetHAlign(gtk.AlignEnd)
	reactionsBox.AddCSSClass("reactions-container")
	reactionsBox.Hide()
	reactionsBox.SetCanTarget(false)
	
	finalBox := gtk.NewBox(gtk.OrientationVertical, 0)
	finalBox.SetFocusable(false)
	finalBox.Append(bubbleBox)
	finalBox.Append(reactionsBox)

	doubleClick := gtk.NewGestureClick()
	doubleClick.SetButton(1)
	finalBox.AddController(doubleClick)

	reactionsBtn := gtk.NewButtonFromIconName("face-smile-symbolic")
	reactionsBtn.SetHasFrame(false)
	reactionsBtn.SetOpacity(0) // Start hidden via opacity to keep it clickable
	reactionsBtn.AddCSSClass("message-reactions-btn")
	reactionsBtn.SetCanTarget(true)
	
	hover := gtk.NewEventControllerMotion()
	alignmentBox.AddController(hover)

	var bb *baseBubble

	popover := gtk.NewPopover()
	hbox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	hbox.SetMarginTop(6); hbox.SetMarginBottom(6); hbox.SetMarginStart(6); hbox.SetMarginEnd(6)
	
	emojis := []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}
	for _, e := range emojis {
		btn := gtk.NewButtonWithLabel(e)
		btn.SetHasFrame(false)
		btn.ConnectClicked(func() {
			if bb != nil && bb.onReactionRequest != nil { bb.onReactionRequest(e) }
			popover.Popdown()
		})
		hbox.Append(btn)
	}
	plusBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	plusBtn.SetHasFrame(false)
	plusBtn.ConnectClicked(func() {
		popover.Popdown()
		fullPopover := gtk.NewPopover()
		fullPopover.SetParent(reactionsBtn)
		flowBox := gtk.NewFlowBox()
		flowBox.SetMaxChildrenPerLine(8)
		allEmojis := []string{
			"👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "✨", 
			"👏", "🎉", "✅", "❌", "💯", "🚀", "💡", "👀",
			"🤣", "😍", "😭", "😊", "🥳", "🤔", "🙄", "😱",
		}
		for _, e := range allEmojis {
			btn := gtk.NewButtonWithLabel(e)
			btn.SetHasFrame(false)
			btn.ConnectClicked(func() {
				if bb != nil && bb.onReactionRequest != nil { bb.onReactionRequest(e) }
				fullPopover.Popdown()
			})
			flowBox.Append(btn)
		}
		scrolled := gtk.NewScrolledWindow()
		scrolled.SetSizeRequest(240, 200)
		scrolled.SetChild(flowBox)
		fullPopover.SetChild(scrolled)
		fullPopover.Popup()
	})
	hbox.Append(plusBtn)
	popover.SetChild(hbox)
	popover.SetParent(reactionsBtn)

	bb = &baseBubble{
		Box:          alignmentBox,
		BubbleBox:    bubbleBox,
		QuotedBox:    quotedBox,
		StatusLabel:  statusLabel,
		AvatarImg:    avatarImg,
		ReactionsBox: reactionsBox,
		ReactionsBtn: reactionsBtn,
		isSelf:       isSelf,
		sender:       name,
		content:      contentText,
	}

	hover.ConnectEnter(func(x, y float64) { reactionsBtn.SetOpacity(1) })
	hover.ConnectLeave(func() { if !popover.Visible() { reactionsBtn.SetOpacity(0) } })

	reactionsBtn.ConnectClicked(func() {
		popover.Popup()
	})

	doubleClick.ConnectPressed(func(n int, x, y float64) {
		if n == 2 && bb.onReplyRequest != nil { bb.onReplyRequest() }
	})

	click.ConnectPressed(func(n int, x, y float64) {
		if bb.onQuotedClick != nil && bb.quotedID != "" { bb.onQuotedClick(bb.quotedID) }
	})

	if isSelf {
		alignmentBox.Prepend(reactionsBtn)
		alignmentBox.Append(finalBox)
		alignmentBox.SetHAlign(gtk.AlignEnd)
	} else {
		alignmentBox.Append(finalBox)
		alignmentBox.Append(reactionsBtn)
		alignmentBox.SetHAlign(gtk.AlignStart)
	}

	alignmentBox.Show()
	quotedBox.Hide()
	return bb, nil
}

func (b *baseBubble) SetQuotedMessage(id, sender, content string) {
	b.quotedID = id
	if id == "" {
		glib.IdleAdd(func() {
			b.QuotedBox.RemoveCSSClass("quoted-message")
			b.QuotedBox.Hide()
		})
		return
	}
	glib.IdleAdd(func() {
		// If it's a sticker or similar that usually has no bubble, enable it for quoted messages
		if !b.BubbleBox.HasCSSClass("message-bubble") {
			b.BubbleBox.AddCSSClass("message-bubble")
			if b.isSelf {
				b.BubbleBox.AddCSSClass("bubble-self")
				b.BubbleBox.SetHAlign(gtk.AlignEnd)
			} else {
				b.BubbleBox.AddCSSClass("bubble-other")
				b.BubbleBox.SetHAlign(gtk.AlignStart)
			}
		}

		for {
			child := b.QuotedBox.FirstChild()
			if child == nil { break }
			b.QuotedBox.Remove(child)
		}
		b.QuotedBox.AddCSSClass("quoted-message")
		senderLabel := gtk.NewLabel("")
		markupSender := sender
		if !strings.Contains(sender, "<span") {
			markupSender = glib.MarkupEscapeText(sender)
		}
		senderLabel.SetMarkup("<b>" + markupSender + "</b>")
		senderLabel.SetXAlign(0)
		senderLabel.AddCSSClass("quoted-sender")
		contentLabel := gtk.NewLabel(content)
		contentLabel.SetXAlign(0)
		contentLabel.SetWrap(true)
		contentLabel.SetWrapMode(pango.WrapWordChar)
		contentLabel.SetMaxWidthChars(40)
		contentLabel.SetEllipsize(pango.EllipsizeEnd)
		contentLabel.SetLines(3)
		b.QuotedBox.Append(senderLabel)
		b.QuotedBox.Append(contentLabel)
		b.QuotedBox.Show()
	})
}

func (b *baseBubble) SetReactions(reactions []string) {
	glib.IdleAdd(func() {
		for {
			child := b.ReactionsBox.FirstChild()
			if child == nil { break }
			b.ReactionsBox.Remove(child)
		}
		if len(reactions) == 0 {
			b.ReactionsBox.Hide()
			return
		}
		b.ReactionsBox.Show()
		for _, r := range reactions {
			label := gtk.NewLabel(r)
			label.AddCSSClass("reaction-badge")
			b.ReactionsBox.Append(label)
		}
	})
}

func (b *baseBubble) UpdateAvatar(tex *gdk.Texture) {
	if b.AvatarImg != nil && tex != nil { b.AvatarImg.SetCustomImage(tex) }
}

func (b *baseBubble) UpdateImage(tex *gdk.Texture) {}
func (b *baseBubble) UpdateDocument(path string) {}

func (b *baseBubble) SetStatus(status string) {
	if b.StatusLabel != nil {
		b.StatusLabel.SetText(getStatusIcon(status))
		b.StatusLabel.RemoveCSSClass("receipt-sent")
		b.StatusLabel.RemoveCSSClass("receipt-delivered")
		b.StatusLabel.RemoveCSSClass("receipt-read")
		b.StatusLabel.RemoveCSSClass("receipt-pending")
		applyStatusClass(b.StatusLabel, status)
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "read": return "✓✓"
	case "delivered": return "✓✓"
	case "sent": return "✓"
	case "pending": return "🕒"
	default: return ""
	}
}

func applyStatusClass(l *gtk.Label, status string) {
	switch status {
	case "read": l.AddCSSClass("receipt-read")
	case "delivered": l.AddCSSClass("receipt-delivered")
	case "sent": l.AddCSSClass("receipt-sent")
	case "pending": l.AddCSSClass("receipt-pending")
	}
}
