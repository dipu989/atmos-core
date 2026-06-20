package repository

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/gmail/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LogRepository struct {
	db *gorm.DB
}

func NewLogRepository(db *gorm.DB) *LogRepository {
	return &LogRepository{db: db}
}

func (r *LogRepository) Create(ctx context.Context, log *domain.EmailIngestionLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *LogRepository) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.EmailIngestionLog, int64, error) {
	var logs []domain.EmailIngestionLog
	var total int64

	base := r.db.WithContext(ctx).Model(&domain.EmailIngestionLog{}).Where("user_id = ?", userID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := base.Order("parsed_at DESC").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

// FindByMessageID fetches a log entry — returns nil (no error) when not found.
func (r *LogRepository) FindByMessageID(ctx context.Context, userID uuid.UUID, messageID string) (*domain.EmailIngestionLog, error) {
	var log domain.EmailIngestionLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND message_id = ?", userID, messageID).
		First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &log, err
}

// ListUnrecognised returns up to limit log entries with status "unrecognised"
// for a specific user, ordered oldest-first so the earliest queued emails are
// processed first.
func (r *LogRepository) ListUnrecognised(ctx context.Context, userID uuid.UUID, limit int) ([]domain.EmailIngestionLog, error) {
	var logs []domain.EmailIngestionLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, domain.StatusUnrecognised).
		Order("parsed_at ASC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// UpdateStatus sets the status (and optionally activity_id) on an existing log entry.
func (r *LogRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, activityID *uuid.UUID) error {
	updates := map[string]any{"status": status}
	if activityID != nil {
		updates["activity_id"] = *activityID
	}
	return r.db.WithContext(ctx).
		Model(&domain.EmailIngestionLog{}).
		Where("id = ?", id).
		Updates(updates).Error
}
