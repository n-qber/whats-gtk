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
		return nil, fmt.Errorf("no device found in store. please pair first")
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	client := whatsmeow.NewClient(device, clientLog)

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
