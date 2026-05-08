package domain

import (
	"time"

	"github.com/google/uuid"
)

type EmissionFactor struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"      json:"id"`
	ActivityType  string     `gorm:"not null"                   json:"activity_type"`
	TransportMode *string    `json:"transport_mode,omitempty"`
	VehicleType   *string    `json:"vehicle_type,omitempty"`
	FuelType      *string    `json:"fuel_type,omitempty"`
	Region        string     `gorm:"not null;default:'global'"  json:"region"`
	KgCO2ePerKM   *float64   `gorm:"type:numeric(12,6)"         json:"kg_co2e_per_km,omitempty"`
	KgCO2ePerKWH  *float64   `gorm:"type:numeric(12,6)"         json:"kg_co2e_per_kwh,omitempty"`
	KgCO2eFlat    *float64   `gorm:"type:numeric(12,6)"         json:"kg_co2e_flat,omitempty"`
	UnitLabel     *string    `json:"unit_label,omitempty"`
	SourceName    string     `gorm:"not null"                   json:"source_name"`
	SourceURL     *string    `json:"source_url,omitempty"`
	EffectiveFrom time.Time  `gorm:"type:date;not null"         json:"effective_from"`
	EffectiveUntil *time.Time `gorm:"type:date"                 json:"effective_until,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (EmissionFactor) TableName() string { return "emission_factors" }

type Emission struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"      json:"id"`
	ActivityID        uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"activity_id"`
	UserID            uuid.UUID `gorm:"type:uuid;not null"         json:"user_id"`
	EmissionFactorID  uuid.UUID `gorm:"type:uuid;not null"         json:"emission_factor_id"`
	KgCO2e            float64   `gorm:"type:numeric(12,6);not null" json:"kg_co2e"`
	CalculationVersion int      `gorm:"not null;default:1"         json:"calculation_version"`
	CalculatedAt      time.Time `gorm:"not null"                   json:"calculated_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (Emission) TableName() string { return "emissions" }
