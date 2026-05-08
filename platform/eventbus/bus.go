package eventbus

import "context"

type EventType string

type Event struct {
	Type    EventType
	Payload any
}

type HandlerFunc func(ctx context.Context, event Event)

type Bus interface {
	Publish(ctx context.Context, event Event)
	Subscribe(eventType EventType, handler HandlerFunc)
}
