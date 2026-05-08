package repository

import (
	"context"

	"github.com/dipu/atmos-core/internal/identity/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *UserRepository) FindOrCreateByOAuth(ctx context.Context, provider, providerUserID, email, displayName string) (*domain.User, bool, error) {
	var op domain.OAuthProvider
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&op).Error

	if err == nil {
		user, userErr := r.FindByID(ctx, op.UserID)
		return user, false, userErr
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}

	// New OAuth user — find by email or create
	user, err := r.FindByEmail(ctx, email)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, false, err
	}

	created := false
	if user == nil {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return nil, false, idErr
		}
		user = &domain.User{
			ID:          id,
			Email:       email,
			DisplayName: displayName,
			Timezone:    "UTC",
			Locale:      "en",
		}
		if createErr := r.db.WithContext(ctx).Create(user).Error; createErr != nil {
			return nil, false, createErr
		}
		created = true
	}

	opID, _ := uuid.NewV7()
	newOp := &domain.OAuthProvider{
		ID:             opID,
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
	}
	if linkErr := r.db.WithContext(ctx).Create(newOp).Error; linkErr != nil {
		return nil, false, linkErr
	}

	return user, created, nil
}
