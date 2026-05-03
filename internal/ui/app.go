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
		.message-row { background-color: transparent; padding: 4px 12px; }
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
		.message-time { color: #667781; font-size: 8pt; }
		.message-sender-name { color: #008069; font-weight: bold; font-size: 9pt; margin-bottom: 2px; }
		.avatar { border-radius: 9999px; }
		.reactions-container {
			margin-top: -10px;
			margin-bottom: 2px;
		}
		.reaction-badge {
			background-color: #f0f2f5;
			border-radius: 12px;
			padding: 1px 4px;
			font-size: 9pt;
			border: 1px solid #ffffff;
		}
	`)
	screen, _ := gdk.ScreenGetDefault()
	gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func (a *App) Show() { a.Window.ShowAll() }
