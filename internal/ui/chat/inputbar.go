package chat

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type InputBar struct {
	Box               *gtk.Box
	TextView          *gtk.TextView
	ReplyPreviewBox   *gtk.Box
	ReplyPreviewLabel *gtk.Label

	ReplyToID      string
	ReplyToSender  string
	ReplyToContent string

	OnSendMessage             func(text string, replyToID string)
	OnSendFile                func(path string)
	OnPasteImage              func(tex *gdk.Texture)
	OnSelectMessageUp         func() bool
	OnSelectMessageDown       func() bool
	OnSelectMessageBlockUp    func() bool
	OnSelectMessageBlockDown  func() bool
	OnConfirmMessageSelection func() bool
	OnCancelMessageSelection  func() bool

	ctx context.Context
}

func NewInputBar(ctx context.Context) *InputBar {
	ib := &InputBar{
		ctx: ctx,
	}

	ib.Box = gtk.NewBox(gtk.OrientationVertical, 0)

	// Reply Preview
	ib.ReplyPreviewBox = gtk.NewBox(gtk.OrientationHorizontal, 5)
	ib.ReplyPreviewBox.AddCSSClass("reply-preview")
	ib.ReplyPreviewLabel = gtk.NewLabel("")
	ib.ReplyPreviewLabel.SetXAlign(0)
	ib.ReplyPreviewLabel.SetEllipsize(pango.EllipsizeEnd)
	closeReplyBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	ib.ReplyPreviewBox.Append(ib.ReplyPreviewLabel)
	ib.ReplyPreviewBox.Append(closeReplyBtn)
	ib.ReplyPreviewBox.Hide()
	ib.Box.Append(ib.ReplyPreviewBox)

	// Input Area
	inputBox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	inputBox.SetMarginTop(6)
	inputBox.SetMarginBottom(6)
	inputBox.SetMarginStart(6)
	inputBox.SetMarginEnd(6)

	inputScrolled := gtk.NewScrolledWindow()
	inputScrolled.SetMinContentHeight(36)
	inputScrolled.SetMaxContentHeight(200)
	inputScrolled.SetPropagateNaturalHeight(true)
	inputScrolled.SetHExpand(true)
	inputScrolled.AddCSSClass("message-input-scrolled")

	ib.TextView = gtk.NewTextView()
	ib.TextView.SetWrapMode(gtk.WrapWordChar)
	ib.TextView.SetAcceptsTab(false)
	ib.TextView.AddCSSClass("message-input-view")
	inputScrolled.SetChild(ib.TextView)

	fileButton := gtk.NewButtonFromIconName("mail-attachment-symbolic")
	fileButton.SetVAlign(gtk.AlignEnd)

	sendButton := gtk.NewButtonWithLabel("Send")
	sendButton.SetVAlign(gtk.AlignEnd)

	inputBox.Append(fileButton)
	inputBox.Append(inputScrolled)
	inputBox.Append(sendButton)
	ib.Box.Append(inputBox)

	closeReplyBtn.ConnectClicked(func() {
		ib.CancelReply()
	})

	sendMsg := func() {
		buffer := ib.TextView.Buffer()
		start, end := buffer.Bounds()
		text := buffer.Text(start, end, false)
		if text != "" && ib.OnSendMessage != nil {
			ib.OnSendMessage(text, ib.ReplyToID)
			ib.CancelReply()
			buffer.SetText("")
		}
	}

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.SetPropagationPhase(gtk.PhaseCapture)
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		name := gdk.KeyvalName(keyval)
		isCtrl := state&gdk.ControlMask != 0
		isShift := state&gdk.ShiftMask != 0

		if isCtrl && (keyval == gdk.KEY_Up || keyval == gdk.KEY_KP_Up || name == "Up" || name == "KP_Up") {
			if ib.OnSelectMessageUp != nil && ib.OnSelectMessageUp() {
				return true
			}
		}
		if isCtrl && (keyval == gdk.KEY_Down || keyval == gdk.KEY_KP_Down || name == "Down" || name == "KP_Down") {
			if ib.OnSelectMessageDown != nil && ib.OnSelectMessageDown() {
				return true
			}
		}
		if isCtrl && (keyval == gdk.KEY_Left || keyval == gdk.KEY_KP_Left || name == "Left" || name == "KP_Left") {
			if ib.OnSelectMessageBlockUp != nil && ib.OnSelectMessageBlockUp() {
				return true
			}
		}
		if isCtrl && (keyval == gdk.KEY_Right || keyval == gdk.KEY_KP_Right || name == "Right" || name == "KP_Right") {
			if ib.OnSelectMessageBlockDown != nil && ib.OnSelectMessageBlockDown() {
				return true
			}
		}
		if !isShift && (keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter || keyval == gdk.KEY_ISO_Enter || name == "Return" || name == "KP_Enter") {
			if ib.OnConfirmMessageSelection != nil && ib.OnConfirmMessageSelection() {
				return true
			}
			sendMsg()
			return true
		}
		if keyval == gdk.KEY_Escape || name == "Escape" {
			if ib.OnCancelMessageSelection != nil && ib.OnCancelMessageSelection() {
				return true
			}
		}
		if keyval == gdk.KEY_v && (state&gdk.ControlMask != 0) {
			clipboard := gdk.DisplayGetDefault().Clipboard()
			if clipboard.Formats().ContainGType(gdk.GTypeTexture) {
				clipboard.ReadTextureAsync(ib.ctx, func(res gio.AsyncResulter) {
					tex, err := clipboard.ReadTextureFinish(res)
					if err == nil && ib.OnPasteImage != nil {
						ib.OnPasteImage(gdk.BaseTexture(tex))
					}
				})
				return true
			}
		}
		return false
	})
	ib.TextView.AddController(keyCtrl)

	fileButton.ConnectClicked(func() {
		dialog := gtk.NewFileDialog()
		var window *gtk.Window
		if root := ib.Box.Root(); root != nil {
			if win, ok := root.Cast().(*gtk.Window); ok {
				window = win
			}
		}
		dialog.Open(ib.ctx, window, func(res gio.AsyncResulter) {
			file, err := dialog.OpenFinish(res)
			if err == nil && ib.OnSendFile != nil {
				ib.OnSendFile(file.Path())
			}
		})
	})

	sendButton.ConnectClicked(sendMsg)

	return ib
}

func (ib *InputBar) SetReplyTo(id, sender, content string) {
	ib.ReplyToID = id
	ib.ReplyToSender = sender
	ib.ReplyToContent = content

	markupSender := sender
	if !strings.Contains(sender, "<span") {
		markupSender = glib.MarkupEscapeText(sender)
	}

	ib.ReplyPreviewLabel.SetMarkup("Replying to <b>" + markupSender + "</b>: " + glib.MarkupEscapeText(content))
	ib.ReplyPreviewBox.Show()
	ib.TextView.GrabFocus()
}

func (ib *InputBar) CancelReply() {
	ib.ReplyToID = ""
	ib.ReplyToSender = ""
	ib.ReplyToContent = ""
	ib.ReplyPreviewBox.Hide()
}

func (ib *InputBar) GrabFocus() {
	ib.TextView.GrabFocus()
}

func (ib *InputBar) SetText(text string) {
	if ib.TextView != nil {
		buffer := ib.TextView.Buffer()
		if buffer != nil {
			buffer.SetText(text)
		}
		ib.TextView.GrabFocus()
	}
}
