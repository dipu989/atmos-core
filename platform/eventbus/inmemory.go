package eventbus

import (
	"context"
	"sync"

	"github.com/dipu/atmos-core/platform/logger"
	"go.uber.org/zap"
)

type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]HandlerFunc
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[EventType][]HandlerFunc),
	}
}

func (b *InMemoryBus) Subscribe(eventType EventType, handler HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish dispatches the event to all subscribers asynchronously.
// Each handler runs in its own goroutine so a slow handler does not block ingestion.
func (b *InMemoryBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, h := range handlers {
		h := h
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.L().Error("event handler panicked",
						zap.Any("event_type", event.Type),
						zap.Any("panic", r),
					)
				}
			}()
			h(ctx, event)
		}()
	}
}
