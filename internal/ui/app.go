package ui

import (
	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type App struct {
	Window                *gtk.ApplicationWindow
	ChatList              *gtk.ListBox
	MessageList           *gtk.ListBox
	MessageScrolledWindow *gtk.ScrolledWindow
	MessageEntry          *gtk.Entry
	SearchEntry           *gtk.Entry
	ChatHeaderLabel       *gtk.Label
	ChatHeaderImage       *gtk.Image
	ChatRows              map[string]*gtk.ListBoxRow
	MessageRows           map[string]bubbles.Bubble
	BubblesByJID          map[string][]bubbles.Bubble
	OnChatSelected        func(jid string)
	OnSendMessage         func(text string)
	OnSearch              func(text string)
}

func NewApp(app *gtk.Application) (*App, error) {
	window, err := gtk.ApplicationWindowNew(app)
	if err != nil {
		return nil, err
	}
	window.SetTitle("WhatsApp GTK")
	window.SetDefaultSize(1000, 700)

	paned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	sidebarBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	sidebarBox.SetSizeRequest(300, -1)

	searchEntry, _ := gtk.EntryNew()
	searchEntry.SetPlaceholderText("Search or start new chat")
	sidebarBox.PackStart(searchEntry, false, false, 5)

	chatList, _ := gtk.ListBoxNew()
	scrolledChat, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledChat.Add(chatList)
	sidebarBox.PackStart(scrolledChat, true, true, 0)

	chatViewBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
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
	chatViewBox.PackStart(headerBox, false, false, 0)

	messageList, _ := gtk.ListBoxNew()
	messageList.SetName("message-list")
	scrolledMsg, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledMsg.Add(messageList)
	chatViewBox.PackStart(scrolledMsg, true, true, 0)

	cssProvider, _ := gtk.CssProviderNew()
	cssProvider.LoadFromData(`
		#message-list { background-color: #efe7de; }
		.message-row { background-color: transparent; padding: 4px 10px; }
		.message-row-connected { padding-top: 1px; }
		.message-bubble {
			padding: 6px 10px;
			border-radius: 8px;
			margin: 2px 0;
			font-family: "Segoe UI", "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", "DejaVu Sans", sans-serif;
			box-shadow: 0 1px 0.5px rgba(0,0,0,0.13);
		}
		.bubble-self { background-color: #dcf8c6; color: #000; }
		.bubble-other { background-color: #ffffff; color: #000; }
		.sidebar-name { font-size: 10pt; font-weight: 500; color: #111b21; }
		.chat-header { background-color: #f0f2f5; border-bottom: 1px solid #d1d7db; padding: 8px 16px; }
		.chat-header-name { font-size: 11pt; font-weight: 600; color: #111b21; }
		.message-time { color: #667781; font-size: 8pt; }
		.message-image { border-radius: 6px; }
		.avatar {
			border-radius: 999px;
			-gtk-outline-radius: 999px;
		}
		.message-sender-name {
			color: #008069; font-size: 9pt; font-weight: 600; margin-bottom: 2px;
			font-family: "Segoe UI", "Roboto", "Cantarell", "Noto Sans", sans-serif;
		}
		.message-number { color: #8696a0; font-size: 8pt; font-weight: 300; }
		.receipt { font-size: 8pt; margin-left: 4px; }
		.receipt-sent { color: #8696a0; }
		.receipt-delivered { color: #8696a0; }
		.receipt-read { color: #53bdeb; }
		.receipt-pending { color: #8696a0; }
	`)
	screen, _ := gdk.ScreenGetDefault()
	gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	inputBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	messageEntry, _ := gtk.EntryNew()
	sendButton, _ := gtk.ButtonNewWithLabel("Send")
	inputBox.PackStart(messageEntry, true, true, 5)
	inputBox.PackStart(sendButton, false, false, 5)
	chatViewBox.PackStart(inputBox, false, false, 5)

	paned.Pack1(sidebarBox, false, false); paned.Pack2(chatViewBox, true, false); window.Add(paned)

	a := &App{
		Window: window, ChatList: chatList, MessageList: messageList,
		MessageScrolledWindow: scrolledMsg, MessageEntry: messageEntry, SearchEntry: searchEntry,
		ChatHeaderLabel: headerLabel, ChatHeaderImage: headerImage,
		ChatRows: make(map[string]*gtk.ListBoxRow),
		MessageRows: make(map[string]bubbles.Bubble),
		BubblesByJID: make(map[string][]bubbles.Bubble),
	}

	chatList.Connect("row-selected", func() {
		row := chatList.GetSelectedRow()
		if row != nil && a.OnChatSelected != nil {
			for jid, r := range a.ChatRows {
				if r.Native() == row.Native() { a.OnChatSelected(jid); break }
			}
		}
	})
	searchEntry.Connect("changed", func() {
		text, _ := searchEntry.GetText()
		if a.OnSearch != nil { a.OnSearch(text) }
	})
	sendMsg := func() {
		text, _ := messageEntry.GetText()
		if text != "" && a.OnSendMessage != nil { a.OnSendMessage(text); messageEntry.SetText("") }
	}
	messageEntry.Connect("activate", sendMsg); sendButton.Connect("clicked", sendMsg)
	return a, nil
}

func (a *App) Show() { a.Window.ShowAll() }
func (a *App) SetChatHeader(name string, pixbuf *gdk.Pixbuf) {
	a.ChatHeaderLabel.SetText(name)
	if pixbuf != nil { a.ChatHeaderImage.SetFromPixbuf(pixbuf) } else { a.ChatHeaderImage.Clear() }
}
func (a *App) AddChat(jid, name string) {
	row, _ := gtk.ListBoxRowNew(); label, _ := gtk.LabelNew(name)
	label.SetXAlign(0); label.SetMarginStart(12); label.SetMarginEnd(12); label.SetMarginTop(6); label.SetMarginBottom(6)
	lCtx, _ := label.GetStyleContext(); lCtx.AddClass("sidebar-name")
	row.Add(label); a.ChatList.Add(row); a.ChatRows[jid] = row; row.ShowAll()
}
func (a *App) MoveChatToTop(jid string) {
	if row, exists := a.ChatRows[jid]; exists { a.ChatList.Remove(row); a.ChatList.Insert(row, 0) }
}
func (a *App) ClearChats() {
	a.ChatRows = make(map[string]*gtk.ListBoxRow)
	children := a.ChatList.GetChildren(); children.Foreach(func(item interface{}) { a.ChatList.Remove(item.(gtk.IWidget)) })
	a.ChatList.ShowAll()
}
func (a *App) AddMessage(text string, isSelf bool) {
	a.AddMessageWithID("", "", "", text, isSelf, false, "", "", nil)
}
func (a *App) AddMessageWithID(id, jid, name, text string, isSelf bool, isCont bool, status string, timeStr string, avatar *gdk.Pixbuf) {
	bubble, err := bubbles.NewTextBubble(name, text, isSelf, status, timeStr, avatar)
	if err == nil {
		if id != "" { a.MessageRows[id] = bubble }
		if jid != "" { a.BubblesByJID[jid] = append(a.BubblesByJID[jid], bubble) }
		a.addBubble(bubble, isCont)
	}
}
func (a *App) UpdateMessageStatus(id, status string) {
	if bubble, exists := a.MessageRows[id]; exists {
		if tb, ok := bubble.(*bubbles.TextBubble); ok { glib.IdleAdd(func() { tb.SetStatus(status) })
		} else if ib, ok := bubble.(*bubbles.ImageBubble); ok { glib.IdleAdd(func() { ib.SetStatus(status) })
		} else if sb, ok := bubble.(*bubbles.StickerBubble); ok { glib.IdleAdd(func() { sb.SetStatus(status) }) }
	}
}
func (a *App) SetAvatar(jid string, pixbuf *gdk.Pixbuf) {
	if blist, exists := a.BubblesByJID[jid]; exists {
		glib.IdleAdd(func() {
			for _, b := range blist {
				if tb, ok := b.(*bubbles.TextBubble); ok { tb.UpdateAvatar(pixbuf)
				} else if ib, ok := b.(*bubbles.ImageBubble); ok { ib.UpdateAvatar(pixbuf)
				} else if sb, ok := b.(*bubbles.StickerBubble); ok { sb.UpdateAvatar(pixbuf) }
			}
		})
	}
}
func (a *App) AddPoll(q string, o []string, isSelf bool) {
	bubble, err := bubbles.NewPollBubble(q, o, isSelf)
	if err == nil { a.addBubble(bubble, false) }
}
func (a *App) AddAudio(isSelf bool) {
	bubble, err := bubbles.NewAudioBubble(isSelf)
	if err == nil { a.addBubble(bubble, false) }
}
func (a *App) AddImage(id, jid, name string, pixbuf *gdk.Pixbuf, isSelf bool, isCont bool, status, tStr string, av *gdk.Pixbuf) {
	bubble, err := bubbles.NewImageBubble(name, pixbuf, isSelf, status, tStr, av)
	if err == nil {
		if id != "" { a.MessageRows[id] = bubble }
		if jid != "" { a.BubblesByJID[jid] = append(a.BubblesByJID[jid], bubble) }
		a.addBubble(bubble, isCont)
	}
}
func (a *App) AddSticker(id, jid, name string, pixbuf *gdk.Pixbuf, isSelf bool, isCont bool, status, tStr string, av *gdk.Pixbuf) {
	bubble, err := bubbles.NewStickerBubble(name, pixbuf, isSelf, status, tStr, av)
	if err == nil {
		if id != "" { a.MessageRows[id] = bubble }
		if jid != "" { a.BubblesByJID[jid] = append(a.BubblesByJID[jid], bubble) }
		a.addBubble(bubble, isCont)
	}
}
func (a *App) addBubble(b bubbles.Bubble, isCont bool) {
	adj := a.MessageScrolledWindow.GetVAdjustment()
	wasAtBottom := adj.GetValue() >= (adj.GetUpper() - adj.GetPageSize() - 20)
	row, _ := gtk.ListBoxRowNew(); rCtx, _ := row.GetStyleContext()
	rCtx.AddClass("message-row"); if isCont { rCtx.AddClass("message-row-connected") }
	row.Add(b.Widget()); a.MessageList.Add(row); row.ShowAll()
	if wasAtBottom { glib.IdleAdd(func() { adj.SetValue(adj.GetUpper() - adj.GetPageSize()) }) }
}
func (a *App) ClearMessages() {
	a.MessageRows = make(map[string]bubbles.Bubble); a.BubblesByJID = make(map[string][]bubbles.Bubble)
	children := a.MessageList.GetChildren(); children.Foreach(func(item interface{}) { a.MessageList.Remove(item.(gtk.IWidget)) })
	a.MessageList.ShowAll()
}
