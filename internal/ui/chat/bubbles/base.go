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
	UpdateImage(tex *gdk.Texture, path string)
	UpdateDocument(path string)
	SetStatus(status string)
	SetReactions(reactions []string)
	SetPinned(pinned bool)
	SetQuotedMessage(id, sender, content string)
	IsSelf() bool
	Sender() string
	Content() string
	SetOnQuotedClick(f func(id string))
	SetOnReplyRequest(f func())
	SetOnReactionRequest(f func(emoji string))
	SetOnMentionClick(f func(jid string))
	SetContentText(text string)
	SetEdited(edited bool)
	MediaPath() string
	SetViewOnce(viewOnce bool)
	SetForwarded(forwarded bool)
	SetOnBubbleClick(f func())
}

type baseBubble struct {
	Box            *gtk.Box
	BubbleBox      *gtk.Box
	QuotedBox      *gtk.Box
	ForwardedBox   *gtk.Box
	QuotedEventBox *gtk.GestureClick
	StatusLabel    *gtk.Label
	PinIcon        *gtk.Image
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
	onMentionClick    func(jid string)
	onBubbleClick     func()
	contentWidget  gtk.Widgetter
	editedLabel    *gtk.Label
	progressBar    *gtk.ProgressBar
	progressTick   glib.SourceHandle
	viewOnceLabel  *gtk.Label
}

func (b *baseBubble) Sender() string  { return b.sender }
func (b *baseBubble) Content() string { return b.content }
func (b *baseBubble) SetOnQuotedClick(f func(id string)) { b.onQuotedClick = f }
func (b *baseBubble) SetOnReplyRequest(f func()) { b.onReplyRequest = f }
func (b *baseBubble) SetOnReactionRequest(f func(emoji string)) { b.onReactionRequest = f }
func (b *baseBubble) SetOnMentionClick(f func(jid string)) { b.onMentionClick = f }
func (b *baseBubble) SetOnBubbleClick(f func()) { b.onBubbleClick = f }
func (b *baseBubble) IsSelf() bool { return b.isSelf }
func (b *baseBubble) Widget() gtk.Widgetter { return b.Box }

func (b *baseBubble) SetContentText(text string) {
	b.content = text
	if l, ok := b.contentWidget.(*gtk.Label); ok {
		l.SetText(text)
	}
}

func (b *baseBubble) MediaPath() string {
	return ""
}

func (b *baseBubble) SetEdited(edited bool) {
	if b.editedLabel != nil {
		if edited {
			b.editedLabel.Show()
		} else {
			b.editedLabel.Hide()
		}
	}
}

func (b *baseBubble) SetViewOnce(viewOnce bool) {
	glib.IdleAdd(func() {
		if b.viewOnceLabel != nil {
			if viewOnce {
				b.viewOnceLabel.Show()
			} else {
				b.viewOnceLabel.Hide()
			}
		}
	})
}

func (b *baseBubble) SetForwarded(forwarded bool) {
	glib.IdleAdd(func() {
		if b.ForwardedBox != nil {
			if forwarded {
				b.ForwardedBox.Show()
			} else {
				b.ForwardedBox.Hide()
			}
		}
	})
}

func newBaseBubble(name string, contentText string, content gtk.Widgetter, isSelf bool, hasBubble bool, status string, time string, avatar *gdk.Texture) (*baseBubble, error) {
	alignmentBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	var bb *baseBubble

	bClick := gtk.NewGestureClick()
	bClick.SetButton(1)
	bClick.SetPropagationPhase(gtk.PhaseCapture)
	bClick.ConnectPressed(func(n int, x, y float64) {
		if bb != nil && bb.onBubbleClick != nil {
			bb.onBubbleClick()
		}
	})
	alignmentBox.AddController(bClick)
	
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

	forwardedBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	forwardedBox.AddCSSClass("forwarded-indicator")
	forwardedBox.Hide()
	
	forwardedIcon := gtk.NewImageFromIconName("mail-forward-symbolic")
	forwardedIcon.SetPixelSize(12)
	
	forwardedLabel := gtk.NewLabel("Encaminhada")
	forwardedLabel.AddCSSClass("forwarded-label")
	
	forwardedBox.Append(forwardedIcon)
	forwardedBox.Append(forwardedLabel)
	bubbleBox.Append(forwardedBox)

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
	statusBox.SetMarginTop(4)

	editedLabel := gtk.NewLabel("(edited)")
	editedLabel.AddCSSClass("time")
	editedLabel.Hide()
	statusBox.Append(editedLabel)

	timeLabel := gtk.NewLabel(time)
	timeLabel.AddCSSClass("message-time")
	
	viewOnceLabel := gtk.NewLabel("①")
	viewOnceLabel.AddCSSClass("time") // reuse small text styling
	viewOnceLabel.Hide()
	statusBox.Append(viewOnceLabel)
	statusBox.Append(timeLabel)

	var statusLabel *gtk.Label
	if isSelf {
		statusLabel = gtk.NewLabel(getStatusIcon(status))
		statusLabel.AddCSSClass("receipt")
		applyStatusClass(statusLabel, status)
		statusBox.Append(statusLabel)
	}

	isPhoto := contentText == "[Image]" || contentText == "[Video]"
	isMedia := isPhoto || contentText == "[Audio]" || strings.HasPrefix(contentText, "[File:") || contentText == "[Sticker]"
	
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

	var progressBar *gtk.ProgressBar
	if isMedia {
		progressBar = gtk.NewProgressBar()
		progressBar.AddCSSClass("media-progress")
		progressBar.Hide()
		bubbleBox.Append(progressBar)
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
		emojiChooser := gtk.NewEmojiChooser()
		emojiChooser.SetParent(reactionsBtn)
		emojiChooser.ConnectEmojiPicked(func(e string) {
			if bb != nil && bb.onReactionRequest != nil {
				bb.onReactionRequest(e)
			}
			emojiChooser.Popdown()
		})
		emojiChooser.ConnectClosed(func() {
			glib.IdleAdd(func() {
				emojiChooser.Unparent()
			})
		})
		emojiChooser.Popup()
	})
	hbox.Append(plusBtn)
	popover.SetChild(hbox)
	popover.SetParent(reactionsBtn)

	bb = &baseBubble{
		Box:          alignmentBox,
		BubbleBox:    bubbleBox,
		QuotedBox:    quotedBox,
		ForwardedBox: forwardedBox,
		StatusLabel:  statusLabel,
		AvatarImg:    avatarImg,
		ReactionsBox: reactionsBox,
		ReactionsBtn: reactionsBtn,
		isSelf:       isSelf,
		sender:       name,
		content:      contentText,
		contentWidget: content,
		editedLabel:  editedLabel,
		progressBar:  progressBar,
		viewOnceLabel: viewOnceLabel,
	}

	pinIcon := gtk.NewImageFromIconName("pin-symbolic")
	pinIcon.AddCSSClass("pin-icon")
	pinIcon.Hide()
	statusBox.Prepend(pinIcon)
	bb.PinIcon = pinIcon

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

func (b *baseBubble) SetPinned(pinned bool) {
	glib.IdleAdd(func() {
		if pinned {
			b.PinIcon.Show()
		} else {
			b.PinIcon.Hide()
		}
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

func (b *baseBubble) UpdateImage(tex *gdk.Texture, path string) {}
func (b *baseBubble) UpdateDocument(path string) {}

func (b *baseBubble) SetStatus(status string) {
	if b.StatusLabel != nil {
		b.StatusLabel.SetText(getStatusIcon(status))
		b.StatusLabel.RemoveCSSClass("receipt-sent")
		b.StatusLabel.RemoveCSSClass("receipt-delivered")
		b.StatusLabel.RemoveCSSClass("receipt-read")
		b.StatusLabel.RemoveCSSClass("receipt-pending")
		b.StatusLabel.RemoveCSSClass("receipt-failed")
		applyStatusClass(b.StatusLabel, status)
	}

	if b.progressBar != nil {
		if status == "pending" {
			b.progressBar.Show()
			if b.progressTick == 0 {
				b.progressTick = glib.TimeoutAdd(50, func() bool {
					if b.progressBar != nil && b.progressBar.Visible() {
						b.progressBar.Pulse()
						return true
					}
					b.progressTick = 0
					return false
				})
			}
		} else {
			b.progressBar.Hide()
			if b.progressTick != 0 {
				glib.SourceRemove(b.progressTick)
				b.progressTick = 0
			}
		}
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "read": return "✓✓"
	case "delivered": return "✓✓"
	case "sent": return "✓"
	case "pending": return "🕒"
	case "failed": return "⚠️"
	default: return ""
	}
}

func applyStatusClass(l *gtk.Label, status string) {
	switch status {
	case "read": l.AddCSSClass("receipt-read")
	case "delivered": l.AddCSSClass("receipt-delivered")
	case "sent": l.AddCSSClass("receipt-sent")
	case "pending": l.AddCSSClass("receipt-pending")
	case "failed": l.AddCSSClass("receipt-failed")
	}
}
