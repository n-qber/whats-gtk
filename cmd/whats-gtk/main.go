package main

import (
	"context"
	"log"
	_ "embed"
	"os"
	"os/signal"
	"syscall"

	"whats-gtk/internal/backend"
	"whats-gtk/internal/bridge"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/paths"
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
	application := adw.NewApplication(appID, gio.ApplicationHandlesOpen)

	ctx := context.Background()

	var initialized bool
	var mainApp *ui.App
	var mainBridge *bridge.Bridge
	var currentAppDB *database.AppDB

	shutdown := func() {
		if currentAppDB != nil {
			log.Println("Closing app database cleanly...")
			_ = currentAppDB.Close()
			currentAppDB = nil
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		shutdown()
		os.Exit(0)
	}()

	initApp := func() {
		if initialized {
			return
		}
		initialized = true

		// Hold application count so GTK main loop stays alive when window is hidden
		application.Hold()

		appDBPath := paths.AppDBPath()
		storeDBPath := paths.StoreDBPath()

		// Resolve any pending Syncthing conflicts dynamically before opening databases
		if err := database.ResolveSyncthingConflicts(appDBPath); err != nil {
			log.Printf("Warning: error resolving app.db conflicts: %v", err)
		}
		if err := database.ResolveSyncthingConflicts(storeDBPath); err != nil {
			log.Printf("Warning: error resolving store.db conflicts: %v", err)
		}
		database.CleanStaleConflictFiles(paths.DataDir())
		database.CleanStaleConflictFiles(".")

		// Initialize AppDB
		appDB, err := database.InitDB(appDBPath)
		if err != nil {
			log.Fatal("Failed to init app db:", err)
		}
		currentAppDB = appDB

		// Initialize Backend
		container, err := backend.InitStore(ctx, storeDBPath)
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
		mainBridge = br
		br.Start(ctx)

		setupTray(app, application, shutdown)

		app.Show()
	}

	handleArgs := func() {
		for _, arg := range os.Args[1:] {
			if _, ok := bridge.ParseWhatsAppURL(arg); ok {
				if mainBridge != nil {
					mainBridge.Chat.HandleOpenURL(arg)
				}
				break
			}
		}
	}

	application.Connect("activate", func() {
		initApp()
		if mainApp != nil {
			mainApp.Window.Present()
		}
		handleArgs()
	})

	application.Connect("open", func() {
		initApp()
		if mainApp != nil {
			mainApp.Window.Present()
		}
		handleArgs()
	})

	code := application.Run(os.Args)
	shutdown()
	os.Exit(code)
}

func setupTray(app *ui.App, application *adw.Application, shutdown func()) {
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
						shutdown()
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

