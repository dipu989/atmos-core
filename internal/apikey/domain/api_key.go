package domain

import (
	"time"

	"github.com/google/uuid"
)

const MaxKeysPerUser = 10

type APIKey struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"  json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null"    json:"user_id"`
	Name       string     `gorm:"not null"              json:"name"`
	KeyHash    string     `gorm:"uniqueIndex;not null"  json:"-"`
	Prefix     string     `gorm:"not null"              json:"prefix"`
	LastUsedAt *time.Time `                             json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `                             json:"expires_at,omitempty"`
	CreatedAt  time.Time  `                             json:"created_at"`
	RevokedAt  *time.Time `                             json:"revoked_at,omitempty"`
}

func (APIKey) TableName() string { return "api_keys" }

func (k *APIKey) IsRevoked() bool {
	return k.RevokedAt != nil
}

func (k *APIKey) IsExpired() bool {
	return k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt)
}

func (k *APIKey) IsActive() bool {
	return !k.IsRevoked() && !k.IsExpired()
}
