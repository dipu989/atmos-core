package domain

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null"   json:"user_id"`
	DeviceID  *uuid.UUID `gorm:"type:uuid"            json:"device_id,omitempty"`
	TokenHash string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null"             json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

func (r *RefreshToken) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

func (r *RefreshToken) IsRevoked() bool {
	return r.RevokedAt != nil
}

func (r *RefreshToken) IsValid() bool {
	return !r.IsExpired() && !r.IsRevoked()
}
