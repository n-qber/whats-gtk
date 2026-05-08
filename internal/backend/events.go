package backend

import (
	"go.mau.fi/whatsmeow/types/events"
)

// AppEvent is a generic interface for all events passed from backend to UI
type AppEvent interface{}

type ConnectedEvent struct{}
type DisconnectedEvent struct{}

type QREvent struct {
	Code string
}

type MessageEvent struct {
	Info *events.Message
}

type HistorySyncEvent struct {
	Data *events.HistorySync
}

type ReceiptEvent struct {
	Info *events.Receipt
}

type PresenceEvent struct {
	Info *events.Presence
}

type OfflineSyncCompletedEvent struct{}

type IdentityChangeEvent struct {
	Info *events.IdentityChange
}

type PushNameEvent struct {
	Info *events.PushName
}

type ContactEvent struct {
	Info *events.Contact
}

type MediaRetryEvent struct {
	Info *events.MediaRetry
}

func (b *Backend) registerEventHandlers() {
	b.Client.AddEventHandler(func(evt interface{}) {
		var appEvt AppEvent

		switch v := evt.(type) {
		case *events.Connected:
			appEvt = &ConnectedEvent{}
		case *events.Disconnected:
			appEvt = &DisconnectedEvent{}
		case *events.QR:
			appEvt = &QREvent{Code: v.Codes[0]}
		case *events.PushName:
			appEvt = &PushNameEvent{Info: v}
		case *events.Contact:
			appEvt = &ContactEvent{Info: v}
		case *events.Message:
			appEvt = &MessageEvent{Info: v}
		case *events.HistorySync:
			appEvt = &HistorySyncEvent{Data: v}
		case *events.Receipt:
			appEvt = &ReceiptEvent{Info: v}
		case *events.Presence:
			appEvt = &PresenceEvent{Info: v}
		case *events.OfflineSyncCompleted:
			appEvt = &OfflineSyncCompletedEvent{}
		case *events.IdentityChange:
			appEvt = &IdentityChangeEvent{Info: v}
		case *events.MediaRetry:
			appEvt = &MediaRetryEvent{Info: v}
		default:
			if v != nil {
				// Log the type for debugging
				// fmt.Printf("Backend: Received unhandled event: %T\n", v)
			}
			appEvt = evt
		}

		if b.eventHandler != nil {
			b.eventHandler(appEvt)
		}
	})
}
