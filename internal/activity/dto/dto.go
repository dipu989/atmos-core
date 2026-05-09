package dto

import (
	"time"

	"github.com/dipu/atmos-core/internal/activity/domain"
)

// IngestActivityRequest is the body for POST /activities.
// activity_type is derived automatically from transport_mode — callers do not set it.
type IngestActivityRequest struct {
	TransportMode   string         `json:"transport_mode"   validate:"required,oneof=walking cycling metro train car cab flight bus walk bicycle auto_rickshaw two_wheeler"`
	DistanceKM      *float64       `json:"distance_km"      validate:"required,gt=0"`
	DurationMinutes *int           `json:"duration_minutes" validate:"omitempty,min=1"`
	Source          string         `json:"source"           validate:"required,oneof=manual uber ola rapido namma_yatri gmail health_kit"`
	Metadata        map[string]any `json:"metadata"`
	StartedAt       time.Time      `json:"started_at"       validate:"required"`
	EndedAt         *time.Time     `json:"ended_at"`
	// IdempotencyKey is optional. When omitted the server derives one from stable fields.
	IdempotencyKey string `json:"idempotency_key"`
}

// ActivitiesPage wraps a paginated list of activities.
type ActivitiesPage struct {
	Activities []domain.Activity `json:"activities"`
	Total      int64             `json:"total"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}
