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
	HomeAddress              *string   `gorm:"column:home_address"            json:"home_address,omitempty"`
	HomeLat                  *float64  `gorm:"column:home_lat"                json:"home_lat,omitempty"`
	HomeLng                  *float64  `gorm:"column:home_lng"                json:"home_lng,omitempty"`
	WorkAddress              *string   `gorm:"column:work_address"            json:"work_address,omitempty"`
	WorkLat                  *float64  `gorm:"column:work_lat"                json:"work_lat,omitempty"`
	WorkLng                  *float64  `gorm:"column:work_lng"                json:"work_lng,omitempty"`
	DefaultTransport         *string   `gorm:"column:default_transport"       json:"default_transport,omitempty"`
	Region                   string    `gorm:"not null;default:'IN'"          json:"region"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (UserPreferences) TableName() string { return "user_preferences" }
