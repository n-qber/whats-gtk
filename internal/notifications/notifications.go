package notifications

import (
	"os"
	"os/exec"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Notifier sends desktop notifications via D-Bus org.freedesktop.Notifications and handles click actions.
type Notifier struct {
	conn         *dbus.Conn
	obj          dbus.BusObject
	onAction     func(chatJID string)
	soundEnabled              bool
	dndEnabled                bool
	groupNotificationsEnabled bool

	mu                sync.Mutex
	chatMap           map[uint32]string
	dndSubs           map[int]func(bool)
	nextDNDSub        int
	soundSubs         map[int]func(bool)
	nextSoundSub      int
	groupNotifSubs    map[int]func(bool)
	nextGroupNotifSub int
}

// NewNotifier creates and connects a D-Bus desktop notification client.
func NewNotifier(onAction func(chatJID string)) *Notifier {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	n := &Notifier{
		conn:         conn,
		obj:          obj,
		onAction:     onAction,
		soundEnabled:              true,
		dndEnabled:                false,
		groupNotificationsEnabled: true,
		chatMap:                   make(map[uint32]string),
		dndSubs:                   make(map[int]func(bool)),
		soundSubs:                 make(map[int]func(bool)),
		groupNotifSubs:            make(map[int]func(bool)),
	}

	if onAction != nil {
		n.listenForActions()
	}

	return n
}

func (n *Notifier) SetSoundEnabled(enabled bool) {
	n.mu.Lock()
	if n.soundEnabled == enabled {
		n.mu.Unlock()
		return
	}
	n.soundEnabled = enabled
	subs := make([]func(bool), 0, len(n.soundSubs))
	for _, fn := range n.soundSubs {
		subs = append(subs, fn)
	}
	n.mu.Unlock()

	for _, fn := range subs {
		fn(enabled)
	}
}

func (n *Notifier) IsSoundEnabled() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.soundEnabled
}

// OnSoundChange registers a callback invoked when sound setting is toggled.
// Returns an unsubscribe function.
func (n *Notifier) OnSoundChange(fn func(bool)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.soundSubs == nil {
		n.soundSubs = make(map[int]func(bool))
	}
	id := n.nextSoundSub
	n.nextSoundSub++
	n.soundSubs[id] = fn

	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.soundSubs, id)
	}
}

func (n *Notifier) SetDNDEnabled(enabled bool) {
	n.mu.Lock()
	if n.dndEnabled == enabled {
		n.mu.Unlock()
		return
	}
	n.dndEnabled = enabled
	subs := make([]func(bool), 0, len(n.dndSubs))
	for _, fn := range n.dndSubs {
		subs = append(subs, fn)
	}
	n.mu.Unlock()

	for _, fn := range subs {
		fn(enabled)
	}
}

func (n *Notifier) IsDNDEnabled() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.dndEnabled
}

// OnDNDChange registers a callback invoked when DND mode is toggled.
// Returns an unsubscribe function.
func (n *Notifier) OnDNDChange(fn func(bool)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.dndSubs == nil {
		n.dndSubs = make(map[int]func(bool))
	}
	id := n.nextDNDSub
	n.nextDNDSub++
	n.dndSubs[id] = fn

	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.dndSubs, id)
	}
}

func (n *Notifier) SetGroupNotificationsEnabled(enabled bool) {
	n.mu.Lock()
	if n.groupNotificationsEnabled == enabled {
		n.mu.Unlock()
		return
	}
	n.groupNotificationsEnabled = enabled
	subs := make([]func(bool), 0, len(n.groupNotifSubs))
	for _, fn := range n.groupNotifSubs {
		subs = append(subs, fn)
	}
	n.mu.Unlock()

	for _, fn := range subs {
		fn(enabled)
	}
}

func (n *Notifier) IsGroupNotificationsEnabled() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.groupNotificationsEnabled
}

// OnGroupNotificationsChange registers a callback invoked when group notifications are toggled.
// Returns an unsubscribe function.
func (n *Notifier) OnGroupNotificationsChange(fn func(bool)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.groupNotifSubs == nil {
		n.groupNotifSubs = make(map[int]func(bool))
	}
	id := n.nextGroupNotifSub
	n.nextGroupNotifSub++
	n.groupNotifSubs[id] = fn

	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.groupNotifSubs, id)
	}
}


func (n *Notifier) listenForActions() {
	if n.conn == nil {
		return
	}

	rule := "type='signal',interface='org.freedesktop.Notifications',member='ActionInvoked'"
	call := n.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		return
	}

	c := make(chan *dbus.Signal, 20)
	n.conn.Signal(c)

	go func() {
		for sig := range c {
			if sig.Name == "org.freedesktop.Notifications.ActionInvoked" && len(sig.Body) >= 2 {
				id, ok1 := sig.Body[0].(uint32)
				actionKey, ok2 := sig.Body[1].(string)
				if ok1 && ok2 && (actionKey == "default" || actionKey == "open" || actionKey == "app.open") {
					n.mu.Lock()
					chatJID, exists := n.chatMap[id]
					delete(n.chatMap, id)
					n.mu.Unlock()

					if exists && chatJID != "" && n.onAction != nil {
						n.onAction(chatJID)
					}
				}
			}
		}
	}()
}

// Notify sends a desktop notification with title, body, and icon, associating it with a chatJID.
func (n *Notifier) Notify(chatJID, title, body, icon string) error {
	if n == nil || n.obj == nil {
		return nil
	}
	n.mu.Lock()
	dnd := n.dndEnabled
	n.mu.Unlock()
	if dnd {
		return nil
	}
	if icon == "" {
		icon = "dialog-information"
	}

	actions := []string{}
	if chatJID != "" && n.onAction != nil {
		actions = []string{"default", "Abrir"}
	}

	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant("com.github.user.whats-gtk"),
		"sound-name":    dbus.MakeVariant("message-new-instant"),
	}

	var notifID uint32
	call := n.obj.Call(
		"org.freedesktop.Notifications.Notify",
		0,
		"whats-gtk",
		uint32(0),
		icon,
		title,
		body,
		actions,
		hints,
		int32(5000),
	)
	if call.Err != nil {
		return call.Err
	}

	if err := call.Store(&notifID); err == nil && notifID > 0 && chatJID != "" {
		n.mu.Lock()
		if len(n.chatMap) > 100 {
			for k := range n.chatMap {
				delete(n.chatMap, k)
				break
			}
		}
		n.chatMap[notifID] = chatJID
		sound := n.soundEnabled
		n.mu.Unlock()

		if sound {
			PlayNotificationSound()
		}
	}

	return nil
}

// PlayNotificationSound plays a system message alert sound in a separate goroutine.
func PlayNotificationSound() {
	soundPaths := []string{
		"/run/current-system/sw/share/sounds/freedesktop/stereo/message-new-instant.oga",
		"/usr/share/sounds/freedesktop/stereo/message-new-instant.oga",
		"/run/current-system/sw/share/sounds/gnome/default/alerts/glass.ogg",
		"/usr/share/sounds/gnome/default/alerts/glass.ogg",
	}

	var foundPath string
	for _, p := range soundPaths {
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	go func() {
		if err := exec.Command("canberra-gtk-play", "-i", "message-new-instant").Run(); err == nil {
			return
		}
		if foundPath != "" {
			for _, cmdName := range []string{"pw-play", "paplay", "aplay"} {
				if path, err := exec.LookPath(cmdName); err == nil {
					_ = exec.Command(path, foundPath).Run()
					return
				}
			}
		}
	}()
}
