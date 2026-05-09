package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type ActivityType string
type TransportMode string
type ActivitySource string
type ActivityStatus string

const (
	ActivityTransport ActivityType = "transport"
	ActivityFlight    ActivityType = "flight"
	ActivityEnergy    ActivityType = "energy"
	ActivityFood      ActivityType = "food"

	ModeCAB          TransportMode = "cab"
	ModeCar          TransportMode = "car"
	ModeAutoRickshaw TransportMode = "auto_rickshaw"
	ModeBus          TransportMode = "bus"
	ModeMetro        TransportMode = "metro"
	ModeTrain        TransportMode = "train"
	ModeTwoWheeler   TransportMode = "two_wheeler"
	ModeWalk         TransportMode = "walk" // legacy; prefer ModeWalking
	ModeWalking      TransportMode = "walking"
	ModeBicycle      TransportMode = "bicycle" // legacy; prefer ModeCycling
	ModeCycling      TransportMode = "cycling"
	ModeFlight       TransportMode = "flight"

	SourceManual     ActivitySource = "manual"
	SourceUber       ActivitySource = "uber"
	SourceOla        ActivitySource = "ola"
	SourceRapido     ActivitySource = "rapido"
	SourceNammaYatri ActivitySource = "namma_yatri"
	SourceGmail      ActivitySource = "gmail"
	SourceHealth     ActivitySource = "health_kit"

	StatusPending   ActivityStatus = "pending"
	StatusProcessed ActivityStatus = "processed"
	StatusFailed    ActivityStatus = "failed"
	StatusSkipped   ActivityStatus = "skipped"
)

type Activity struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey"      json:"id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null"         json:"user_id"`
	DeviceID        *uuid.UUID     `gorm:"type:uuid"                  json:"device_id,omitempty"`
	ActivityType    ActivityType   `gorm:"not null"                   json:"activity_type"`
	TransportMode   *TransportMode `json:"transport_mode,omitempty"`
	DistanceKM      *float64       `gorm:"type:numeric(10,3)"         json:"distance_km,omitempty"`
	DurationMinutes *int           `json:"duration_minutes,omitempty"`
	Source          ActivitySource `gorm:"not null"                   json:"source"`
	Provider        *string        `json:"provider,omitempty"`
	RawMetadata     RawMetadata    `gorm:"type:jsonb;not null;default:'{}'" json:"raw_metadata"`
	StartedAt       time.Time      `gorm:"not null"                   json:"started_at"`
	EndedAt         *time.Time     `json:"ended_at,omitempty"`
	DateLocal       time.Time      `gorm:"type:date;not null"         json:"date_local"`
	IdempotencyKey  string         `gorm:"uniqueIndex;not null"       json:"-"`
	Status          ActivityStatus `gorm:"not null;default:'pending'" json:"status"`
	FailureReason   *string        `json:"failure_reason,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (Activity) TableName() string { return "activities" }

// RawMetadata is a flexible JSONB bag for provider-specific fields.
type RawMetadata map[string]any

func (r RawMetadata) Value() (driver.Value, error) {
	if r == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(r)
}

func (r *RawMetadata) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("RawMetadata: expected []byte from db")
	}
	return json.Unmarshal(bytes, r)
}
