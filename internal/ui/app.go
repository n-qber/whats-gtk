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
		#message-list { background-color: #efe7de; }
		.message-row { background-color: transparent; padding: 4px 10px; }
		.message-row-connected { padding-top: 1px; }
		.message-bubble {
			padding: 6px 10px;
			border-radius: 8px;
			margin: 2px 0;
			font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol";
		}
		.bubble-self { background-color: #dcf8c6; color: #000; }
		.bubble-other { background-color: #ffffff; color: #000; }
		.sidebar-name { font-size: 10pt; font-weight: 500; color: #111b21; }
		.chat-header { background-color: #f0f2f5; border-bottom: 1px solid #d1d7db; padding: 8px 16px; }
		.chat-header-name { font-size: 11pt; font-weight: 600; color: #111b21; }
		.message-time { 
			color: #667781; 
			font-size: 8pt; 
			font-feature-settings: "tnum";
		}
		.message-image { border-radius: 6px; }
		.message-image-thumbnail { opacity: 0.6; }
		.message-sticker-thumbnail { opacity: 0.5; }
		.avatar {
			border-radius: 9999px;
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
		.reactions-container {
			margin-top: -4px;
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
