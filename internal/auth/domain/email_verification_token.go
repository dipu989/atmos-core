package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailVerificationToken is a single-use token sent to the user's inbox.
// The raw token is in the email; only the SHA-256 hash is stored here.
type EmailVerificationToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null"   json:"user_id"`
	TokenHash string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null"             json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (EmailVerificationToken) TableName() string { return "email_verification_tokens" }

func (t *EmailVerificationToken) IsExpired() bool { return time.Now().After(t.ExpiresAt) }
func (t *EmailVerificationToken) IsUsed() bool    { return t.UsedAt != nil }
func (t *EmailVerificationToken) IsValid() bool   { return !t.IsExpired() && !t.IsUsed() }
