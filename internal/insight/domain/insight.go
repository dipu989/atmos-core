package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type InsightType string
type PeriodType string

const (
	InsightStreak     InsightType = "streak"
	InsightMilestone  InsightType = "milestone"
	InsightComparison InsightType = "comparison"
	InsightTip        InsightType = "tip"
	InsightAnomaly    InsightType = "anomaly"

	PeriodDaily   PeriodType = "daily"
	PeriodWeekly  PeriodType = "weekly"
	PeriodMonthly PeriodType = "monthly"
)

type InsightMetadata map[string]any

func (m InsightMetadata) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func (m *InsightMetadata) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("InsightMetadata: expected []byte from db")
	}
	return json.Unmarshal(bytes, m)
}

type Insight struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey"      json:"id"`
	UserID      uuid.UUID       `gorm:"type:uuid;not null"         json:"user_id"`
	InsightType InsightType     `gorm:"not null"                   json:"insight_type"`
	PeriodType  PeriodType      `gorm:"not null"                   json:"period_type"`
	PeriodStart time.Time       `gorm:"type:date;not null"         json:"period_start"`
	PeriodEnd   time.Time       `gorm:"type:date;not null"         json:"period_end"`
	Title       string          `gorm:"not null"                   json:"title"`
	Body        string          `gorm:"not null"                   json:"body"`
	CTALabel    *string         `json:"cta_label,omitempty"`
	CTATarget   *string         `json:"cta_target,omitempty"`
	Metadata    InsightMetadata `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	IsRead      bool            `gorm:"not null;default:false"     json:"is_read"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func (Insight) TableName() string { return "insights" }
