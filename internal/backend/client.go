package backend

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Backend struct {
	Client       *whatsmeow.Client
	Store        *sqlstore.Container
	Device       *store.Device
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

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(device, clientLog)

	// Enable all other loggers to stdout
	client.Log = clientLog
	// We don't have direct access to all sub-loggers easily in NewClient, 
	// but setting client.Log usually propagates or covers the main ones.
	// Actually, whatsmeow uses separate loggers. Let's try to set them if possible.

	b := &Backend{
		Client: client,
		Store:  container,
		Device: device,
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
