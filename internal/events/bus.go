package events

import (
	"context"
	"sync"
)

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

// Unsubscribe removes a subscription channel and closes it.
func (b *EventBus) Unsubscribe(eventType EventType, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, found := b.subscribers[eventType]
	if !found {
		return
	}

	for i, sub := range subs {
		if sub == ch {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}
}

// Close closes all subscriber channels and clears all subscriptions.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for eventType, chans := range b.subscribers {
		for _, ch := range chans {
			close(ch)
		}
		delete(b.subscribers, eventType)
	}
}

// SubscribeTypedCtx registers a type-safe handler that automatically terminates and unsubscribes when ctx is cancelled.
func SubscribeTypedCtx[T any](ctx context.Context, b *EventBus, eventType EventType, handler func(payload T)) {
	ch := b.Subscribe(eventType)
	go func() {
		defer b.Unsubscribe(eventType, ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if payload, ok := ev.Data.(T); ok {
					handler(payload)
				}
			}
		}
	}()
}

// SubscribeTyped registers a type-safe handler for events of a specific payload type T.
func SubscribeTyped[T any](b *EventBus, eventType EventType, handler func(payload T)) {
	SubscribeTypedCtx(context.Background(), b, eventType, handler)
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
