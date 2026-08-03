package main

import (
	"context"
	"log"
	_ "embed"
	"os"

	"whats-gtk/internal/backend"
	"whats-gtk/internal/bridge"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"fyne.io/systray"
)

//go:embed assets/icon.png
var iconData []byte

func main() {
	const appID = "com.github.user.whats-gtk"
	application := adw.NewApplication(appID, gio.ApplicationFlagsNone)

	ctx := context.Background()

	var initialized bool
	var mainApp *ui.App

	application.Connect("activate", func() {
		if initialized {
			if mainApp != nil {
				mainApp.Window.Present()
			}
			return
		}
		initialized = true

		// Hold application count so GTK main loop stays alive when window is hidden
		application.Hold()

		// Initialize AppDB
		appDB, err := database.InitDB("app.db")
		if err != nil {
			log.Fatal("Failed to init app db:", err)
		}

		// Initialize Backend
		container, err := backend.InitStore(ctx, "store.db")
		if err != nil {
			log.Fatal("Failed to init store:", err)
		}

		b, err := backend.NewBackend(ctx, container)
		if err != nil {
			log.Fatal("Failed to create backend:", err)
		}

		// Initialize EventBus
		bus := events.NewEventBus()

		// Initialize UI
		app, err := ui.NewApp(application, bus)
		if err != nil {
			log.Fatal("Failed to create app UI:", err)
		}
		mainApp = app

		// Initialize Bridge
		br := bridge.NewBridge(b, app, appDB, ctx, bus)
		br.Start(ctx)

		setupTray(app, application)

		app.Show()
	})

	os.Exit(application.Run(os.Args))
}

func setupTray(app *ui.App, application *adw.Application) {
	go func() {
		systray.Run(func() {
			systray.SetIcon(iconData)
			systray.SetTitle("WhatsApp")
			systray.SetTooltip("WhatsApp GTK")

			toggleFunc := func() {
				glib.IdleAdd(func() {
					if app.Window.IsVisible() {
						app.Window.Hide()
					} else {
						app.Window.Present()
					}
				})
			}

			systray.SetOnTapped(toggleFunc)

			mToggle := systray.AddMenuItem("Show/Hide", "Toggle WhatsApp GTK Window")
			mQuit := systray.AddMenuItem("Quit", "Quit WhatsApp GTK")

			go func() {
				for {
					select {
					case <-mToggle.ClickedCh:
						toggleFunc()
					case <-mQuit.ClickedCh:
						glib.IdleAdd(func() {
							application.Release()
							application.Quit()
						})
						systray.Quit()
						os.Exit(0)
					}
				}
			}()
		}, func() {})
	}()
}

