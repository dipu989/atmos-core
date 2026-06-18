package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateAPIKeyRequest struct {
	Name string `json:"name" validate:"required,min=1,max=64"`
}

// CreateAPIKeyResponse is returned once at creation time — Key is never returned again.
type CreateAPIKeyResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKeyItem struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
