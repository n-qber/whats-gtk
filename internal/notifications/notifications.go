package notifications

import (
	"github.com/godbus/dbus/v5"
)

// Notifier sends desktop notifications via D-Bus org.freedesktop.Notifications.
type Notifier struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

// NewNotifier creates and connects a D-Bus desktop notification client.
func NewNotifier() *Notifier {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return &Notifier{
		conn: conn,
		obj:  obj,
	}
}

// Notify sends a desktop notification with title, body, and icon.
func (n *Notifier) Notify(title, body, icon string) error {
	if n == nil || n.obj == nil {
		return nil
	}
	if icon == "" {
		icon = "dialog-information"
	}
	call := n.obj.Call(
		"org.freedesktop.Notifications.Notify",
		0,
		"whats-gtk",
		uint32(0),
		icon,
		title,
		body,
		[]string{},
		map[string]dbus.Variant{},
		int32(5000),
	)
	return call.Err
}
