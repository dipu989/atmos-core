package domain

import (
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken stores a hashed single-use token for password resets.
// The raw token is sent to the user by email; only the hash lives in the DB.
type PasswordResetToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null"   json:"user_id"`
	TokenHash string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null"             json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (PasswordResetToken) TableName() string { return "password_reset_tokens" }

func (t *PasswordResetToken) IsExpired() bool { return time.Now().After(t.ExpiresAt) }
func (t *PasswordResetToken) IsUsed() bool    { return t.UsedAt != nil }
func (t *PasswordResetToken) IsValid() bool   { return !t.IsExpired() && !t.IsUsed() }
