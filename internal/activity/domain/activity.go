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
	SourceGPS        ActivitySource = "gps"
	SourceGPSReceipt ActivitySource = "gps+receipt" // GPS trip enriched with a matched receipt
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
	EnergyKWH       *float64       `gorm:"type:numeric(10,3)"         json:"energy_kwh,omitempty"`
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
	// Dedup fields — populated by GPS tracking and/or receipt ingestion.
	Origin      *string `json:"origin,omitempty"`
	Destination *string `json:"destination,omitempty"`
	// DisplayOrigin/DisplayDestination are short, human-friendly addresses
	// (e.g. "Kaggadasapura, Bengaluru") resolved from coordinates via Google
	// Places. Clients should prefer these over Origin/Destination and fall
	// back when nil (no coords, resolver unavailable, or not yet resolved).
	DisplayOrigin      *string   `json:"display_origin,omitempty"`
	DisplayDestination *string   `json:"display_destination,omitempty"`
	OriginLat          *float64  `gorm:"type:double precision"      json:"origin_lat,omitempty"`
	OriginLng          *float64  `gorm:"type:double precision"      json:"origin_lng,omitempty"`
	DestLat            *float64  `gorm:"type:double precision"      json:"dest_lat,omitempty"`
	DestLng            *float64  `gorm:"type:double precision"      json:"dest_lng,omitempty"`
	ReceiptID          *string   `json:"receipt_id,omitempty"`
	FareAmount         *float64  `gorm:"type:numeric(10,2)"         json:"fare_amount,omitempty"`
	FareCurrency       *string   `json:"fare_currency,omitempty"`
	MatchConfidence    *float64  `gorm:"type:numeric(4,3)"          json:"match_confidence,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// KgCO2e is populated via a LEFT JOIN with the emissions table on read.
	// It is not a column on activities and must never be written by GORM.
	KgCO2e *float64 `gorm:"<-:false;column:kg_co2e" json:"kg_co2e,omitempty"`
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
