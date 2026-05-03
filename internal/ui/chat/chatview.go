package chat

import (
	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type ChatView struct {
	Box                   *gtk.Box
	MessageList           *gtk.ListBox
	MessageScrolledWindow *gtk.ScrolledWindow
	MessageEntry          *gtk.Entry
	ChatHeaderLabel       *gtk.Label
	ChatHeaderImage       *gtk.Image
	MessageRows           map[string]bubbles.Bubble
	BubblesByJID          map[string][]bubbles.Bubble
	OnSendMessage         func(text string)
	OnPasteImage          func(pixbuf *gdk.Pixbuf)
	OnDownloadMedia       func(id string)
	OnSendReaction        func(id, emoji string)
}

func NewChatView() (*ChatView, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}

	headerBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
	hCtx, _ := headerBox.GetStyleContext()
	hCtx.AddClass("chat-header")

	headerImage, _ := gtk.ImageNew()
	hiCtx, _ := headerImage.GetStyleContext()
	hiCtx.AddClass("avatar")

	headerLabel, _ := gtk.LabelNew("Select a chat")
	hlCtx, _ := headerLabel.GetStyleContext()
	hlCtx.AddClass("chat-header-name")

	headerBox.PackStart(headerImage, false, false, 0)
	headerBox.PackStart(headerLabel, false, false, 0)
	box.PackStart(headerBox, false, false, 0)

	messageList, _ := gtk.ListBoxNew()
	messageList.SetName("message-list")
	scrolledMsg, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledMsg.Add(messageList)
	box.PackStart(scrolledMsg, true, true, 0)

	inputBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	messageEntry, _ := gtk.EntryNew()
	sendButton, _ := gtk.ButtonNewWithLabel("Send")
	inputBox.PackStart(messageEntry, true, true, 5)
	inputBox.PackStart(sendButton, false, false, 5)
	box.PackStart(inputBox, false, false, 5)

	cv := &ChatView{
		Box:                   box,
		MessageList:           messageList,
		MessageScrolledWindow: scrolledMsg,
		MessageEntry:          messageEntry,
		ChatHeaderLabel:       headerLabel,
		ChatHeaderImage:       headerImage,
		MessageRows:           make(map[string]bubbles.Bubble),
		BubblesByJID:          make(map[string][]bubbles.Bubble),
	}

	messageEntry.Connect("key-press-event", func(entry *gtk.Entry, event *gdk.Event) bool {
		keyEvent := gdk.EventKey{Event: event}
		if keyEvent.State()&gdk.CONTROL_MASK != 0 && keyEvent.KeyVal() == gdk.KEY_v {
			clipboard, _ := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
			pixbuf, _ := clipboard.WaitForImage()
			if pixbuf != nil && cv.OnPasteImage != nil {
				cv.OnPasteImage(pixbuf)
				return true // Stop standard paste
			}
		}
		return false
	})

	sendMsg := func() {
		text, _ := messageEntry.GetText()
		if text != "" && cv.OnSendMessage != nil {
			cv.OnSendMessage(text)
			messageEntry.SetText("")
		}
	}
	messageEntry.Connect("activate", sendMsg)
	sendButton.Connect("clicked", sendMsg)

	return cv, nil
}

func (cv *ChatView) SetHeader(name string, pixbuf *gdk.Pixbuf) {
	cv.ChatHeaderLabel.SetText(name)
	if pixbuf != nil {
		bubbles.ApplyCircularAvatar(cv.ChatHeaderImage, pixbuf, 40)
	} else {
		cv.ChatHeaderImage.Clear()
	}
}

func (cv *ChatView) AddMessage(id, jid, name, text string, isSelf bool, isCont bool, status string, timeStr string, avatar *gdk.Pixbuf) {
	bubble, err := bubbles.NewTextBubble(name, text, isSelf, status, timeStr, avatar)
	if err == nil {
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddImage(id, jid, name string, pixbuf, thumb *gdk.Pixbuf, isSelf bool, isCont bool, status, tStr string, av *gdk.Pixbuf) {
	bubble, err := bubbles.NewImageBubble(name, pixbuf, thumb, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil { cv.OnDownloadMedia(id) }
		}
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddSticker(id, jid, name string, pixbuf, thumb *gdk.Pixbuf, isSelf bool, isCont bool, status, tStr string, av *gdk.Pixbuf) {
	bubble, err := bubbles.NewStickerBubble(name, pixbuf, thumb, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil { cv.OnDownloadMedia(id) }
		}
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
	cv.addBubble(id, b, isCont)
}

func (cv *ChatView) addBubble(id string, b bubbles.Bubble, isCont bool) {
	row, _ := gtk.ListBoxRowNew()
	rCtx, _ := row.GetStyleContext()
	rCtx.AddClass("message-row")
	if isCont {
		rCtx.AddClass("message-row-connected")
	}

	bubbleWidget := b.Widget().ToWidget()
	row.Add(bubbleWidget)

	// Context menu for reactions - Using interface{} for w to avoid type conversion panic
	bubbleWidget.Connect("button-press-event", func(w interface{}, event *gdk.Event) bool {
		e := gdk.EventButton{Event: event}
		if e.Type() == gdk.EVENT_BUTTON_PRESS && e.Button() == 3 { // Right click
			cv.showReactionMenu(id, e.Time())
			return true
		}
		return false
	})

	cv.MessageList.Add(row)
	row.ShowAll()
	bubbleWidget.ShowAll()
}

func (cv *ChatView) showReactionMenu(msgID string, timestamp uint32) {
	menu, _ := gtk.MenuNew()
	
	reactItem, _ := gtk.MenuItemNewWithLabel("React...")
	reactSub, _ := gtk.MenuNew()
	
	emojis := []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}
	for _, e := range emojis {
		item, _ := gtk.MenuItemNewWithLabel(e)
		item.Connect("activate", func() {
			if cv.OnSendReaction != nil {
				cv.OnSendReaction(msgID, e)
			}
		})
		reactSub.Append(item)
	}
	
	reactItem.SetSubmenu(reactSub)
	menu.Append(reactItem)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (cv *ChatView) ScrollToBottom() {
	glib.IdleAdd(func() {
		adj := cv.MessageScrolledWindow.GetVAdjustment()
		adj.SetValue(adj.GetUpper() - adj.GetPageSize())
	})
}

func (cv *ChatView) UpdateMessageStatus(id, status string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetStatus(status) })
	}
}

func (cv *ChatView) UpdateMessageImage(id string, pixbuf *gdk.Pixbuf) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateImage(pixbuf) })
	}
}

func (cv *ChatView) UpdateMessageReactions(id string, reactions []string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetReactions(reactions) })
	}
}

func (cv *ChatView) SetAvatar(jid string, pixbuf *gdk.Pixbuf) {
	if blist, exists := cv.BubblesByJID[jid]; exists {
		glib.IdleAdd(func() {
			for _, b := range blist {
				b.UpdateAvatar(pixbuf)
			}
		})
	}
}

func (cv *ChatView) Clear() {
	cv.MessageRows = make(map[string]bubbles.Bubble)
	cv.BubblesByJID = make(map[string][]bubbles.Bubble)
	children := cv.MessageList.GetChildren()
	children.Foreach(func(item interface{}) { cv.MessageList.Remove(item.(gtk.IWidget)) })
	cv.MessageList.ShowAll()
}
