package backend

import (
	"context"
	"fmt"

	"github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"whats-gtk/internal/logger"
)

type Backend struct {
	Client       *whatsmeow.Client
	Store        *sqlstore.Container
	Device       *store.Device
	CallClient   *meowcaller.Client
	eventHandler func(AppEvent)
}

func NewBackend(ctx context.Context, container *sqlstore.Container) (*Backend, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if device == nil {
		device = container.NewDevice()
	}

	clientLog := logger.WALogger("Client")
	client := whatsmeow.NewClient(device, clientLog)
	client.EmitAppStateEventsOnFullSync = true

	// Enable all other loggers to unified logger
	client.Log = clientLog

	// Initialize VoIP meowcaller client before connection
	callClient := meowcaller.NewClient(client)

	b := &Backend{
		Client:     client,
		Store:      container,
		Device:     device,
		CallClient: callClient,
	}
	b.registerEventHandlers()
	return b, nil
}

func (b *Backend) Connect() error {
	return b.Client.Connect()
}

func (b *Backend) Disconnect() {
	b.Client.Disconnect()
}

func (b *Backend) SetEventHandler(handler func(AppEvent)) {
	b.eventHandler = handler
}

func (b *Backend) Logout(ctx context.Context) error {
	if b.Client == nil {
		return nil
	}
	err := b.Client.Logout(ctx)
	if err != nil {
		b.Client.Disconnect()
		if b.Device != nil {
			_ = b.Device.Delete(ctx)
		}
	}
	return err
}
