package service

import (
	"context"

	"github.com/dipu/atmos-core/internal/identity/domain"
	"github.com/dipu/atmos-core/internal/identity/repository"
	"github.com/google/uuid"
)

type IdentityService struct {
	repo *repository.UserRepository
}

func NewIdentityService(repo *repository.UserRepository) *IdentityService {
	return &IdentityService{repo: repo}
}

func (s *IdentityService) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.repo.FindByID(ctx, userID)
}

func (s *IdentityService) UpdateProfile(ctx context.Context, userID uuid.UUID, displayName, timezone, locale string, avatarURL *string) (*domain.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if displayName != "" {
		user.DisplayName = displayName
	}
	if timezone != "" {
		user.Timezone = timezone
	}
	if locale != "" {
		user.Locale = locale
	}
	if avatarURL != nil {
		user.AvatarURL = avatarURL
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
