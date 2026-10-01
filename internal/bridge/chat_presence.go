package bridge

import (
	"context"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types"
)

type typingState struct {
	timer       *time.Timer
	lastSent    time.Time
	isComposing bool
}

// StartTyping notifies WhatsApp that the user is typing in a chat and starts an inactivity timer.
func (cc *ChatController) StartTyping(jid types.JID) {
	if jid.IsEmpty() {
		return
	}
	cleanJID := jid.ToNonAD()
	jidStr := cleanJID.String()

	cc.typingMu.Lock()
	defer cc.typingMu.Unlock()

	if cc.typingStates == nil {
		cc.typingStates = make(map[string]*typingState)
	}

	state, exists := cc.typingStates[jidStr]
	if !exists {
		state = &typingState{}
		cc.typingStates[jidStr] = state
	}

	now := time.Now()
	shouldSend := false

	if !state.isComposing {
		state.isComposing = true
		state.lastSent = now
		shouldSend = true
	} else if now.Sub(state.lastSent) >= 8*time.Second {
		// Periodically refresh composing lease if user continues typing uninterrupted
		state.lastSent = now
		shouldSend = true
	}

	// Reset 3-second inactivity timer
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(3*time.Second, func() {
		cc.StopTyping(cleanJID)
	})

	if shouldSend && cc.Backend != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cc.Backend.SendChatPresence(ctx, cleanJID, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		}()
	}
}

// StopTyping clears the typing status for the specified chat and sends paused presence.
func (cc *ChatController) StopTyping(jid types.JID) {
	if jid.IsEmpty() {
		return
	}
	cleanJID := jid.ToNonAD()
	jidStr := cleanJID.String()

	cc.typingMu.Lock()
	if cc.typingStates == nil {
		cc.typingMu.Unlock()
		return
	}

	state, exists := cc.typingStates[jidStr]
	if !exists {
		cc.typingMu.Unlock()
		return
	}

	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}

	wasComposing := state.isComposing
	state.isComposing = false
	delete(cc.typingStates, jidStr)
	cc.typingMu.Unlock()

	if wasComposing && cc.Backend != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cc.Backend.SendChatPresence(ctx, cleanJID, types.ChatPresencePaused, types.ChatPresenceMediaText)
		}()
	}
}

// StopAllTyping halts all timers and sends paused presence for every currently active composing chat.
func (cc *ChatController) StopAllTyping() {
	cc.typingMu.Lock()
	if cc.typingStates == nil {
		cc.typingMu.Unlock()
		return
	}

	var toPause []types.JID
	for jidStr, state := range cc.typingStates {
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		if state.isComposing {
			if parsed, err := types.ParseJID(jidStr); err == nil {
				toPause = append(toPause, parsed)
			}
		}
	}
	cc.typingStates = make(map[string]*typingState)
	cc.typingMu.Unlock()

	if cc.Backend != nil {
		for _, jid := range toPause {
			target := jid
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = cc.Backend.SendChatPresence(ctx, target, types.ChatPresencePaused, types.ChatPresenceMediaText)
			}()
		}
	}
}

// HandleIncomingChatPresence handles typing notifications received from other users or groups.
func (cc *ChatController) HandleIncomingChatPresence(chatJID, senderJID types.JID, state types.ChatPresence, media types.ChatPresenceMedia) {
	chatStr := chatJID.ToNonAD().String()

	cc.typingMu.Lock()
	if cc.incomingTypingTimers == nil {
		cc.incomingTypingTimers = make(map[string]*time.Timer)
	}
	if timer, exists := cc.incomingTypingTimers[chatStr]; exists && timer != nil {
		timer.Stop()
		delete(cc.incomingTypingTimers, chatStr)
	}
	cc.typingMu.Unlock()

	if state == types.ChatPresenceComposing {
		action := "digitando..."
		if media == types.ChatPresenceMediaAudio {
			action = "gravando áudio..."
		}

		displayText := action
		if chatJID.Server == types.GroupServer && !senderJID.IsEmpty() {
			senderStr := senderJID.ToNonAD().String()
			name := senderStr
			if contact, err := cc.DB.GetContact(senderStr); err == nil && contact != nil && contact.DisplayName() != "" {
				name = contact.DisplayName()
			}
			displayText = fmt.Sprintf("%s: %s", name, action)
		}

		glib.IdleAdd(func() {
			if cv := cc.App.GetChatViewForJID(chatStr); cv != nil {
				cv.SetTopBarSubtitle(displayText)
			}
			cc.App.Sidebar.SetChatSubtitle(chatStr, fmt.Sprintf("<i>%s</i>", glib.MarkupEscapeText(displayText)))
		})

		// Reset after 4 seconds of inactivity if no explicit paused event is received
		cc.typingMu.Lock()
		cc.incomingTypingTimers[chatStr] = time.AfterFunc(4*time.Second, func() {
			cc.typingMu.Lock()
			delete(cc.incomingTypingTimers, chatStr)
			base := ""
			if cc.incomingPresence != nil {
				base = cc.incomingPresence[chatStr]
			}
			cc.typingMu.Unlock()

			glib.IdleAdd(func() {
				if cv := cc.App.GetChatViewForJID(chatStr); cv != nil {
					cv.SetTopBarSubtitle(base)
				}
				cc.App.Sidebar.SetChatSubtitle(chatStr, "")
			})
		})
		cc.typingMu.Unlock()

	} else if state == types.ChatPresencePaused {
		cc.typingMu.Lock()
		base := ""
		if cc.incomingPresence != nil {
			base = cc.incomingPresence[chatStr]
		}
		cc.typingMu.Unlock()

		glib.IdleAdd(func() {
			if cv := cc.App.GetChatViewForJID(chatStr); cv != nil {
				cv.SetTopBarSubtitle(base)
			}
			cc.App.Sidebar.SetChatSubtitle(chatStr, "")
		})
	}
}

// HandleIncomingPresence updates the online or last-seen status of a contact.
func (cc *ChatController) HandleIncomingPresence(from types.JID, unavailable bool, lastSeen time.Time) {
	fromStr := from.ToNonAD().String()
	var status string
	if !unavailable {
		status = "online"
	} else if !lastSeen.IsZero() {
		status = formatLastSeen(lastSeen)
	}

	cc.typingMu.Lock()
	if cc.incomingPresence == nil {
		cc.incomingPresence = make(map[string]string)
	}
	cc.incomingPresence[fromStr] = status
	_, isTyping := cc.incomingTypingTimers[fromStr]
	cc.typingMu.Unlock()

	if !isTyping {
		glib.IdleAdd(func() {
			if cv := cc.App.GetChatViewForJID(fromStr); cv != nil {
				cv.SetTopBarSubtitle(status)
			}
		})
	}
}

func formatLastSeen(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	if diff < time.Minute {
		return "visto por último agora mesmo"
	}
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	timePart := t.Format("15:04")
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return fmt.Sprintf("visto por último hoje às %s", timePart)
	}
	yesterday := now.AddDate(0, 0, -1)
	y3, m3, d3 := yesterday.Date()
	if y2 == y3 && m2 == m3 && d2 == d3 {
		return fmt.Sprintf("visto por último ontem às %s", timePart)
	}
	return fmt.Sprintf("visto por último em %s às %s", t.Format("02/01/2006"), timePart)
}
