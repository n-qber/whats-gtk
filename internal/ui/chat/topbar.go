package chat

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type TopBar struct {
	Header *adw.HeaderBar
	Label  *gtk.Label
	Avatar *adw.Avatar

	DetachBtn *gtk.Button

	OnHeaderClick func()
	OnDetach      func()
}

func NewTopBar() *TopBar {
	header := adw.NewHeaderBar()

	headerLabel := gtk.NewLabel("Select a chat")
	headerLabel.AddCSSClass("chat-header-name")
	header.SetTitleWidget(headerLabel)

	headerAvatar := adw.NewAvatar(32, "", true)
	header.PackStart(headerAvatar)

	detachBtn := gtk.NewButtonFromIconName("window-new-symbolic")
	detachBtn.SetTooltipText("Detach chat into separate window")
	header.PackEnd(detachBtn)

	tb := &TopBar{
		Header:    header,
		Label:     headerLabel,
		Avatar:    headerAvatar,
		DetachBtn: detachBtn,
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
	headerLabel.AddController(headerTitleGesture)

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
	tb.Label.SetText(name)
	if name == "WhatsApp GTK" {
		tb.Avatar.SetVisible(false)
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
