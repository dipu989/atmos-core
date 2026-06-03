package domain

import "github.com/dipu/atmos-core/platform/eventbus"

const EventInsightCreated eventbus.EventType = "insight.created"

// InsightCreatedPayload is published by the Engine each time a new insight is persisted.
type InsightCreatedPayload struct {
	Insight *Insight
}
