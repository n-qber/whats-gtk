package bridge

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// DefaultStandbyTimeout is how long the app waits without any UI activity
// before entering standby.
const DefaultStandbyTimeout = 5 * time.Minute

// pendingRead is a read receipt that was deferred because the app was in
// standby. It is flushed as soon as the user becomes active again.
type pendingRead struct {
	chat      types.JID
	sender    types.JID
	ids       []string
	timestamp time.Time
}

// LoadStandbySettings reads the persisted standby preferences. Standby is
// enabled by default and uses DefaultStandbyTimeout when no value is stored.
func (cc *ChatController) LoadStandbySettings() {
	enabled := true
	minutes := int(DefaultStandbyTimeout / time.Minute)

	if cc.DB != nil {
		if val, err := cc.DB.GetSetting("standby_enabled"); err == nil && val != "" {
			enabled = val != "false"
		}
		if val, err := cc.DB.GetSetting("standby_timeout_minutes"); err == nil && val != "" {
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				minutes = n
			}
		}
	}

	cc.standbyMu.Lock()
	cc.standbyEnabled = enabled
	cc.standbyTimeout = time.Duration(minutes) * time.Minute
	cc.standbyMu.Unlock()
}

// StartStandbyWatcher starts the goroutine that checks for user inactivity.
// It must be called once after the UI activity callback has been wired.
func (cc *ChatController) StartStandbyWatcher() {
	if cc.ctx == nil {
		return
	}

	cc.HandleUserActivity()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-cc.ctx.Done():
				return
			case <-ticker.C:
				cc.checkStandby()
			}
		}
	}()
}

// SetStandbyEnabled enables or disables the standby behaviour at runtime.
func (cc *ChatController) SetStandbyEnabled(enabled bool) {
	cc.standbyMu.Lock()
	cc.standbyEnabled = enabled
	cc.lastActivity = time.Now()
	cc.standbyMu.Unlock()

	if !enabled {
		cc.leaveStandby()
	}
}

// SetStandbyTimeout changes how long the app waits before entering standby.
func (cc *ChatController) SetStandbyTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultStandbyTimeout
	}
	cc.standbyMu.Lock()
	cc.standbyTimeout = timeout
	cc.lastActivity = time.Now()
	cc.standbyMu.Unlock()
}

// IsInStandby reports whether the app is currently idle and suppressing ACKs.
func (cc *ChatController) IsInStandby() bool {
	cc.standbyMu.Lock()
	defer cc.standbyMu.Unlock()
	return cc.standbyActive
}

// IsStandbyEnabled reports whether standby is currently enabled.
func (cc *ChatController) IsStandbyEnabled() bool {
	cc.standbyMu.Lock()
	defer cc.standbyMu.Unlock()
	return cc.standbyEnabled
}

// StandbyTimeoutMinutes returns the configured standby timeout in minutes.
func (cc *ChatController) StandbyTimeoutMinutes() int {
	cc.standbyMu.Lock()
	defer cc.standbyMu.Unlock()
	minutes := int(cc.standbyTimeout / time.Minute)
	if minutes <= 0 {
		minutes = int(DefaultStandbyTimeout / time.Minute)
	}
	return minutes
}

// HandleUserActivity is called by the UI whenever the user moves the pointer,
// clicks, scrolls, types or focuses a chat window. It resets the inactivity
// timer and wakes the app up from standby when necessary.
func (cc *ChatController) HandleUserActivity() {
	active := cc.appWindowActive()

	cc.standbyMu.Lock()
	cc.windowActive = active
	if active {
		cc.lastActivity = time.Now()
	}
	wasStandby := cc.standbyActive
	cc.standbyMu.Unlock()

	if active && wasStandby {
		cc.leaveStandby()
	}
}

// MarkRead sends a read receipt unless the app is in standby. While in standby
// the receipt is queued and sent when the user becomes active again, so the
// sender does not see blue ticks while nobody is actually looking at the chat.
func (cc *ChatController) MarkRead(jid types.JID, ids []string, sender types.JID, timestamp time.Time) {
	if cc.Backend == nil || len(ids) == 0 {
		return
	}

	cc.standbyMu.Lock()
	if cc.standbyActive {
		copied := append([]string(nil), ids...)
		cc.pendingReads = append(cc.pendingReads, pendingRead{
			chat:      jid,
			sender:    sender,
			ids:       copied,
			timestamp: timestamp,
		})
		cc.standbyMu.Unlock()
		return
	}
	cc.standbyMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cc.Backend.MarkRead(ctx, jid, ids, sender, timestamp); err != nil {
			fmt.Printf("[Standby] Falha ao enviar confirmação de leitura: %v\n", err)
		}
	}()
}

// SyncPresenceAfterConnect restores the correct presence state after a
// reconnection. If the app was idle, it stays unavailable instead of becoming
// online again on its own.
func (cc *ChatController) SyncPresenceAfterConnect() {
	if cc.Backend == nil {
		return
	}

	cc.standbyMu.Lock()
	active := cc.standbyActive
	cc.standbyMu.Unlock()

	state := types.PresenceAvailable
	if active {
		state = types.PresenceUnavailable
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cc.Backend.SendPresence(ctx, state)
	}()
}

// checkStandby enters standby once the configured inactivity timeout expires.
func (cc *ChatController) checkStandby() {
	cc.standbyMu.Lock()
	enabled := cc.standbyEnabled
	active := cc.standbyActive
	windowActive := cc.windowActive
	timeout := cc.standbyTimeout
	last := cc.lastActivity
	cc.standbyMu.Unlock()

	if !enabled || active || !windowActive || timeout <= 0 || last.IsZero() {
		return
	}

	if time.Since(last) >= timeout {
		cc.enterStandby()
	}
}

// enterStandby marks the app as unavailable and stops sending read receipts.
func (cc *ChatController) enterStandby() {
	cc.standbyMu.Lock()
	if !cc.standbyEnabled || cc.standbyActive {
		cc.standbyMu.Unlock()
		return
	}
	cc.standbyActive = true
	cc.standbyMu.Unlock()

	cc.StopAllTyping()

	fmt.Println("[Standby] Usuário inativo; entrando em standby (sem ACKs)")

	if cc.Backend != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cc.Backend.SendPresence(ctx, types.PresenceUnavailable); err != nil {
				fmt.Printf("[Standby] Falha ao enviar presença indisponível: %v\n", err)
			}
		}()
	}
}

// leaveStandby marks the app as available again and flushes the read receipts
// that were deferred while the user was idle.
func (cc *ChatController) leaveStandby() {
	cc.standbyMu.Lock()
	if !cc.standbyActive {
		cc.standbyMu.Unlock()
		return
	}
	cc.standbyActive = false
	cc.standbyMu.Unlock()

	fmt.Println("[Standby] Atividade detectada; saindo do standby")

	go func() {
		if cc.Backend != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = cc.Backend.SendPresence(ctx, types.PresenceAvailable)
			cancel()
		}
		cc.flushPendingReads()
	}()
}

// flushPendingReads groups the deferred receipts by chat and sender and sends
// them once the app is active again.
func (cc *ChatController) flushPendingReads() {
	cc.standbyMu.Lock()
	pending := cc.pendingReads
	cc.pendingReads = nil
	cc.standbyMu.Unlock()

	if len(pending) == 0 || cc.Backend == nil {
		return
	}

	for _, group := range groupPendingReads(pending) {
		go func(item *pendingRead) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := cc.Backend.MarkRead(ctx, item.chat, item.ids, item.sender, item.timestamp); err != nil {
				fmt.Printf("[Standby] Falha ao enviar confirmações pendentes: %v\n", err)
			}
		}(group)
	}
}

// groupPendingReads merges deferred receipts that target the same chat and
// sender, preserving the newest timestamp so WhatsApp receives one receipt
// per conversation instead of one per message.
func groupPendingReads(pending []pendingRead) []*pendingRead {
	type groupKey struct {
		chat   string
		sender string
	}

	groups := make(map[groupKey]*pendingRead)
	order := make([]groupKey, 0, len(pending))

	for _, item := range pending {
		key := groupKey{
			chat:   item.chat.ToNonAD().String(),
			sender: item.sender.ToNonAD().String(),
		}
		group, ok := groups[key]
		if !ok {
			group = &pendingRead{
				chat:      item.chat,
				sender:    item.sender,
				timestamp: item.timestamp,
			}
			groups[key] = group
			order = append(order, key)
		}
		group.ids = append(group.ids, item.ids...)
		if item.timestamp.After(group.timestamp) {
			group.timestamp = item.timestamp
		}
	}

	result := make([]*pendingRead, 0, len(order))
	for _, key := range order {
		result = append(result, groups[key])
	}
	return result
}

// appWindowActive reports whether any chat window is currently focused.
func (cc *ChatController) appWindowActive() bool {
	if cc.App == nil {
		return false
	}
	if cc.App.Window != nil && cc.App.Window.IsActive() {
		return true
	}
	for _, win := range cc.App.DetachedWindows {
		if win != nil && win.IsActive() {
			return true
		}
	}
	return false
}
