package repository

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/auth/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailVerificationRepository struct {
	db *gorm.DB
}

func NewEmailVerificationRepository(db *gorm.DB) *EmailVerificationRepository {
	return &EmailVerificationRepository{db: db}
}

func (r *EmailVerificationRepository) Create(ctx context.Context, t *domain.EmailVerificationToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *EmailVerificationRepository) FindByHash(ctx context.Context, hash string) (*domain.EmailVerificationToken, error) {
	var t domain.EmailVerificationToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}

func (r *EmailVerificationRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&domain.EmailVerificationToken{}).
		Where("id = ?", id).
		Update("used_at", gorm.Expr("NOW()")).Error
}

// InvalidatePrevious marks all unused tokens for this user as consumed,
// ensuring only one active verification token exists per user at a time.
func (r *EmailVerificationRepository) InvalidatePrevious(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&domain.EmailVerificationToken{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Update("used_at", gorm.Expr("NOW()")).Error
}
