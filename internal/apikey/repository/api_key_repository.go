package repository

import (
	"context"
	"errors"
	"time"

	"github.com/dipu/atmos-core/internal/apikey/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("api key not found")

type APIKeyRepository struct {
	db *gorm.DB
}

func NewAPIKeyRepository(db *gorm.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(ctx context.Context, key *domain.APIKey) error {
	return r.db.WithContext(ctx).Create(key).Error
}

func (r *APIKeyRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.APIKey, error) {
	var keys []domain.APIKey
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > NOW())", userID).
		Order("created_at DESC").
		Find(&keys).Error
	return keys, err
}

func (r *APIKeyRepository) FindByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	var key domain.APIKey
	err := r.db.WithContext(ctx).Where("key_hash = ?", hash).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&domain.APIKey{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *APIKeyRepository) CountActiveByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.APIKey{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.APIKey{}).
		Where("id = ?", id).
		Update("last_used_at", now).Error
}
