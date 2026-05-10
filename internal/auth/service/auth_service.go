package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	authdomain "github.com/dipu/atmos-core/internal/auth/domain"
	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	identitydomain "github.com/dipu/atmos-core/internal/identity/domain"
	identityrepo "github.com/dipu/atmos-core/internal/identity/repository"
	"github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/uuid"
	googleuuid "github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired refresh token")
	ErrOAuthNotConfigured = errors.New("google OAuth is not configured")
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AuthService struct {
	userRepo    *identityrepo.UserRepository
	tokenRepo   *authrepo.TokenRepository
	jwtManager  *jwt.Manager
	googleOAuth *oauth2.Config // nil when Google credentials are not set
}

func NewAuthService(
	userRepo *identityrepo.UserRepository,
	tokenRepo *authrepo.TokenRepository,
	jwtManager *jwt.Manager,
	googleClientID, googleClientSecret, googleRedirectURL string,
) *AuthService {
	svc := &AuthService{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtManager: jwtManager,
	}
	if googleClientID != "" && googleClientSecret != "" {
		svc.googleOAuth = &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  googleRedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}
	return svc
}

// ── Email / Password ─────────────────────────────────────────────────────────

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

	user := &identitydomain.User{
		ID:           uuid.New(),
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

func (s *AuthService) Login(ctx context.Context, email, password string, deviceID *googleuuid.UUID) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.PasswordHash == nil {
		// OAuth-only account — no password set
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

	stored, err := s.tokenRepo.FindByHash(ctx, hashToken(rawToken))
	if err != nil || !stored.IsValid() {
		return nil, ErrInvalidToken
	}

	if err := s.tokenRepo.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}
	return s.issuePair(ctx, claims.UserID, stored.DeviceID)
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	stored, err := s.tokenRepo.FindByHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil // already gone — treat as success
	}
	return s.tokenRepo.Revoke(ctx, stored.ID)
}

// ── Google OAuth ─────────────────────────────────────────────────────────────

// GoogleAuthURL returns the Google consent-screen redirect URL and the CSRF state value.
func (s *AuthService) GoogleAuthURL() (url, state string, err error) {
	if s.googleOAuth == nil {
		return "", "", ErrOAuthNotConfigured
	}
	state, err = generateState()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate OAuth state: %w", err)
	}
	url = s.googleOAuth.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return url, state, nil
}

// HandleGoogleCallback exchanges the authorization code for tokens, fetches the
// Google user profile, and finds or creates the local user record.
// Returns (user, tokenPair, isNewUser, error).
func (s *AuthService) HandleGoogleCallback(ctx context.Context, code string) (*identitydomain.User, *TokenPair, bool, error) {
	if s.googleOAuth == nil {
		return nil, nil, false, ErrOAuthNotConfigured
	}

	oauthToken, err := s.googleOAuth.Exchange(ctx, code)
	if err != nil {
		return nil, nil, false, fmt.Errorf("code exchange failed: %w", err)
	}

	profile, err := fetchGoogleProfile(ctx, s.googleOAuth, oauthToken)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to fetch Google profile: %w", err)
	}

	user, isNew, err := s.userRepo.FindOrCreateByOAuth(ctx, "google", profile.ID, profile.Email, profile.Name)
	if err != nil {
		return nil, nil, false, err
	}

	// Update avatar from Google if not already set
	if user.AvatarURL == nil && profile.Picture != "" {
		user.AvatarURL = &profile.Picture
		_ = s.userRepo.Update(ctx, user)
	}

	pair, err := s.issuePair(ctx, user.ID, nil)
	if err != nil {
		return nil, nil, false, err
	}
	return user, pair, isNew, nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (s *AuthService) issuePair(ctx context.Context, userID googleuuid.UUID, deviceID *googleuuid.UUID) (*TokenPair, error) {
	accessToken, err := s.jwtManager.IssueAccessToken(userID)
	if err != nil {
		return nil, err
	}
	refreshTokenStr, err := s.jwtManager.IssueRefreshToken(userID)
	if err != nil {
		return nil, err
	}

	record := &authdomain.RefreshToken{
		ID:        uuid.New(),
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

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// googleProfile holds the fields we read from Google's userinfo endpoint.
type googleProfile struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func fetchGoogleProfile(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (*googleProfile, error) {
	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google userinfo returned %d: %s", resp.StatusCode, body)
	}

	var profile googleProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}
	if profile.ID == "" || profile.Email == "" {
		return nil, errors.New("google profile missing id or email")
	}
	return &profile, nil
}
