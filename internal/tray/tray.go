package tray

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
)

// Setup initializes the system tray icon, menu items, and starts the background
// D-Bus watcher that keeps the tray item registered across bar/compositor restarts.
func Setup(ctx context.Context, iconData []byte, onToggle func(), onQuit func()) {
	go func() {
		systray.Run(func() {
			if len(iconData) > 0 {
				systray.SetIcon(iconData)
			}
			systray.SetTitle("WhatsApp")
			systray.SetTooltip("WhatsApp GTK")

			if onToggle != nil {
				systray.SetOnTapped(onToggle)
			}

			mToggle := systray.AddMenuItem("Show/Hide", "Toggle WhatsApp GTK Window")
			mQuit := systray.AddMenuItem("Quit", "Quit WhatsApp GTK")

			// Start the StatusNotifierWatcher auto-reconnection monitor
			go watchStatusNotifier(ctx)

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-mToggle.ClickedCh:
						if onToggle != nil {
							onToggle()
						}
					case <-mQuit.ClickedCh:
						if onQuit != nil {
							onQuit()
						}
						systray.Quit()
						return
					}
				}
			}()
		}, func() {})
	}()
}

func watchStatusNotifier(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			log.Printf("[tray] Failed to connect to D-Bus session bus: %v (retrying in 5s)", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Subscribe to signals:
		// 1. StatusNotifierHostRegistered: emitted when a host (Waybar, KDE panel, etc.) starts/refreshes.
		_ = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
			"type='signal',interface='org.kde.StatusNotifierWatcher',member='StatusNotifierHostRegistered'")

		// 2. NameOwnerChanged for org.kde.StatusNotifierWatcher: emitted when the watcher daemon restarts.
		_ = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
			"type='signal',interface='org.freedesktop.DBus',member='NameOwnerChanged',arg0='org.kde.StatusNotifierWatcher'")

		sigChan := make(chan *dbus.Signal, 20)
		conn.Signal(sigChan)

		var debounceMu sync.Mutex
		var debounceTimer *time.Timer

		triggerReRegistration := func(reason string) {
			debounceMu.Lock()
			defer debounceMu.Unlock()

			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			debounceTimer = time.AfterFunc(150*time.Millisecond, func() {
				log.Printf("[tray] %s, re-registering tray item...", reason)
				_ = registerStatusNotifierItem(conn)
			})
		}

		runLoop := true
		for runLoop {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case sig, ok := <-sigChan:
				if !ok || sig == nil {
					runLoop = false
					break
				}

				switch sig.Name {
				case "org.kde.StatusNotifierWatcher.StatusNotifierHostRegistered":
					triggerReRegistration("StatusNotifierHostRegistered signal received")
				case "org.freedesktop.DBus.NameOwnerChanged":
					if len(sig.Body) >= 3 {
						if newOwner, ok := sig.Body[2].(string); ok && newOwner != "" {
							triggerReRegistration(fmt.Sprintf("StatusNotifierWatcher acquired by %s", newOwner))
						}
					}
				}
			}
		}

		conn.Close()
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func registerStatusNotifierItem(conn *dbus.Conn) error {
	serviceName := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	obj := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")

	call := obj.Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, serviceName)
	if call.Err != nil {
		return call.Err
	}
	return nil
}
