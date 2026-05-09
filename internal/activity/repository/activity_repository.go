package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ActivityRepository struct {
	db *gorm.DB
}

func NewActivityRepository(db *gorm.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

func (r *ActivityRepository) Create(ctx context.Context, activity *domain.Activity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

func (r *ActivityRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.Activity, error) {
	var a domain.Activity
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *ActivityRepository) ListByUser(ctx context.Context, userID uuid.UUID, from, to time.Time, limit, offset int) ([]domain.Activity, error) {
	var activities []domain.Activity
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND date_local BETWEEN ? AND ?", userID, from, to).
		Order("started_at DESC").
		Limit(limit).
		Offset(offset)
	return activities, q.Find(&activities).Error
}

func (r *ActivityRepository) CountByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Activity{}).
		Where("user_id = ? AND date_local BETWEEN ? AND ?", userID, from, to).
		Count(&count).Error
	return count, err
}

func (r *ActivityRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ActivityStatus, reason *string) error {
	updates := map[string]any{"status": status}
	if reason != nil {
		updates["failure_reason"] = *reason
	}
	return r.db.WithContext(ctx).Model(&domain.Activity{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ActivityRepository) ExistsByIdempotencyKey(ctx context.Context, key string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Activity{}).Where("idempotency_key = ?", key).Count(&count).Error
	return count > 0, err
}
