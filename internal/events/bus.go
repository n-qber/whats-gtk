package events

import "sync"

// EventType represents the type of an event that can be published or subscribed to.
type EventType string

const (
	EventMessageReceived      EventType = "MessageReceived"
	EventMessageUpdated       EventType = "MessageUpdated"
	EventChatUpdated          EventType = "ChatUpdated"
	EventContactsUpdated      EventType = "ContactsUpdated"
	EventChatMessagesLoaded   EventType = "ChatMessagesLoaded"
	EventLiveMessageReceived  EventType = "LiveMessageReceived"
	EventProfilesUpdated      EventType = "ProfilesUpdated"
	EventActiveProfileChanged EventType = "ActiveProfileChanged"
)

type LiveMessagePayload struct {
	Msg       interface{} // *meowEvents.Message
	IsSyncing bool
}

// Event is a container for an event payload.
type Event struct {
	Type EventType
	Data interface{}
}

// EventBus is a thread-safe pub/sub system for decoupling the backend from the UI.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]chan Event
}

// NewEventBus creates a new EventBus instance.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]chan Event),
	}
}

// Subscribe returns a channel that will receive events of the specified type.
func (b *EventBus) Subscribe(eventType EventType) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 100)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// Publish sends an event to all subscribers of its type.
// It is non-blocking; if a subscriber's channel is full, the event is dropped.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if chans, found := b.subscribers[event.Type]; found {
		for _, ch := range chans {
			select {
			case ch <- event:
			default:
				// Subscriber channel is full, drop event to avoid blocking publishers
			}
		}
	}
}
