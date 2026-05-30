package repository

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/auth/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PasswordResetRepository struct {
	db *gorm.DB
}

func NewPasswordResetRepository(db *gorm.DB) *PasswordResetRepository {
	return &PasswordResetRepository{db: db}
}

// Create persists a new reset token.
func (r *PasswordResetRepository) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// FindByHash looks up an unused, unexpired token by its hash.
func (r *PasswordResetRepository) FindByHash(ctx context.Context, hash string) (*domain.PasswordResetToken, error) {
	var t domain.PasswordResetToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ?", hash).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}

// MarkUsed stamps the token as consumed so it cannot be replayed.
func (r *PasswordResetRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&domain.PasswordResetToken{}).
		Where("id = ?", id).
		Update("used_at", gorm.Expr("NOW()")).Error
}

// InvalidatePrevious marks all unused tokens for a user as used,
// ensuring only one active token exists per user at a time.
func (r *PasswordResetRepository) InvalidatePrevious(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&domain.PasswordResetToken{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Update("used_at", gorm.Expr("NOW()")).Error
}
