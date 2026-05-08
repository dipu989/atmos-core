package domain

import (
	"time"

	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/google/uuid"
)

const EventActivityIngested eventbus.EventType = "activity.ingested"

type ActivityIngestedPayload struct {
	ActivityID    uuid.UUID
	UserID        uuid.UUID
	ActivityType  ActivityType
	TransportMode *TransportMode
	DistanceKM    *float64
	StartedAt     time.Time
	DateLocal     time.Time
	RawMetadata   RawMetadata
}
