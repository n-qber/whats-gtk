package call

import (
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"whats-gtk/internal/calling"
)

// CallWindow displays the active or incoming VoIP call dialog.
type CallWindow struct {
	Window        *adw.Window
	Avatar        *adw.Avatar
	NameLabel     *gtk.Label
	SubtitleLabel *gtk.Label
	StatusLabel   *gtk.Label
	DurationLabel *gtk.Label

	ActiveControlsBox   *gtk.Box
	IncomingControlsBox *gtk.Box

	MuteBtn    *gtk.ToggleButton
	HangupBtn  *gtk.Button
	AcceptBtn  *gtk.Button
	DeclineBtn *gtk.Button

	OnAccept     func()
	OnDecline    func()
	OnHangup     func()
	OnToggleMute func()

	autoCloseTimer glib.SourceHandle
}

// NewCallWindow creates and initializes a Libadwaita call window.
func NewCallWindow(parent *gtk.Window) *CallWindow {
	win := adw.NewWindow()
	win.SetTitle("Chamada WhatsApp")
	win.SetDefaultSize(360, 480)
	win.SetResizable(false)
	if parent != nil {
		win.SetTransientFor(parent)
	}

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.AddCSSClass("call-window-box")

	// Header Bar
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(true)
	titleWidget := adw.NewWindowTitle("WhatsApp VoIP", "")
	headerBar.SetTitleWidget(titleWidget)
	mainBox.Append(headerBar)

	// Content area
	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetHAlign(gtk.AlignCenter)
	contentBox.SetVAlign(gtk.AlignCenter)
	contentBox.SetVExpand(true)
	contentBox.SetMarginTop(20)
	contentBox.SetMarginBottom(24)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Large Avatar
	avatar := adw.NewAvatar(110, "", true)
	avatar.AddCSSClass("call-avatar")
	contentBox.Append(avatar)

	// Name label
	nameLabel := gtk.NewLabel("")
	nameLabel.AddCSSClass("title-1")
	nameLabel.SetWrap(true)
	nameLabel.SetMaxWidthChars(20)
	nameLabel.SetJustify(gtk.JustifyCenter)
	contentBox.Append(nameLabel)

	// Subtitle / Phone Number
	subLabel := gtk.NewLabel("")
	subLabel.AddCSSClass("dim-label")
	contentBox.Append(subLabel)

	// Status label ("Chamando...", "Tocando...", "Conectado", etc.)
	statusLabel := gtk.NewLabel("Iniciando chamada...")
	statusLabel.AddCSSClass("title-3")
	contentBox.Append(statusLabel)

	// Duration label ("00:00")
	durationLabel := gtk.NewLabel("")
	durationLabel.AddCSSClass("heading")
	durationLabel.SetVisible(false)
	contentBox.Append(durationLabel)

	// Controls Box: Active Call (Mute + Hangup)
	activeBox := gtk.NewBox(gtk.OrientationHorizontal, 32)
	activeBox.SetHAlign(gtk.AlignCenter)
	activeBox.SetMarginTop(16)

	muteBtn := gtk.NewToggleButton()
	muteBtn.SetIconName("audio-input-microphone-symbolic")
	muteBtn.AddCSSClass("circular")
	muteBtn.AddCSSClass("call-btn-circle")
	muteBtn.SetTooltipText("Silenciar Microfone")

	hangupBtn := gtk.NewButtonFromIconName("call-stop-symbolic")
	hangupBtn.AddCSSClass("circular")
	hangupBtn.AddCSSClass("destructive-action")
	hangupBtn.AddCSSClass("call-btn-circle")
	hangupBtn.SetTooltipText("Desligar")

	activeBox.Append(muteBtn)
	activeBox.Append(hangupBtn)
	contentBox.Append(activeBox)

	// Controls Box: Incoming Call (Decline + Accept)
	incomingBox := gtk.NewBox(gtk.OrientationHorizontal, 32)
	incomingBox.SetHAlign(gtk.AlignCenter)
	incomingBox.SetMarginTop(16)
	incomingBox.SetVisible(false)

	declineBtn := gtk.NewButtonFromIconName("call-stop-symbolic")
	declineBtn.AddCSSClass("circular")
	declineBtn.AddCSSClass("destructive-action")
	declineBtn.AddCSSClass("call-btn-circle")
	declineBtn.SetTooltipText("Recusar Chamada")

	acceptBtn := gtk.NewButtonFromIconName("call-start-symbolic")
	acceptBtn.AddCSSClass("circular")
	acceptBtn.AddCSSClass("suggested-action")
	acceptBtn.AddCSSClass("call-btn-circle")
	acceptBtn.SetTooltipText("Atender Chamada")

	incomingBox.Append(declineBtn)
	incomingBox.Append(acceptBtn)
	contentBox.Append(incomingBox)

	mainBox.Append(contentBox)
	win.SetContent(mainBox)

	cw := &CallWindow{
		Window:              win,
		Avatar:              avatar,
		NameLabel:           nameLabel,
		SubtitleLabel:       subLabel,
		StatusLabel:         statusLabel,
		DurationLabel:       durationLabel,
		ActiveControlsBox:   activeBox,
		IncomingControlsBox: incomingBox,
		MuteBtn:             muteBtn,
		HangupBtn:           hangupBtn,
		AcceptBtn:           acceptBtn,
		DeclineBtn:          declineBtn,
	}

	// Wire button clicks
	muteBtn.ConnectToggled(func() {
		if cw.OnToggleMute != nil {
			cw.OnToggleMute()
		}
	})

	hangupBtn.ConnectClicked(func() {
		if cw.OnHangup != nil {
			cw.OnHangup()
		}
	})

	acceptBtn.ConnectClicked(func() {
		if cw.OnAccept != nil {
			cw.OnAccept()
		}
	})

	declineBtn.ConnectClicked(func() {
		if cw.OnDecline != nil {
			cw.OnDecline()
		}
	})

	// Keyboard shortcuts controller
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(val uint, code uint, state gdk.ModifierType) bool {
		name := gdk.KeyvalName(val)
		if name == "Escape" {
			if cw.IncomingControlsBox.Visible() {
				if cw.OnDecline != nil {
					cw.OnDecline()
				}
			} else {
				if cw.OnHangup != nil {
					cw.OnHangup()
				}
			}
			return true
		} else if name == "Return" || name == "KP_Enter" {
			if cw.IncomingControlsBox.Visible() {
				if cw.OnAccept != nil {
					cw.OnAccept()
				}
				return true
			}
		} else if name == "m" || name == "M" {
			if cw.ActiveControlsBox.Visible() {
				cw.MuteBtn.SetActive(!cw.MuteBtn.Active())
				return true
			}
		}
		return false
	})
	win.AddController(keyCtrl)

	return cw
}

// Update updates the UI with the latest call state and remote contact info.
func (cw *CallWindow) Update(info calling.CallInfo, displayName, number string, avatarTex *gdk.Texture) {
	glib.IdleAdd(func() {
		if cw.Window == nil {
			return
		}

		if displayName != "" {
			cw.NameLabel.SetText(displayName)
			cw.Avatar.SetText(displayName)
		} else if number != "" {
			cw.NameLabel.SetText(number)
			cw.Avatar.SetText(number)
		}

		if number != "" && number != displayName {
			cw.SubtitleLabel.SetText(number)
			cw.SubtitleLabel.SetVisible(true)
		} else {
			cw.SubtitleLabel.SetVisible(false)
		}

		if avatarTex != nil {
			cw.Avatar.SetCustomImage(avatarTex)
		}

		cw.MuteBtn.SetActive(info.IsMuted)
		if info.IsMuted {
			cw.MuteBtn.SetIconName("microphone-sensitivity-muted-symbolic")
		} else {
			cw.MuteBtn.SetIconName("audio-input-microphone-symbolic")
		}

		switch info.State {
		case calling.CallStateIncoming:
			cw.StatusLabel.SetText("Recebendo chamada...")
			cw.DurationLabel.SetVisible(false)
			cw.IncomingControlsBox.SetVisible(true)
			cw.ActiveControlsBox.SetVisible(false)

		case calling.CallStateOutgoing:
			cw.StatusLabel.SetText("Chamando...")
			cw.DurationLabel.SetVisible(false)
			cw.IncomingControlsBox.SetVisible(false)
			cw.ActiveControlsBox.SetVisible(true)

		case calling.CallStateConnecting:
			cw.StatusLabel.SetText("Conectando...")
			cw.DurationLabel.SetVisible(false)
			cw.IncomingControlsBox.SetVisible(false)
			cw.ActiveControlsBox.SetVisible(true)

		case calling.CallStateActive:
			cw.StatusLabel.SetText("Em chamada")
			cw.IncomingControlsBox.SetVisible(false)
			cw.ActiveControlsBox.SetVisible(true)
			cw.DurationLabel.SetVisible(true)

			dur := info.Duration.Truncate(time.Second)
			mins := int(dur.Minutes())
			secs := int(dur.Seconds()) % 60
			cw.DurationLabel.SetText(fmt.Sprintf("%02d:%02d", mins, secs))

		case calling.CallStateEnded:
			msg := "Chamada encerrada"
			if info.EndReason != "" {
				msg = fmt.Sprintf("Chamada encerrada (%s)", info.EndReason)
			}
			cw.StatusLabel.SetText(msg)
			cw.IncomingControlsBox.SetVisible(false)
			cw.ActiveControlsBox.SetVisible(false)

			// Auto close window after 2.5 seconds
			if cw.autoCloseTimer == 0 {
				cw.autoCloseTimer = glib.TimeoutAdd(2500, func() bool {
					cw.Close()
					return false
				})
			}
		}
	})
}

// Show presents the call window.
func (cw *CallWindow) Show() {
	if cw.Window != nil {
		cw.Window.Present()
	}
}

// Close destroys and closes the window.
func (cw *CallWindow) Close() {
	if cw.autoCloseTimer != 0 {
		glib.SourceRemove(cw.autoCloseTimer)
		cw.autoCloseTimer = 0
	}
	if cw.Window != nil {
		cw.Window.Destroy()
		cw.Window = nil
	}
}
