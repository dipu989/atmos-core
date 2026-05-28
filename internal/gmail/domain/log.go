package domain

import (
	"time"

	"github.com/google/uuid"
)

// Ingestion status values.
const (
	StatusParsed  = "parsed"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// EmailIngestionLog records every Gmail message we attempted to process.
// UNIQUE (user_id, message_id) guarantees idempotency — same message is
// never processed twice even if sync is triggered multiple times.
type EmailIngestionLog struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"     json:"id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	MessageID   string     `gorm:"not null"                 json:"message_id"`
	SenderCode  *string    `json:"sender_code,omitempty"` // FK → provider_email_types.code; NULL for pre-route skips
	Subject     string     `gorm:"not null;default:''"      json:"subject"`
	Snippet     string     `gorm:"not null;default:''"      json:"snippet"`
	Status      string     `gorm:"not null;default:'pending'" json:"status"`
	ActivityID  *uuid.UUID `gorm:"type:uuid"                json:"activity_id,omitempty"`
	ErrorReason *string    `json:"error_reason,omitempty"`
	ParsedAt    time.Time  `gorm:"not null"                 json:"parsed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (EmailIngestionLog) TableName() string { return "email_ingestion_logs" }
