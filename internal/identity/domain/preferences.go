package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserPreferences struct {
	ID                       uuid.UUID `gorm:"type:uuid;primaryKey"           json:"id"`
	UserID                   uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	DistanceUnit             string    `gorm:"not null;default:'km'"          json:"distance_unit"`
	PushNotificationsEnabled bool      `gorm:"not null;default:true"          json:"push_notifications_enabled"`
	WeeklyReportEnabled      bool      `gorm:"not null;default:true"          json:"weekly_report_enabled"`
	DailyGoalKgCO2e          *float64  `gorm:"column:daily_goal_kg_co2e"      json:"daily_goal_kg_co2e,omitempty"`
	DataSharingEnabled       bool      `gorm:"not null;default:false"         json:"data_sharing_enabled"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (UserPreferences) TableName() string { return "user_preferences" }
