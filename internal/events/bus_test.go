package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEventBusPubSub(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	ch := bus.Subscribe(EventMessageReceived)

	bus.Publish(Event{
		Type: EventMessageReceived,
		Data: "test message",
	})

	select {
	case ev := <-ch:
		if ev.Data != "test message" {
			t.Fatalf("Expected 'test message', got %v", ev.Data)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for event")
	}

	// Test Unsubscribe
	bus.Unsubscribe(EventMessageReceived, ch)
	bus.Publish(Event{
		Type: EventMessageReceived,
		Data: "another message",
	})

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("Expected channel to be closed after Unsubscribe")
	}
}

func TestSubscribeTypedCtxCancellation(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var received string
	wg.Add(1)

	SubscribeTypedCtx(ctx, bus, EventMessageReceived, func(payload string) {
		received = payload
		wg.Done()
	})

	bus.Publish(Event{
		Type: EventMessageReceived,
		Data: "typed payload",
	})

	wg.Wait()
	if received != "typed payload" {
		t.Fatalf("Expected 'typed payload', got %q", received)
	}

	// Cancel context to stop goroutine and unsubscribe
	cancel()
	time.Sleep(50 * time.Millisecond)

	bus.mu.RLock()
	subsCount := len(bus.subscribers[EventMessageReceived])
	bus.mu.RUnlock()

	if subsCount != 0 {
		t.Fatalf("Expected 0 subscribers after ctx cancel, got %d", subsCount)
	}
}
