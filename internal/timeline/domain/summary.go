package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ModeBreakdown holds per-transport-mode aggregates stored inside the breakdown JSONB.
type ModeBreakdown struct {
	KgCO2e     float64 `json:"kg_co2e"`
	DistanceKM float64 `json:"distance_km"`
	Count      int     `json:"count"`
}

type Breakdown map[string]ModeBreakdown

func (b Breakdown) Value() (driver.Value, error) {
	if b == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(b)
}

func (b *Breakdown) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("Breakdown: expected []byte from db")
	}
	return json.Unmarshal(bytes, b)
}

type DailySummary struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"           json:"id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_daily_user_date" json:"user_id"`
	DateLocal       time.Time `gorm:"type:date;not null;uniqueIndex:idx_daily_user_date" json:"date_local"`
	TotalKgCO2e     float64   `gorm:"type:numeric(12,4);not null;default:0" json:"total_kg_co2e"`
	TotalDistanceKM float64   `gorm:"type:numeric(10,3);not null;default:0" json:"total_distance_km"`
	ActivityCount   int       `gorm:"not null;default:0"             json:"activity_count"`
	Breakdown       Breakdown `gorm:"type:jsonb;not null;default:'{}'" json:"breakdown"`
	ComputedAt      time.Time `gorm:"not null"                       json:"computed_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (DailySummary) TableName() string { return "daily_summaries" }

type WeeklySummary struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"           json:"id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_weekly_user_week" json:"user_id"`
	WeekStart       time.Time `gorm:"type:date;not null;uniqueIndex:idx_weekly_user_week" json:"week_start"`
	WeekEnd         time.Time `gorm:"type:date;not null"             json:"week_end"`
	TotalKgCO2e     float64   `gorm:"type:numeric(12,4);not null;default:0" json:"total_kg_co2e"`
	TotalDistanceKM float64   `gorm:"type:numeric(10,3);not null;default:0" json:"total_distance_km"`
	ActivityCount   int       `gorm:"not null;default:0"             json:"activity_count"`
	Breakdown       Breakdown `gorm:"type:jsonb;not null;default:'{}'" json:"breakdown"`
	ComputedAt      time.Time `gorm:"not null"                       json:"computed_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (WeeklySummary) TableName() string { return "weekly_summaries" }

type MonthlySummary struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"           json:"id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_monthly_user_ym" json:"user_id"`
	Year            int       `gorm:"not null;uniqueIndex:idx_monthly_user_ym" json:"year"`
	Month           int       `gorm:"not null;uniqueIndex:idx_monthly_user_ym" json:"month"`
	TotalKgCO2e     float64   `gorm:"type:numeric(12,4);not null;default:0" json:"total_kg_co2e"`
	TotalDistanceKM float64   `gorm:"type:numeric(10,3);not null;default:0" json:"total_distance_km"`
	ActivityCount   int       `gorm:"not null;default:0"             json:"activity_count"`
	Breakdown       Breakdown `gorm:"type:jsonb;not null;default:'{}'" json:"breakdown"`
	ComputedAt      time.Time `gorm:"not null"                       json:"computed_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (MonthlySummary) TableName() string { return "monthly_summaries" }
