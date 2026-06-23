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
	Source          string         `json:"source"           validate:"required,oneof=manual gps uber ola rapido namma_yatri gmail health_kit"`
	Metadata        map[string]any `json:"metadata"`
	StartedAt       time.Time      `json:"started_at"       validate:"required"`
	EndedAt         *time.Time     `json:"ended_at"`
	// IdempotencyKey is optional. When omitted the server derives one from stable fields.
	IdempotencyKey string `json:"idempotency_key"`
	// Dedup fields — GPS coordinates for trip origin/destination.
	OriginLat    *float64 `json:"origin_lat"    validate:"omitempty,min=-90,max=90"`
	OriginLng    *float64 `json:"origin_lng"    validate:"omitempty,min=-180,max=180"`
	DestLat      *float64 `json:"dest_lat"      validate:"omitempty,min=-90,max=90"`
	DestLng      *float64 `json:"dest_lng"      validate:"omitempty,min=-180,max=180"`
	FareAmount   *float64 `json:"fare_amount"   validate:"omitempty,gt=0"`
	FareCurrency *string  `json:"fare_currency"`
}

// UpdateActivityRequest is the body for PATCH /activities/:id.
// All fields are optional — only provided fields are changed.
// Source and provider cannot be changed after ingestion.
type UpdateActivityRequest struct {
	TransportMode   *string    `json:"transport_mode"   validate:"omitempty,oneof=walking cycling metro train car cab flight bus walk bicycle auto_rickshaw two_wheeler"`
	DistanceKM      *float64   `json:"distance_km"      validate:"omitempty,gt=0"`
	DurationMinutes *int       `json:"duration_minutes" validate:"omitempty,min=1"`
	StartedAt       *time.Time `json:"started_at"`
}

// ActivitiesPage wraps a paginated list of activities. Deliberately omits
// per-item ImpactContext — computing it requires an extra alternative-factor
// lookup per activity (see EmissionService.ComputeImpactContext), which is
// only worth paying on the single-activity detail view, not for every row
// of a list/export response. Clients fetch GET /activities/:id for impact data.
type ActivitiesPage struct {
	Activities []domain.Activity `json:"activities"`
	Total      int64             `json:"total"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}

// ImpactContext translates an activity's kg CO2e into relatable comparisons.
// TreesNeededToOffset, LedHoursEquivalent, and GlobalAveragePct are derived
// from published reference figures (EPA, Our World in Data, CEA) that vary by
// region and climate — Approximate is always true and clients must surface
// that as a "global approximation" disclosure, not a precise measurement.
type ImpactContext struct {
	TreesNeededToOffset int  `json:"trees_needed_to_offset"`
	LedHoursEquivalent  int  `json:"led_hours_equivalent"`
	GlobalAveragePct    int  `json:"global_average_pct"`
	Approximate         bool `json:"approximate"`

	// AlternativeMode is the greenest readily-available substitute transport
	// mode, omitted when the activity's mode is already zero-emission or no
	// alternative is actually greener for this trip's distance.
	AlternativeMode   *domain.TransportMode `json:"alternative_mode,omitempty"`
	AlternativeKgCO2e *float64              `json:"alternative_kg_co2e,omitempty"`
	SavingsKgCO2e     *float64              `json:"savings_kg_co2e,omitempty"`
	SavingsPct        *int                  `json:"savings_pct,omitempty"`
}

// ActivityDetailResponse wraps a single activity with its derived impact
// context. Used by GET /activities/:id.
type ActivityDetailResponse struct {
	domain.Activity
	Impact ImpactContext `json:"impact"`
}
