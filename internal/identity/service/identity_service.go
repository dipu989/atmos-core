package service

import (
	"context"
	"errors"
	"strings"
	"time"

	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	"github.com/dipu/atmos-core/internal/identity/domain"
	"github.com/dipu/atmos-core/internal/identity/repository"
	pkguuid "github.com/dipu/atmos-core/platform/uuid"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotFound            = errors.New("user not found")
	ErrInvalidConfirmation = errors.New("confirmation text does not match")
)

// deletionGracePeriod is how long a soft-deleted account can be recovered.
const deletionGracePeriod = 7 * 24 * time.Hour

type IdentityService struct {
	repo      *repository.UserRepository
	tokenRepo *authrepo.TokenRepository
}

func NewIdentityService(repo *repository.UserRepository, tokenRepo *authrepo.TokenRepository) *IdentityService {
	return &IdentityService{repo: repo, tokenRepo: tokenRepo}
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
	DailyGoalKgCO2e          *float64
	HomeAddress              *string
	HomeLat                  *float64
	HomeLng                  *float64
	WorkAddress              *string
	WorkLat                  *float64
	WorkLng                  *float64
	DefaultTransport         *string
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
	if input.DailyGoalKgCO2e != nil {
		prefs.DailyGoalKgCO2e = input.DailyGoalKgCO2e
	}
	if input.HomeAddress != nil {
		prefs.HomeAddress = input.HomeAddress
	}
	if input.HomeLat != nil {
		prefs.HomeLat = input.HomeLat
	}
	if input.HomeLng != nil {
		prefs.HomeLng = input.HomeLng
	}
	if input.WorkAddress != nil {
		prefs.WorkAddress = input.WorkAddress
	}
	if input.WorkLat != nil {
		prefs.WorkLat = input.WorkLat
	}
	if input.WorkLng != nil {
		prefs.WorkLng = input.WorkLng
	}
	if input.DefaultTransport != nil {
		prefs.DefaultTransport = input.DefaultTransport
	}

	if err := s.repo.UpdatePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return prefs, nil
}

// ── Account deletion ──────────────────────────────────────────────────────────

// DeleteAccount soft-deletes the user's account and revokes all their sessions.
// All accounts must supply the exact string "delete" as confirmation.
func (s *IdentityService) DeleteAccount(ctx context.Context, userID uuid.UUID, confirmation string) error {
	if strings.ToLower(strings.TrimSpace(confirmation)) != "delete" {
		return ErrInvalidConfirmation
	}

	if _, err := s.repo.FindByID(ctx, userID); err != nil {
		return ErrNotFound
	}

	if err := s.repo.SoftDelete(ctx, userID); err != nil {
		return err
	}

	// Revoke all active sessions so the user is immediately logged out.
	_ = s.tokenRepo.RevokeAllForUser(ctx, userID)
	return nil
}

// PurgeDeletedAccounts hard-deletes accounts whose grace period has expired.
// Safe to call repeatedly — idempotent. Returns count of purged accounts.
func (s *IdentityService) PurgeDeletedAccounts(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-deletionGracePeriod)
	return s.repo.PurgeDeleted(ctx, cutoff)
}

func (s *IdentityService) createDefaultPreferences(ctx context.Context, userID uuid.UUID) (*domain.UserPreferences, error) {
	prefs := &domain.UserPreferences{
		ID:                       pkguuid.New(),
		UserID:                   userID,
		DistanceUnit:             "km",
		PushNotificationsEnabled: true,
	}
	if err := s.repo.CreatePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return prefs, nil
}
