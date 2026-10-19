package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/purpshell/meowcaller"
	"whats-gtk/internal/calling"
	"whats-gtk/internal/events"
)

// CallingController manages the VoIP call workflow, hooking the audio engine
// into the Bridge, event bus, desktop notifications, and GTK CallWindow.
type CallingController struct {
	bridge       *Bridge
	Manager      *calling.Manager
	activeJID    string
	activeName   string
	activeNumber string
	activeAvatar *gdk.Texture
}

// NewCallingController creates and initializes the calling controller.
func NewCallingController(br *Bridge) *CallingController {
	var callClient *meowcaller.Client
	if br.Backend != nil {
		callClient = br.Backend.CallClient
	}

	mgr := calling.NewManager(callClient)
	cc := &CallingController{
		bridge:  br,
		Manager: mgr,
	}

	mgr.Subscribe(cc.handleCallStateChanged)
	mgr.SetVideoFrameCallback(func(tex *gdk.Texture) {
		glib.IdleAdd(func() {
			if cc.bridge.App != nil && cc.bridge.App.CallWindow != nil {
				cc.bridge.App.CallWindow.SetVideoTexture(tex)
			}
		})
	})
	return cc
}

// StartCall initiates an outbound voice or video call.
func (cc *CallingController) StartCall(targetJID string, isVideo bool) error {
	cleanJID := strings.TrimSpace(targetJID)
	if cleanJID == "" {
		return fmt.Errorf("invalid empty target JID")
	}

	name := cleanJID
	number := cleanJID
	if strings.Contains(cleanJID, "@") {
		number = "+" + strings.Split(cleanJID, "@")[0]
	}
	var avatar *gdk.Texture

	if cc.bridge.Contacts != nil {
		avatar = cc.bridge.Contacts.GetAvatar(cleanJID)
		if clean := cc.bridge.Contacts.GetCleanName(cleanJID); clean != "" {
			name = clean
		}
	}
	if cc.bridge.DB != nil {
		if contact, _ := cc.bridge.DB.GetContact(cleanJID); contact != nil {
			if dn := contact.DisplayName(); dn != "" && dn != contact.JID {
				name = dn
			}
		}
	}

	cc.activeJID = cleanJID
	cc.activeName = name
	cc.activeNumber = number
	cc.activeAvatar = avatar

	// Present the call window immediately in outgoing state
	glib.IdleAdd(func() {
		win := cc.bridge.App.ShowCallWindow()
		win.OnHangup = func() { _ = cc.Manager.Hangup() }
		win.OnToggleMute = func() { cc.Manager.ToggleMute() }
		win.OnToggleScreenShare = func() { _ = cc.ToggleScreenShare() }
		initInfo := calling.CallInfo{
			RemoteJID: cleanJID,
			Direction: calling.DirectionOutgoing,
			IsVideo:   isVideo,
			State:     calling.CallStateOutgoing,
		}
		win.Update(initInfo, name, number, avatar)
	})

	ctx := cc.bridge.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	_, err := cc.Manager.StartCall(ctx, cleanJID, isVideo)
	return err
}

// AnswerCall accepts an incoming call.
func (cc *CallingController) AnswerCall() error {
	return cc.Manager.AnswerCall()
}

// RejectCall declines an incoming call.
func (cc *CallingController) RejectCall() error {
	return cc.Manager.RejectCall()
}

// Hangup ends the current call.
func (cc *CallingController) Hangup() error {
	return cc.Manager.Hangup()
}

// ToggleMute toggles microphone mute.
func (cc *CallingController) ToggleMute() bool {
	return cc.Manager.ToggleMute()
}

// ToggleScreenShare toggles screen sharing on the active call.
func (cc *CallingController) ToggleScreenShare() error {
	ctx := cc.bridge.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return cc.Manager.ToggleScreenShare(ctx)
}

func (cc *CallingController) handleCallStateChanged(info calling.CallInfo) {
	if info.State == calling.CallStateIncoming {
		callerJID := info.RemoteJID
		name := callerJID
		number := callerJID
		var avatar *gdk.Texture

		if cc.bridge.Contacts != nil {
			avatar = cc.bridge.Contacts.GetAvatar(callerJID)
			if clean := cc.bridge.Contacts.GetCleanName(callerJID); clean != "" {
				name = clean
			}
		}
		if cc.bridge.DB != nil {
			if contact, _ := cc.bridge.DB.GetContact(callerJID); contact != nil {
				if dn := contact.DisplayName(); dn != "" && dn != contact.JID {
					name = dn
				}
			}
		}
		if strings.Contains(callerJID, "@") {
			number = "+" + strings.Split(callerJID, "@")[0]
		}

		cc.activeJID = callerJID
		cc.activeName = name
		cc.activeNumber = number
		cc.activeAvatar = avatar

		// Desktop notification
		if cc.bridge.Notifier != nil {
			callType := "voz"
			if info.IsVideo {
				callType = "vídeo"
			}
			_ = cc.bridge.Notifier.Notify(
				callerJID,
				"Chamada WhatsApp",
				fmt.Sprintf("Chamada de %s recebida de %s", callType, name),
				"call-start-symbolic",
			)
		}

		// Pop up CallWindow
		glib.IdleAdd(func() {
			win := cc.bridge.App.ShowCallWindow()
			win.OnAccept = func() { _ = cc.Manager.AnswerCall() }
			win.OnDecline = func() { _ = cc.Manager.RejectCall() }
			win.OnHangup = func() { _ = cc.Manager.Hangup() }
			win.OnToggleMute = func() { cc.Manager.ToggleMute() }
			win.OnToggleScreenShare = func() { _ = cc.ToggleScreenShare() }
			win.Update(info, name, number, avatar)
		})
	} else {
		// Update CallWindow on UI thread
		glib.IdleAdd(func() {
			if cc.bridge.App.CallWindow != nil {
				cc.bridge.App.CallWindow.OnHangup = func() { _ = cc.Manager.Hangup() }
				cc.bridge.App.CallWindow.OnToggleMute = func() { cc.Manager.ToggleMute() }
				cc.bridge.App.CallWindow.OnToggleScreenShare = func() { _ = cc.ToggleScreenShare() }
				cc.bridge.App.CallWindow.Update(info, cc.activeName, cc.activeNumber, cc.activeAvatar)
			}
		})
	}

	// Dispatch to EventBus
	if cc.bridge.EventBus != nil {
		cc.bridge.EventBus.Publish(events.Event{
			Type: events.EventCallStateChanged,
			Data: info,
		})
	}
}
