package ui

import (
	"whats-gtk/internal/ui/chat"
	"whats-gtk/internal/ui/sidebar"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type App struct {
	Window   *gtk.ApplicationWindow
	Sidebar  *sidebar.Sidebar
	ChatView *chat.ChatView
}

func NewApp(app *gtk.Application) (*App, error) {
	window, err := gtk.ApplicationWindowNew(app)
	if err != nil {
		return nil, err
	}
	window.SetTitle("WhatsApp GTK")
	window.SetDefaultSize(1000, 700)

	paned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	
	s, err := sidebar.NewSidebar()
	if err != nil {
		return nil, err
	}

	cv, err := chat.NewChatView()
	if err != nil {
		return nil, err
	}

	loadCSS()

	paned.Pack1(s.Box, false, false)
	paned.Pack2(cv.Box, true, false)
	window.Add(paned)

	return &App{
		Window:   window,
		Sidebar:  s,
		ChatView: cv,
	}, nil
}

func loadCSS() {
	cssProvider, _ := gtk.CssProviderNew()
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
			font-family: Segoe UI, Roboto, Ubuntu, Cantarell, sans-serif;
		}
		.bubble-self { background-color: #dcf8c6; }
		.bubble-other { background-color: #ffffff; }
		.sidebar-name { font-size: 11pt; color: #111b21; }
		.chat-header { background-color: #f0f2f5; border-bottom: 1px solid #d1d7db; padding: 8px 16px; }
		.chat-header-name { font-weight: bold; font-size: 11pt; }
		.message-time { color: #667781; font-size: 7pt; margin-left: 2px; }
		.status-overlay { margin-top: -10px; margin-right: -4px; margin-bottom: -2px; }
		.message-sender-name { color: #008069; font-weight: bold; font-size: 9pt; margin-bottom: 2px; }
		.avatar { border-radius: 9999px; }
		.reactions-container {
			margin-top: 2px;
			margin-bottom: 2px;
			padding: 2px;
		}
		.reaction-badge {
			background-color: #ffffff;
			border-radius: 10px;
			padding: 2px 6px;
			font-size: 10pt;
			border: 1px solid #d1d7db;
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
	`)
	screen, _ := gdk.ScreenGetDefault()
	gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func (a *App) Show() { a.Window.ShowAll() }
