package domain

import (
	"time"

	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/google/uuid"
)

const (
	EventActivityIngested          eventbus.EventType = "activity.ingested"
	EventActivityPossibleDuplicate eventbus.EventType = "activity.possible_duplicate"
)

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

// ActivityPossibleDuplicatePayload is published when a receipt or GPS trip scores
// in the review range (0.45–0.65) against an existing activity. The notification
// service uses this to prompt the user to review the potential duplicate.
type ActivityPossibleDuplicatePayload struct {
	ActivityID      uuid.UUID
	UserID          uuid.UUID
	MatchConfidence float64
	StartedAt       time.Time
	UserTimezone    string // IANA tz string; empty falls back to UTC
}
