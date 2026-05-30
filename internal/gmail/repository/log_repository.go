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

// IsProcessed returns true if we have already attempted this Gmail message for this user.
func (r *LogRepository) IsProcessed(ctx context.Context, userID uuid.UUID, messageID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.EmailIngestionLog{}).
		Where("user_id = ? AND message_id = ?", userID, messageID).
		Count(&count).Error
	return count > 0, err
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
