package chat

import (
	"fmt"
	"strings"

	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type ChatView struct {
	Box                   *gtk.Box
	MessageList           *gtk.ListBox
	MessageScrolledWindow *gtk.ScrolledWindow
	MessageEntry          *gtk.Entry
	ChatHeaderLabel       *gtk.Label
	ChatHeaderImage       *adw.Avatar
	MessageRows           map[string]bubbles.Bubble
	MessageListRows       map[string]*gtk.ListBoxRow
	BubblesByJID          map[string][]bubbles.Bubble
	OnSendMessage         func(text string, replyToID string)
	OnPasteImage          func(tex *gdk.Texture)
	OnDownloadMedia       func(id string)
	OnSendReaction        func(id, emoji string)
	AudioPlayer           *AudioPlayer
	ReplyToID             string
	ReplyToSender         string
	ReplyToContent        string
	ReplyPreviewBox       *gtk.Box
	ReplyPreviewLabel     *gtk.Label
}

func NewChatView() (*ChatView, error) {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetName("chat-view-box")

	header := adw.NewHeaderBar()
	
	headerLabel := gtk.NewLabel("Select a chat")
	headerLabel.AddCSSClass("chat-header-name")
	header.SetTitleWidget(headerLabel)
	
	headerAvatar := adw.NewAvatar(32, "", true)
	header.PackStart(headerAvatar)
	
	box.Append(header)

	messageList := gtk.NewListBox()
	messageList.SetName("message-list")
	messageList.SetSelectionMode(gtk.SelectionNone)

	scrolledMsg := gtk.NewScrolledWindow()
	scrolledMsg.SetVExpand(true)
	scrolledMsg.SetChild(messageList)
	box.Append(scrolledMsg)

	replyPreviewBox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	replyPreviewBox.AddCSSClass("reply-preview")
	replyLabel := gtk.NewLabel("")
	replyLabel.SetXAlign(0)
	replyLabel.SetEllipsize(pango.EllipsizeEnd)
	closeReplyBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	replyPreviewBox.Append(replyLabel)
	replyPreviewBox.Append(closeReplyBtn)
	replyPreviewBox.Hide()
	box.Append(replyPreviewBox)

	inputBox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	inputBox.SetMarginTop(6)
	inputBox.SetMarginBottom(6)
	inputBox.SetMarginStart(6)
	inputBox.SetMarginEnd(6)
	
	messageEntry := gtk.NewEntry()
	messageEntry.SetHExpand(true)
	sendButton := gtk.NewButtonWithLabel("Send")
	inputBox.Append(messageEntry)
	inputBox.Append(sendButton)
	box.Append(inputBox)

	cv := &ChatView{
		Box:                   box,
		MessageList:           messageList,
		MessageScrolledWindow: scrolledMsg,
		MessageEntry:          messageEntry,
		ChatHeaderLabel:       headerLabel,
		ChatHeaderImage:       headerAvatar,
		MessageRows:           make(map[string]bubbles.Bubble),
		MessageListRows:       make(map[string]*gtk.ListBoxRow),
		BubblesByJID:          make(map[string][]bubbles.Bubble),
		AudioPlayer:           NewAudioPlayer(),
		ReplyPreviewBox:       replyPreviewBox,
		ReplyPreviewLabel:     replyLabel,
	}

	closeReplyBtn.ConnectClicked(func() {
		cv.CancelReply()
	})

	sendMsg := func() {
		text := messageEntry.Text()
		if text != "" && cv.OnSendMessage != nil {
			cv.OnSendMessage(text, cv.ReplyToID)
			cv.CancelReply()
			messageEntry.SetText("")
		}
	}

	messageEntry.ConnectActivate(sendMsg)
	sendButton.ConnectClicked(sendMsg)

	return cv, nil
}

func (cv *ChatView) SetHeader(name string, tex *gdk.Texture) {
	cv.ChatHeaderLabel.SetText(name)
	cv.ChatHeaderImage.SetText(name)
	if tex != nil {
		cv.ChatHeaderImage.SetCustomImage(tex)
	}
}

func (cv *ChatView) SetReplyTo(id, sender, content string) {
	cv.ReplyToID = id
	cv.ReplyToSender = sender
	cv.ReplyToContent = content

	markupSender := sender
	if !strings.Contains(sender, "<span") {
		markupSender = glib.MarkupEscapeText(sender)
	}

	cv.ReplyPreviewLabel.SetMarkup("Replying to <b>" + markupSender + "</b>: " + glib.MarkupEscapeText(content))
	cv.ReplyPreviewBox.Show()
	glib.IdleAdd(func() {
		cv.MessageEntry.GrabFocus()
	})
}

func (cv *ChatView) CancelReply() {
	cv.ReplyToID = ""
	cv.ReplyToSender = ""
	cv.ReplyToContent = ""
	cv.ReplyPreviewBox.Hide()
}

func (cv *ChatView) AddMessage(id, jid, name, text string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewTextBubble(name, text, isSelf, status, tStr, av)
	if err == nil {
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddImage(id, jid, name string, tex, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewImageBubble(name, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddSticker(id, jid, name string, tex, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewStickerBubble(name, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddAudio(id, jid, name string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewAudioBubble(name, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.OnPlayRequest = func() {
			err := cv.AudioPlayer.Play(bubble.AudioPath(), func() {
				glib.IdleAdd(func() {
					bubble.SetPlaying(false)
				})
			})
			if err != nil {
				fmt.Printf("ChatView: Audio play error: %v\n", err)
				return
			}
			bubble.SetPlaying(true)
		}
		bubble.OnStopRequest = func() {
			cv.AudioPlayer.Stop()
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddVideo(id, jid, name string, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	// For now, video uses image bubble with thumbnail
	bubble, err := bubbles.NewImageBubble(name, nil, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddDocument(id, jid, name, fileName string, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewDocumentBubble(name, fileName, thumb, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddPoll(id, jid, name, question string, options []string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewPollBubble(name, question, options, isSelf, status, tStr, av)
	if err == nil {
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) registerBubble(id, jid string, b bubbles.Bubble, isCont bool) {
	if id != "" {
		cv.MessageRows[id] = b
	}
	if jid != "" {
		cv.BubblesByJID[jid] = append(cv.BubblesByJID[jid], b)
	}
	
	b.SetOnQuotedClick(func(quotedID string) {
		cv.ScrollToMessage(quotedID)
	})

	b.SetOnReplyRequest(func() {
		sender := b.Sender()
		if sender == "" { sender = "Unknown" }
		cv.SetReplyTo(id, sender, b.Content())
	})

	b.SetOnReactionRequest(func(emoji string) {
		if cv.OnSendReaction != nil {
			cv.OnSendReaction(id, emoji)
		}
	})

	cv.addBubble(id, b, isCont)
}

func (cv *ChatView) ScrollToMessage(id string) {
	if row, ok := cv.MessageListRows[id]; ok {
		glib.IdleAdd(func() {
			adj := cv.MessageScrolledWindow.VAdjustment()
			// In GTK4 we can use row.TranslateCoordinates to get position
			// or just use SelectRow and let the adjustment handle it if possible.
			// Better way:
			cv.MessageList.SelectRow(row)
			
			// Force scroll to row
			row.GrabFocus()
			
			// Get root coordinates of the row relative to the listbox
			_, y, _ := row.TranslateCoordinates(cv.MessageList, 0, 0)
			adj.SetValue(y)
		})
	}
}
func (cv *ChatView) showContextMenu(id string, b bubbles.Bubble) {
	// In GTK4, we use PopoverMenu or a simpler approach
	// [TODO: Implement GTK4 context menu]
}

func (cv *ChatView) addBubble(id string, b bubbles.Bubble, isCont bool) {
	row := gtk.NewListBoxRow()
	row.SetFocusable(false)
	row.AddCSSClass("message-row")
	if isCont {
		row.AddCSSClass("message-row-connected")
	}

	row.SetChild(b.Widget())

	click := gtk.NewGestureClick()
	click.SetButton(3) // Right click
	click.ConnectPressed(func(n int, x, y float64) {
		cv.showContextMenu(id, b)
	})
	row.AddController(click)

	if id != "" {
		cv.MessageListRows[id] = row
	}

	cv.MessageList.Append(row)
	glib.IdleAdd(func() {
		cv.ScrollToBottom()
	})
}

func (cv *ChatView) ScrollToBottom() {
	glib.IdleAdd(func() {
		adj := cv.MessageScrolledWindow.VAdjustment()
		adj.SetValue(adj.Upper() - adj.PageSize())
	})
}

func (cv *ChatView) UpdateMessageStatus(id, status string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetStatus(status) })
	}
}

func (cv *ChatView) UpdateMessageReactions(id string, reactions []string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetReactions(reactions) })
	}
}

func (cv *ChatView) UpdateMessageImage(id string, tex *gdk.Texture) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateImage(tex) })
	}
}

func (cv *ChatView) UpdateMessageDocument(id, path string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateDocument(path) })
	}
}

func (cv *ChatView) UpdateMessageAudio(id, path string) {
	if b, ok := cv.MessageRows[id]; ok {
		if ab, ok := b.(*bubbles.AudioBubble); ok {
			ab.SetAudioPath(path)
		}
	}
}

func (cv *ChatView) SetAvatar(jid string, tex *gdk.Texture) {
	if bubbles, exists := cv.BubblesByJID[jid]; exists {
		glib.IdleAdd(func() {
			for _, b := range bubbles {
				b.UpdateAvatar(tex)
			}
		})
	}
}

func (cv *ChatView) Clear() {
	cv.MessageRows = make(map[string]bubbles.Bubble)
	cv.BubblesByJID = make(map[string][]bubbles.Bubble)
	cv.MessageListRows = make(map[string]*gtk.ListBoxRow)
	
	for {
		child := cv.MessageList.FirstChild()
		if child == nil {
			break
		}
		cv.MessageList.Remove(child)
	}
}

func (cv *ChatView) FocusEntry() {
	glib.IdleAdd(func() {
		cv.MessageEntry.GrabFocus()
	})
}

