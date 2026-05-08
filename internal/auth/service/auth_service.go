package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	authdomain "github.com/dipu/atmos-core/internal/auth/domain"
	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	identitydomain "github.com/dipu/atmos-core/internal/identity/domain"
	identityrepo "github.com/dipu/atmos-core/internal/identity/repository"
	"github.com/dipu/atmos-core/platform/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmailTaken      = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken    = errors.New("invalid or expired refresh token")
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AuthService struct {
	userRepo     *identityrepo.UserRepository
	tokenRepo    *authrepo.TokenRepository
	jwtManager   *jwt.Manager
}

func NewAuthService(
	userRepo *identityrepo.UserRepository,
	tokenRepo *authrepo.TokenRepository,
	jwtManager *jwt.Manager,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtManager: jwtManager,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password, displayName string) (*identitydomain.User, *TokenPair, error) {
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if existing != nil {
		return nil, nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, err
	}
	hashStr := string(hash)

	id, err := uuid.NewV7()
	if err != nil {
		return nil, nil, err
	}
	user := &identitydomain.User{
		ID:           id,
		Email:        email,
		PasswordHash: &hashStr,
		DisplayName:  displayName,
		Timezone:     "UTC",
		Locale:       "en",
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, err
	}

	pair, err := s.issuePair(ctx, user.ID, nil)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string, deviceID *uuid.UUID) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.PasswordHash == nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issuePair(ctx, user.ID, deviceID)
}

func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*TokenPair, error) {
	claims, err := s.jwtManager.ParseRefreshToken(rawToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	hash := hashToken(rawToken)
	stored, err := s.tokenRepo.FindByHash(ctx, hash)
	if err != nil || !stored.IsValid() {
		return nil, ErrInvalidToken
	}

	if err := s.tokenRepo.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}
	return s.issuePair(ctx, claims.UserID, stored.DeviceID)
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	hash := hashToken(rawToken)
	stored, err := s.tokenRepo.FindByHash(ctx, hash)
	if err != nil {
		return nil // already gone — treat as success
	}
	return s.tokenRepo.Revoke(ctx, stored.ID)
}

func (s *AuthService) issuePair(ctx context.Context, userID uuid.UUID, deviceID *uuid.UUID) (*TokenPair, error) {
	accessToken, err := s.jwtManager.IssueAccessToken(userID)
	if err != nil {
		return nil, err
	}
	refreshTokenStr, err := s.jwtManager.IssueRefreshToken(userID)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	record := &authdomain.RefreshToken{
		ID:        id,
		UserID:    userID,
		DeviceID:  deviceID,
		TokenHash: hashToken(refreshTokenStr),
		ExpiresAt: time.Now().Add(s.jwtManager.RefreshTTL()),
	}
	if err := s.tokenRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
	}, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
