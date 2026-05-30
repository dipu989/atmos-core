package dto

import (
	"time"

	"github.com/dipu/atmos-core/internal/gmail/domain"
)

// ConnectionStatus is returned by GET /gmail/status.
// The frontend uses this to render the "Gmail connected" card — it shows the
// connected email, when the last sync ran, and how many trips were found.
type ConnectionStatus struct {
	Connected       bool                `json:"connected"`
	Email           string              `json:"email,omitempty"`
	ConnectedAt     *time.Time          `json:"connected_at,omitempty"`
	LastSyncAt      *time.Time          `json:"last_sync_at,omitempty"`
	LastSyncSummary *domain.SyncSummary `json:"last_sync_summary,omitempty"`
}

func ConnectionStatusFromDomain(conn *domain.GmailConnection) ConnectionStatus {
	if conn == nil {
		return ConnectionStatus{Connected: false}
	}
	return ConnectionStatus{
		Connected:       true,
		Email:           conn.Email,
		ConnectedAt:     &conn.ConnectedAt,
		LastSyncAt:      conn.LastSyncAt,
		LastSyncSummary: conn.LastSyncSummary,
	}
}

// SyncResponse is returned by POST /gmail/sync.
type SyncResponse struct {
	MessagesChecked int    `json:"messages_checked"`
	Parsed          int    `json:"parsed"`
	Skipped         int    `json:"skipped"`
	Failed          int    `json:"failed"`
	Message         string `json:"message"`
}

// LogsPage is returned by GET /gmail/logs.
type LogsPage struct {
	Logs   []domain.EmailIngestionLog `json:"logs"`
	Total  int64                      `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}
