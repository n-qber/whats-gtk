package chat

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type TopBar struct {
	Header      *adw.HeaderBar
	WindowTitle *adw.WindowTitle
	Avatar      *adw.Avatar

	DetachBtn    *gtk.Button
	AudioCallBtn *gtk.Button
	VideoCallBtn *gtk.Button

	OnHeaderClick func()
	OnDetach      func()
	OnCall        func(isVideo bool)
}

func NewTopBar() *TopBar {
	header := adw.NewHeaderBar()
	header.SetDecorationLayout(WindowDecorationLayout())
	header.SetShowEndTitleButtons(true)

	winTitle := adw.NewWindowTitle("Select a chat", "")
	winTitle.AddCSSClass("chat-header-name")
	winTitle.SetTooltipText("Ver informações do chat")
	header.SetTitleWidget(winTitle)

	headerAvatar := adw.NewAvatar(32, "", true)
	headerAvatar.SetTooltipText("Ver informações do chat")
	header.PackStart(headerAvatar)

	detachBtn := gtk.NewButtonFromIconName("window-new-symbolic")
	detachBtn.SetTooltipText("Detach chat into separate window")
	header.PackEnd(detachBtn)

	videoCallBtn := gtk.NewButtonFromIconName("camera-video-symbolic")
	videoCallBtn.SetTooltipText("Iniciar chamada de vídeo")
	videoCallBtn.SetVisible(false)
	header.PackEnd(videoCallBtn)

	audioCallBtn := gtk.NewButtonFromIconName("call-start-symbolic")
	audioCallBtn.SetTooltipText("Iniciar chamada de voz")
	audioCallBtn.SetVisible(false)
	header.PackEnd(audioCallBtn)

	tb := &TopBar{
		Header:       header,
		WindowTitle:  winTitle,
		Avatar:       headerAvatar,
		DetachBtn:    detachBtn,
		AudioCallBtn: audioCallBtn,
		VideoCallBtn: videoCallBtn,
	}

	audioCallBtn.ConnectClicked(func() {
		if tb.OnCall != nil {
			tb.OnCall(false)
		}
	})

	videoCallBtn.ConnectClicked(func() {
		if tb.OnCall != nil {
			tb.OnCall(true)
		}
	})

	detachBtn.ConnectClicked(func() {
		if tb.OnDetach != nil {
			tb.OnDetach()
		}
	})

	headerTitleGesture := gtk.NewGestureClick()
	headerTitleGesture.ConnectReleased(func(nPress int, x, y float64) {
		if tb.OnHeaderClick != nil {
			tb.OnHeaderClick()
		}
	})
	winTitle.AddController(headerTitleGesture)

	headerAvatarGesture := gtk.NewGestureClick()
	headerAvatarGesture.ConnectReleased(func(nPress int, x, y float64) {
		if tb.OnHeaderClick != nil {
			tb.OnHeaderClick()
		}
	})
	headerAvatar.AddController(headerAvatarGesture)

	return tb
}

func (tb *TopBar) SetInfo(name string, tex *gdk.Texture) {
	tb.WindowTitle.SetTitle(name)
	if name == "WhatsApp GTK" || name == "Select a chat" {
		tb.Avatar.SetVisible(false)
		tb.WindowTitle.SetSubtitle("")
		tb.AudioCallBtn.SetVisible(false)
		tb.VideoCallBtn.SetVisible(false)
	} else {
		tb.Avatar.SetVisible(true)
		tb.Avatar.SetText(name)
		tb.AudioCallBtn.SetVisible(true)
		tb.VideoCallBtn.SetVisible(true)
		if tex != nil {
			tb.Avatar.SetCustomImage(tex)
		} else {
			tb.Avatar.SetCustomImage(nil)
		}
	}
}

func (tb *TopBar) SetSubtitle(sub string) {
	if tb.WindowTitle != nil {
		tb.WindowTitle.SetSubtitle(sub)
	}
}

func (tb *TopBar) Subtitle() string {
	if tb.WindowTitle != nil {
		return tb.WindowTitle.Subtitle()
	}
	return ""
}

func (tb *TopBar) SetDetachAction(iconName, tooltip string) {
	if tb.DetachBtn != nil {
		if iconName != "" {
			tb.DetachBtn.SetIconName(iconName)
		}
		if tooltip != "" {
			tb.DetachBtn.SetTooltipText(tooltip)
		}
	}
}
