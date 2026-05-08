package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string     `gorm:"uniqueIndex;not null"  json:"email"`
	PasswordHash *string    `gorm:"column:password_hash"  json:"-"`
	DisplayName  string     `gorm:"not null;default:''"   json:"display_name"`
	AvatarURL    *string    `gorm:"column:avatar_url"     json:"avatar_url,omitempty"`
	Timezone     string     `gorm:"not null;default:'UTC'" json:"timezone"`
	Locale       string     `gorm:"not null;default:'en'" json:"locale"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `gorm:"index"                 json:"-"`
}

func (User) TableName() string { return "users" }

type OAuthProvider struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null"   json:"user_id"`
	Provider       string     `gorm:"not null"             json:"provider"`
	ProviderUserID string     `gorm:"not null"             json:"provider_user_id"`
	AccessToken    string     `gorm:"not null;default:''"  json:"-"`
	RefreshToken   *string    `json:"-"`
	TokenExpiry    *time.Time `json:"token_expiry,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (OAuthProvider) TableName() string { return "oauth_providers" }
