package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SyncSummary is stored after every sync run and returned in the status response.
// It lets the frontend show "Last synced 2 hours ago — 3 trips found" without
// an extra API call.
type SyncSummary struct {
	MessagesChecked int `json:"messages_checked"`
	Parsed          int `json:"parsed"`
	Skipped         int `json:"skipped"`
	Failed          int `json:"failed"`
}

// Value / Scan make SyncSummary storable as JSONB in Postgres.
func (s SyncSummary) Value() (driver.Value, error) { return json.Marshal(s) }
func (s *SyncSummary) Scan(v any) error {
	b, ok := v.([]byte)
	if !ok {
		return errors.New("SyncSummary: expected []byte")
	}
	return json.Unmarshal(b, s)
}

// GmailConnection holds the OAuth tokens we need to access a user's Gmail.
type GmailConnection struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey"           json:"id"`
	UserID          uuid.UUID    `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	Email           string       `gorm:"not null"                       json:"email"`
	AccessToken     string       `gorm:"not null"                       json:"-"` // never expose
	RefreshToken    string       `gorm:"not null"                       json:"-"` // never expose
	TokenExpiry     time.Time    `gorm:"not null"                       json:"token_expiry"`
	HistoryID       *string      `json:"history_id,omitempty"` // Gmail historyId for incremental sync
	LastSyncAt      *time.Time   `json:"last_sync_at,omitempty"`
	LastSyncSummary *SyncSummary `gorm:"type:jsonb"                     json:"last_sync_summary,omitempty"`
	ConnectedAt     time.Time    `gorm:"not null"                       json:"connected_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

func (GmailConnection) TableName() string { return "gmail_connections" }
