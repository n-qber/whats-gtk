package ui

import (
	"fmt"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui/chat"
	"whats-gtk/internal/ui/info"
	"whats-gtk/internal/ui/sidebar"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type App struct {
	Window             *adw.ApplicationWindow
	Sidebar            *sidebar.Sidebar
	ChatView           *chat.ChatView
	InfoView           *info.InfoView
	InfoFlap           *adw.Flap
	QRDialog           *gtk.Window
	QRImage            *gtk.Image
	ZoomLevel          float64
	ZoomCSSProvider    *gtk.CSSProvider
	OnKeyPressed       func(key string, mods gdk.ModifierType) bool
	OnModifiersChanged func(mods gdk.ModifierType)
	OnSearchRequested  func()
	DetachedChats      map[string]*chat.ChatView
	ActiveMainJID      string
	EventBus           *events.EventBus
	ResolveJIDFunc     func(jid string) string
}

func NewApp(app *adw.Application, bus *events.EventBus) (*App, error) {
	window := adw.NewApplicationWindow(&app.Application)
	window.SetTitle("WhatsApp GTK")
	window.SetDefaultSize(1000, 700)
	window.SetHideOnClose(true)

	loadCSS()

	splitView := adw.NewOverlaySplitView()
	
	s, err := sidebar.NewSidebar()
	if err != nil {
		return nil, err
	}

	cv, err := chat.NewChatView()
	if err != nil {
		return nil, err
	}

	iv := info.NewInfoView()

	infoFlap := adw.NewFlap()
	infoFlap.SetContent(cv.Box)
	infoFlap.SetFlap(iv.Box)
	infoFlap.SetFlapPosition(gtk.PackEnd)
	infoFlap.SetRevealFlap(false)

	cv.OnHeaderClick = func() {
		infoFlap.SetRevealFlap(!infoFlap.RevealFlap())
	}

	splitView.SetSidebar(s.Box)
	splitView.SetContent(infoFlap)
	window.SetContent(splitView)

	a := &App{
		Window:          window,
		Sidebar:         s,
		ChatView:        cv,
		InfoView:        iv,
		InfoFlap:        infoFlap,
		ZoomLevel:       1.0,
		ZoomCSSProvider: gtk.NewCSSProvider(),
		DetachedChats:   make(map[string]*chat.ChatView),
		EventBus:        bus,
	}

	a.setupSubscriptions()

	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), a.ZoomCSSProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		keyName := gdk.KeyvalName(keyval)
		
		if state&gdk.ControlMask != 0 && state&gdk.ShiftMask != 0 {
			if keyName == "f" || keyName == "F" {
				if a.OnSearchRequested != nil {
					a.OnSearchRequested()
					return true
				}
			}
		} else if state&gdk.ControlMask != 0 {
			switch keyName {
			case "plus", "equal", "KP_Add":
				a.Zoom(0.1)
				return true
			case "minus", "underscore", "KP_Subtract":
				a.Zoom(-0.1)
				return true
			case "space":
				a.Sidebar.SearchEntry.GrabFocus()
				return true
			}
		}

		if keyName == "Control_L" || keyName == "Control_R" {
			if a.OnModifiersChanged != nil {
				a.OnModifiersChanged(state | gdk.ControlMask)
			}
		}
		if a.OnKeyPressed != nil {
			return a.OnKeyPressed(keyName, state)
		}
		return false
	})
	keyCtrl.ConnectKeyReleased(func(keyval uint, keycode uint, state gdk.ModifierType) {
		keyName := gdk.KeyvalName(keyval)
		if keyName == "Control_L" || keyName == "Control_R" {
			if a.OnModifiersChanged != nil {
				// When released, we explicitly clear the control mask bit for the callback
				a.OnModifiersChanged(state &^ gdk.ControlMask)
			}
		}
	})
	window.AddController(keyCtrl)

	return a, nil
}

func (a *App) setupSubscriptions() {
	ch := a.EventBus.Subscribe(events.EventChatMessagesLoaded)
	go func() {
		for ev := range ch {
			if payload, ok := ev.Data.(events.ChatMessagesPayload); ok {
				glib.IdleAdd(func() {
					cv := a.GetChatViewForJID(payload.JID)
					if cv == nil { return }
					
					if !payload.Append {
						cv.Clear()
					}
					
					// If we're appending (loading older), we might need to handle scrolling gracefully.
					// For now, we'll just insert/append them.
					// Since they are older messages, ChatView's Messagelist needs to insert at top or we just append.
					// Wait, the order in payload is oldest first? Or oldest last?
					// In chat.go we preserve the order from database (newest to oldest or oldest to newest).
					// Assuming oldest to newest is what we send.
					for _, m := range payload.Messages {
						if m.DateSeparator != "" {
							cv.AddSeparator(m.DateSeparator)
						}
						
						if m.Type == "text" {
							cv.AddMessage(m.ID, m.JID, m.SenderName, m.Content, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
						} else if m.Type == "image" || m.Type == "video" || m.Type == "sticker" {
							var texImg, texThumb *gdk.Texture
							texThumb = bytesToTexture(m.Thumbnail)
							
							if m.DocumentPath != "" && m.Type != "sticker" {
								pixbuf, _ := gdkpixbuf.NewPixbufFromFile(m.DocumentPath)
								if pixbuf != nil {
									texImg = gdk.NewTextureForPixbuf(pixbuf)
								}
							}
							
							if m.Type == "image" {
								cv.AddImage(m.ID, m.JID, m.SenderName, m.Content, texImg, texThumb, m.DocumentPath, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
							} else if m.Type == "sticker" {
								cv.AddSticker(m.ID, m.JID, m.SenderName, m.StickerAnim, texImg, texThumb, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
							} else if m.Type == "video" {
								cv.AddVideo(m.ID, m.JID, m.SenderName, m.Content, texThumb, m.DocumentPath, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
							}
						} else if m.Type == "audio" {
							cv.AddAudio(m.ID, m.JID, m.SenderName, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
							if m.AudioPath != "" {
								cv.UpdateMessageAudio(m.ID, m.AudioPath)
							}
						} else if m.Type == "document" {
							texThumb := bytesToTexture(m.Thumbnail)
							cv.AddDocument(m.ID, m.JID, m.SenderName, m.Content, texThumb, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
							if m.DocumentPath != "" {
								cv.UpdateMessageDocument(m.ID, m.DocumentPath)
							}
						} else if m.Type == "poll" {
							cv.AddPoll(m.ID, m.JID, m.SenderName, m.Content, m.PollOptions, m.PollVotes, m.MyJID, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
						}
						
						if len(m.Reactions) > 0 {
							cv.UpdateMessageReactions(m.ID, m.Reactions)
						}
						if m.IsPinned {
							cv.UpdateMessagePinned(m.ID, true)
						}
						if m.IsViewOnce {
							if b, exists := cv.MessageList.MessageRows[m.ID]; exists {
								b.SetViewOnce(true)
							}
						}
						if m.IsForwarded {
							cv.UpdateMessageForwarded(m.ID, true)
						}
					}
					
					if !payload.Append {
						cv.ScrollToBottom()
					}
				})
			}
		}
	}()

	chLive := a.EventBus.Subscribe(events.EventLiveMessageReceived)
	go func() {
		for ev := range chLive {
			if payload, ok := ev.Data.(events.LiveUIMessagePayload); ok {
				glib.IdleAdd(func() {
					cv := a.GetChatViewForJID(payload.JID)
					if cv == nil { return }
					
					m := payload.Message
					if m.DateSeparator != "" {
						cv.AddSeparator(m.DateSeparator)
					}
					
					if m.Type == "text" {
						cv.AddMessage(m.ID, m.JID, m.SenderName, m.Content, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
					} else if m.Type == "image" || m.Type == "video" || m.Type == "sticker" {
						var texImg, texThumb *gdk.Texture
						texThumb = bytesToTexture(m.Thumbnail)
						
						if m.Content != "" && m.Type != "sticker" {
							pixbuf, _ := gdkpixbuf.NewPixbufFromFile(m.Content)
							if pixbuf != nil {
								texImg = gdk.NewTextureForPixbuf(pixbuf)
							}
						}
						
						if m.Type == "image" {
							cv.AddImage(m.ID, m.JID, m.SenderName, m.Content, texImg, texThumb, m.DocumentPath, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
						} else if m.Type == "sticker" {
							cv.AddSticker(m.ID, m.JID, m.SenderName, m.StickerAnim, texImg, texThumb, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
						} else if m.Type == "video" {
							cv.AddVideo(m.ID, m.JID, m.SenderName, m.Content, texThumb, m.DocumentPath, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText, m.MediaWidth, m.MediaHeight)
						}
					} else if m.Type == "audio" {
						cv.AddAudio(m.ID, m.JID, m.SenderName, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
						if m.AudioPath != "" {
							cv.UpdateMessageAudio(m.ID, m.AudioPath)
						}
					} else if m.Type == "document" {
						texThumb := bytesToTexture(m.Thumbnail)
						cv.AddDocument(m.ID, m.JID, m.SenderName, m.Content, texThumb, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
						if m.DocumentPath != "" {
							cv.UpdateMessageDocument(m.ID, m.DocumentPath)
						}
					} else if m.Type == "poll" {
						cv.AddPoll(m.ID, m.JID, m.SenderName, m.Content, m.PollOptions, m.PollVotes, m.MyJID, m.IsFromMe, m.IsContinuation, m.Status, m.TimeString, m.Avatar, m.QuotedMsgID, m.QuotedMsgSender, m.QuotedMsgText)
					}
					
					if len(m.Reactions) > 0 {
						cv.UpdateMessageReactions(m.ID, m.Reactions)
					}
					if m.IsPinned {
						cv.UpdateMessagePinned(m.ID, true)
					}
					if m.IsViewOnce {
						if b, exists := cv.MessageList.MessageRows[m.ID]; exists {
							b.SetViewOnce(true)
						}
					}
					if m.IsForwarded {
						cv.UpdateMessageForwarded(m.ID, true)
					}

					cv.ScrollToBottom()
				})
			}
		}
	}()

	chSidebar := a.EventBus.Subscribe(events.EventChatUpdated)
	go func() {
		for ev := range chSidebar {
			if jid, ok := ev.Data.(string); ok {
				glib.IdleAdd(func() {
					a.Sidebar.MoveChatToTop(jid)
				})
			}
		}
	}()

	chContacts := a.EventBus.Subscribe(events.EventContactsUpdated)
	go func() {
		for ev := range chContacts {
			if items, ok := ev.Data.([]events.SidebarItem); ok {
				glib.IdleAdd(func() {
					a.Sidebar.SetRefreshing(true)
					a.Sidebar.ClearChats()
					foundActive := false
					for _, item := range items {
						a.Sidebar.AddChat(item.JID, item.Name, item.IsGroup, item.UnreadCount, item.IsPinned)
						if item.Avatar != nil {
							a.Sidebar.SetAvatar(item.JID, item.Avatar)
						}
						if item.JID == a.ActiveMainJID {
							foundActive = true
						}
					}
					if foundActive && a.ActiveMainJID != "" {
						a.Sidebar.SelectChat(a.ActiveMainJID)
					} else {
						a.Sidebar.ClearSelection()
					}
					a.Sidebar.SetRefreshing(false)
				})
			}
		}
	}()
}

func bytesToTexture(data []byte) *gdk.Texture {
	if len(data) == 0 { return nil }
	loader := gdkpixbuf.NewPixbufLoader()
	loader.Write(data)
	loader.Close()
	pix := loader.Pixbuf()
	if pix == nil { return nil }
	return gdk.NewTextureForPixbuf(pix)
}

func loadCSS() {
	cssProvider := gtk.NewCSSProvider()
	cssProvider.LoadFromData(`
		#chat-view-box { background-color: #efe7de; }
		#message-list { background-color: #efe7de; }
		.message-row { background-color: transparent; padding: 8px 12px; }
		.message-row-connected { padding-top: 1px; padding-bottom: 1px; }
		.message-bubble {
			padding: 6px 10px;
			border-radius: 8px;
			margin: 2px 0;
			color: #000000;
		}
		.bubble-self { background-color: #dcf8c6; }
		.bubble-other { background-color: #ffffff; }
		.chat-header-name { font-weight: bold; font-size: 11pt; }
		.message-time { color: #667781; font-size: 7pt; margin-left: 2px; }
		.status-overlay { margin-top: -10px; margin-right: -4px; margin-bottom: -2px; }
		.message-sender-name { color: #008069; font-weight: bold; font-size: 9pt; margin-bottom: 2px; }
		.message-image { border-radius: 4px; }
		.document-bubble-main {
			background-color: rgba(0,0,0,0.03);
			border-radius: 4px;
			overflow: hidden;
		}
		.document-preview-image {
			min-height: 120px;
			max-height: 120px;
			min-width: 240px;
			object-fit: cover;
		}
		.document-info-box {
			background-color: rgba(0,0,0,0.05);
		}
		.sticker-placeholder {
			background-color: rgba(0,0,0,0.05);
			border-radius: 8px;
			border: 1px dashed rgba(0,0,0,0.1);
		}
		.image-container {
			border-radius: 8px;
			overflow: hidden;
		}
		.image-placeholder {
			background-color: rgba(0, 0, 0, 0.12);
			border-radius: 8px;
		}
		.image-download-button {
			background-color: rgba(0, 0, 0, 0.55);
			color: #ffffff;
			border-radius: 50%;
			width: 48px;
			height: 48px;
			margin: auto;
			box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
		}
		.image-download-button:hover {
			background-color: rgba(0, 0, 0, 0.75);
		}
		.message-sticker { margin: 4px; }
		.reactions-container {
			margin-top: 2px;
			margin-bottom: 2px;
		}
		.reaction-badge {
			background-color: #ffffff;
			border: 1px solid #d1d7db;
			border-radius: 12px;
			padding: 2px 6px;
			font-size: 10pt;
			box-shadow: 0 1px 1px rgba(0,0,0,0.1);
		}
		.quoted-message {
			background-color: rgba(0,0,0,0.05);
			border-left: 4px solid #34b7f1;
			padding: 4px 8px;
			border-radius: 4px;
			margin-bottom: 4px;
		}
		.quoted-sender {
			color: #34b7f1;
			font-weight: bold;
			font-size: 9pt;
		}
		.reply-preview {
			background-color: #f0f2f5;
			border-top: 1px solid #d1d7db;
			padding: 8px 16px;
		}
		.audio-play-button {
			background: transparent;
			border: none;
			color: #667781;
			padding: 0;
			margin: 0;
		}
		.audio-play-button:hover {
			color: #111b21;
		}
		.message-reactions-btn {
			background: transparent;
			border: none;
			color: #8696a0;
			padding: 0;
			margin: 0 4px;
		}
		.message-reactions-btn:hover {
			color: #111b21;
		}
		scale.audio-slider contents trough highlight {
			background-color: #00a884;
		}
		scale.audio-slider contents trough {
			background-color: #d1d7db;
			min-height: 4px;
		}
		scale.audio-slider contents trough slider {
			background-color: #00a884;
			min-width: 12px;
			min-height: 12px;
			margin: -4px;
		}
		.pinned-message-bar {
			background-color: #ffffff;
			border-bottom: 1px solid #d1d7db;
			padding: 8px 16px;
			box-shadow: 0 1px 2px rgba(0,0,0,0.05);
		}
		.pin-icon {
			color: #8696a0;
			margin-right: 4px;
		}
		.receipt-read {
			color: #53bdeb;
		}
		.date-separator {
			background-color: #ffffff;
			border-radius: 8px;
			padding: 4px 12px;
			margin: 8px auto;
			font-size: 9pt;
			color: #54656f;
			box-shadow: 0 1px 1px rgba(0,0,0,0.05);
		}
		.media-progress {
			margin-top: 4px;
			min-height: 4px;
		}
		progressbar.media-progress trough {
			min-height: 4px;
			border-radius: 2px;
		}
		progressbar.media-progress progress {
			background-color: #00a884;
			border-radius: 2px;
		}
		.forwarded-indicator {
			color: #667781;
			margin-bottom: 2px;
		}
		.forwarded-label {
			font-size: 8.5pt;
			font-style: italic;
			color: #667781;
		}
		.highlighted-row {
			background-color: rgba(0, 168, 132, 0.18);
			border-radius: 4px;
			transition: background-color 0.8s ease-out;
		}
		.highlighted-row .message-bubble {
			background-color: #d9fdd3;
			box-shadow: 0 0 0 2px #00a884, 0 2px 8px rgba(0, 168, 132, 0.4);
			transition: background-color 0.8s ease-out, box-shadow 0.8s ease-out;
		}
		.selection-bar {
			background-color: #f0f2f5;
			border-bottom: 1px solid #d1d7db;
			padding: 8px 16px;
			box-shadow: 0 1px 3px rgba(0,0,0,0.1);
		}
		.message-row-selected {
			background-color: rgba(0, 168, 132, 0.22);
			border-radius: 6px;
		}
		.message-row-selected .message-bubble {
			box-shadow: 0 0 0 2px #00a884;
		}
	`)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func (a *App) Show() { a.Window.Present() }

func (a *App) ShowQRCode(tex *gdk.Texture) {
	if a.QRDialog == nil {
		a.QRDialog = gtk.NewWindow()
		a.QRDialog.SetTitle("Scan QR Code")
		a.QRDialog.SetTransientFor(&a.Window.Window)
		a.QRDialog.SetModal(true)
		a.QRDialog.SetResizable(false)

		box := gtk.NewBox(gtk.OrientationVertical, 10)
		box.SetMarginBottom(20)
		box.SetMarginTop(20)
		box.SetMarginStart(20)
		box.SetMarginEnd(20)

		label := gtk.NewLabel("Scan this QR code with WhatsApp on your phone")
		box.Append(label)

		a.QRImage = gtk.NewImage()
		a.QRImage.SetPixelSize(256)
		a.QRImage.SetFromPaintable(tex)
		box.Append(a.QRImage)

		a.QRDialog.SetChild(box)
	} else {
		a.QRImage.SetFromPaintable(tex)
	}
	a.QRDialog.Present()
}

func (a *App) HideQRCode() {
	if a.QRDialog != nil {
		a.QRDialog.Destroy()
		a.QRDialog = nil
		a.QRImage = nil
	}
}

func (a *App) Zoom(delta float64) {
	a.ZoomLevel += delta
	if a.ZoomLevel < 0.5 {
		a.ZoomLevel = 0.5
	} else if a.ZoomLevel > 3.0 {
		a.ZoomLevel = 3.0
	}
	a.applyZoom()
}

func (a *App) ResetZoom() {
	a.ZoomLevel = 1.0
	a.applyZoom()
}

func (a *App) applyZoom() {
	css := "window { font-size: " + fmt.Sprintf("%.1f", a.ZoomLevel*10) + "pt; }"
	a.ZoomCSSProvider.LoadFromData(css)
}

func (a *App) isSameJID(j1, j2 string) bool {
	if j1 == j2 {
		return true
	}
	if j1 == "" || j2 == "" {
		return false
	}
	if a.ResolveJIDFunc != nil {
		r1 := a.ResolveJIDFunc(j1)
		r2 := a.ResolveJIDFunc(j2)
		if r1 != "" && r1 == r2 {
			return true
		}
	}
	return false
}

func (a *App) GetChatViewForJID(jid string) *chat.ChatView {
	if jid == "" {
		return nil
	}
	if cv, ok := a.DetachedChats[jid]; ok {
		return cv
	}
	if a.ActiveMainJID == jid {
		return a.ChatView
	}
	for dJID, cv := range a.DetachedChats {
		if a.isSameJID(dJID, jid) {
			return cv
		}
	}
	if a.isSameJID(a.ActiveMainJID, jid) {
		return a.ChatView
	}
	return nil
}
