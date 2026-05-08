package core

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

// MessageHook is a function that processes an incoming message.
type MessageHook func(context.Context, *events.Message) error

// MessagePipeline manages the processing of messages through hooks.
type MessagePipeline struct {
	hooks []MessageHook
}

// NewMessagePipeline creates a new MessagePipeline.
func NewMessagePipeline() *MessagePipeline {
	return &MessagePipeline{
		hooks: make([]MessageHook, 0),
	}
}

// AddHook registers a new message hook.
func (p *MessagePipeline) AddHook(hook MessageHook) {
	p.hooks = append(p.hooks, hook)
}

// Process processes a message through all registered hooks.
func (p *MessagePipeline) Process(msg *events.Message) {
	for i, hook := range p.hooks {
		go func(h MessageHook, idx int) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- h(ctx, msg)
			}()

			select {
			case err := <-done:
				if err != nil {
					fmt.Printf("MessagePipeline: Hook #%d returned error: %v\n", idx, err)
				}
			case <-ctx.Done():
				fmt.Printf("MessagePipeline: Hook #%d timed out after 5s\n", idx)
			}
		}(hook, i)
	}
}
