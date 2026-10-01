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

	DetachBtn *gtk.Button

	OnHeaderClick func()
	OnDetach      func()
}

func NewTopBar() *TopBar {
	header := adw.NewHeaderBar()

	winTitle := adw.NewWindowTitle("Select a chat", "")
	winTitle.AddCSSClass("chat-header-name")
	header.SetTitleWidget(winTitle)

	headerAvatar := adw.NewAvatar(32, "", true)
	header.PackStart(headerAvatar)

	detachBtn := gtk.NewButtonFromIconName("window-new-symbolic")
	detachBtn.SetTooltipText("Detach chat into separate window")
	header.PackEnd(detachBtn)

	tb := &TopBar{
		Header:      header,
		WindowTitle: winTitle,
		Avatar:      headerAvatar,
		DetachBtn:   detachBtn,
	}

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
	if name == "WhatsApp GTK" {
		tb.Avatar.SetVisible(false)
		tb.WindowTitle.SetSubtitle("")
	} else {
		tb.Avatar.SetVisible(true)
		tb.Avatar.SetText(name)
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
