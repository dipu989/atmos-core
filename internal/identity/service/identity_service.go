package service

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/identity/domain"
	"github.com/dipu/atmos-core/internal/identity/repository"
	pkguuid "github.com/dipu/atmos-core/platform/uuid"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("user not found")

type IdentityService struct {
	repo *repository.UserRepository
}

func NewIdentityService(repo *repository.UserRepository) *IdentityService {
	return &IdentityService{repo: repo}
}

// ── Profile ───────────────────────────────────────────────────────────────────

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

// ── Preferences ───────────────────────────────────────────────────────────────

type UpdatePreferencesInput struct {
	DistanceUnit             *string
	PushNotificationsEnabled *bool
	WeeklyReportEnabled      *bool
	DailyGoalKgCO2e          *float64
	DataSharingEnabled       *bool
}

func (s *IdentityService) GetPreferences(ctx context.Context, userID uuid.UUID) (*domain.UserPreferences, error) {
	prefs, err := s.repo.GetPreferences(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.createDefaultPreferences(ctx, userID)
	}
	return prefs, err
}

func (s *IdentityService) UpdatePreferences(ctx context.Context, userID uuid.UUID, input UpdatePreferencesInput) (*domain.UserPreferences, error) {
	prefs, err := s.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}

	if input.DistanceUnit != nil {
		prefs.DistanceUnit = *input.DistanceUnit
	}
	if input.PushNotificationsEnabled != nil {
		prefs.PushNotificationsEnabled = *input.PushNotificationsEnabled
	}
	if input.WeeklyReportEnabled != nil {
		prefs.WeeklyReportEnabled = *input.WeeklyReportEnabled
	}
	if input.DailyGoalKgCO2e != nil {
		prefs.DailyGoalKgCO2e = input.DailyGoalKgCO2e
	}
	if input.DataSharingEnabled != nil {
		prefs.DataSharingEnabled = *input.DataSharingEnabled
	}

	if err := s.repo.UpdatePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return prefs, nil
}

func (s *IdentityService) createDefaultPreferences(ctx context.Context, userID uuid.UUID) (*domain.UserPreferences, error) {
	prefs := &domain.UserPreferences{
		ID:                       pkguuid.New(),
		UserID:                   userID,
		DistanceUnit:             "km",
		PushNotificationsEnabled: true,
		WeeklyReportEnabled:      true,
		DataSharingEnabled:       false,
	}
	if err := s.repo.CreatePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return prefs, nil
}
