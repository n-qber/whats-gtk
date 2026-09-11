package bridge

import (
	"context"
	"time"

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
