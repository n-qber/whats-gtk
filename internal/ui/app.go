package ui

import (
	"fmt"
	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type App struct {
	Window         *gtk.ApplicationWindow
	ChatList       *gtk.ListBox
	MessageList    *gtk.ListBox
	MessageEntry   *gtk.Entry
	OnChatSelected func(index int)
	OnSendMessage  func(text string)
}

func NewApp(app *gtk.Application) (*App, error) {
	window, err := gtk.ApplicationWindowNew(app)
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}
	window.SetTitle("whats-gtk")
	window.SetDefaultSize(1000, 700)

	// Sidebar
	sidebarBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	sidebarBox.SetSizeRequest(300, -1)

	searchEntry, _ := gtk.EntryNew()
	searchEntry.SetPlaceholderText("Search chats...")
	sidebarBox.PackStart(searchEntry, false, false, 5)

	chatList, _ := gtk.ListBoxNew()
	scrolledChat, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledChat.Add(chatList)
	sidebarBox.PackStart(scrolledChat, true, true, 0)

	// Chat View
	chatViewBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	
	messageList, _ := gtk.ListBoxNew()
	scrolledMsg, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledMsg.Add(messageList)
	chatViewBox.PackStart(scrolledMsg, true, true, 0)

	inputBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	messageEntry, _ := gtk.EntryNew()
	sendButton, _ := gtk.ButtonNewWithLabel("Send")
	inputBox.PackStart(messageEntry, true, true, 5)
	inputBox.PackStart(sendButton, false, false, 5)
	chatViewBox.PackStart(inputBox, false, false, 5)

	// Paned for split view
	paned, err := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	if err != nil {
		return nil, err
	}
	paned.Pack1(sidebarBox, false, false)
	paned.Pack2(chatViewBox, true, false)

	window.Add(paned)

	a := &App{
		Window:       window,
		ChatList:     chatList,
		MessageList:  messageList,
		MessageEntry: messageEntry,
	}

	chatList.Connect("row-selected", func() {
		row := chatList.GetSelectedRow()
		if row != nil && a.OnChatSelected != nil {
			a.OnChatSelected(row.GetIndex())
		}
	})

	sendButton.Connect("clicked", func() {
		text, _ := messageEntry.GetText()
		if text != "" && a.OnSendMessage != nil {
			a.OnSendMessage(text)
			messageEntry.SetText("")
		}
	})

	return a, nil
}

func (a *App) Show() {
	a.Window.ShowAll()
}

func (a *App) AddChat(name string) {
	row, _ := gtk.ListBoxRowNew()
	label, _ := gtk.LabelNew(name)
	label.SetXAlign(0)
	row.Add(label)
	a.ChatList.Add(row)
	row.ShowAll()
}

func (a *App) AddMessage(text string, isSelf bool) {
	bubble, err := bubbles.NewTextBubble(text, isSelf)
	if err != nil {
		fmt.Printf("Failed to create text bubble: %v\n", err)
		return
	}
	a.addBubble(bubble)
}

func (a *App) AddPoll(question string, options []string, isSelf bool) {
	bubble, err := bubbles.NewPollBubble(question, options, isSelf)
	if err != nil {
		fmt.Printf("Failed to create poll bubble: %v\n", err)
		return
	}
	a.addBubble(bubble)
}

func (a *App) AddAudio(isSelf bool) {
	bubble, err := bubbles.NewAudioBubble(isSelf)
	if err != nil {
		fmt.Printf("Failed to create audio bubble: %v\n", err)
		return
	}
	a.addBubble(bubble)
}

func (a *App) AddImage(pixbuf *gdk.Pixbuf, isSelf bool) {
	bubble, err := bubbles.NewImageBubble(pixbuf, isSelf)
	if err != nil {
		fmt.Printf("Failed to create image bubble: %v\n", err)
		return
	}
	a.addBubble(bubble)
}

func (a *App) AddSticker(pixbuf *gdk.Pixbuf, isSelf bool) {
	bubble, err := bubbles.NewStickerBubble(pixbuf, isSelf)
	if err != nil {
		fmt.Printf("Failed to create sticker bubble: %v\n", err)
		return
	}
	a.addBubble(bubble)
}

func (a *App) addBubble(b bubbles.Bubble) {
	row, _ := gtk.ListBoxRowNew()
	row.Add(b.Widget())
	a.MessageList.Add(row)
	row.ShowAll()
}

func (a *App) ClearMessages() {
	children := a.MessageList.GetChildren()
	children.Foreach(func(item interface{}) {
		widget := item.(gtk.IWidget)
		a.MessageList.Remove(widget)
	})
	a.MessageList.ShowAll()
}
