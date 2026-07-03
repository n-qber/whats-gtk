package ui

import (
	"fmt"
	"whats-gtk/internal/ui/chat"
	"whats-gtk/internal/ui/sidebar"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type App struct {
	Window             *adw.ApplicationWindow
	Sidebar            *sidebar.Sidebar
	ChatView           *chat.ChatView
	QRDialog           *gtk.Window
	QRImage            *gtk.Image
	ZoomLevel          float64
	ZoomCSSProvider    *gtk.CSSProvider
	OnKeyPressed       func(key string, mods gdk.ModifierType) bool
	OnModifiersChanged func(mods gdk.ModifierType)
	DetachedChats      map[string]*chat.ChatView
	ActiveMainJID      string
}

func NewApp(app *adw.Application) (*App, error) {
	window := adw.NewApplicationWindow(&app.Application)
	window.SetTitle("WhatsApp GTK")
	window.SetDefaultSize(1000, 700)

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

	splitView.SetSidebar(s.Box)
	splitView.SetContent(cv.Box)
	window.SetContent(splitView)

	a := &App{
		Window:          window,
		Sidebar:         s,
		ChatView:        cv,
		ZoomLevel:       1.0,
		ZoomCSSProvider: gtk.NewCSSProvider(),
		DetachedChats:   make(map[string]*chat.ChatView),
	}

	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), a.ZoomCSSProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		keyName := gdk.KeyvalName(keyval)
		
		if state&gdk.ControlMask != 0 {
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
		.image-placeholder {
			background-color: rgba(0,0,0,0.05);
			border-radius: 4px;
			border: 1px dashed rgba(0,0,0,0.1);
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

func (a *App) GetChatViewForJID(jid string) *chat.ChatView {
	if cv, ok := a.DetachedChats[jid]; ok {
		return cv
	}
	if a.ActiveMainJID == jid {
		return a.ChatView
	}
	return nil
}
