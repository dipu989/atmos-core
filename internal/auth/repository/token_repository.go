package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/auth/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *TokenRepository) FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var token domain.RefreshToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *TokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked_at", now).Error
}

func (r *TokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}
