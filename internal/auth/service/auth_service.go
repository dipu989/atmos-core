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
	"strings"
	"time"

	authdomain "github.com/dipu/atmos-core/internal/auth/domain"
	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	identitydomain "github.com/dipu/atmos-core/internal/identity/domain"
	identityrepo "github.com/dipu/atmos-core/internal/identity/repository"
	"github.com/dipu/atmos-core/platform/email"
	"github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/uuid"
	googleuuid "github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"gorm.io/gorm"
)

var (
	ErrEmailTaken               = errors.New("email already registered")
	ErrInvalidCredentials       = errors.New("invalid email or password")
	ErrInvalidToken             = errors.New("invalid or expired refresh token")
	ErrOAuthNotConfigured       = errors.New("google OAuth is not configured")
	ErrInvalidResetToken        = errors.New("invalid or expired password reset token")
	ErrInvalidVerificationToken = errors.New("invalid or expired verification token")
	ErrAlreadyVerified          = errors.New("email is already verified")
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AuthService struct {
	userRepo         *identityrepo.UserRepository
	tokenRepo        *authrepo.TokenRepository
	resetRepo        *authrepo.PasswordResetRepository
	verificationRepo *authrepo.EmailVerificationRepository
	jwtManager       *jwt.Manager
	emailSender      email.Sender
	frontendURL      string
	googleOAuth      *oauth2.Config // nil when Google credentials are not set
}

type Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	EmailSender        email.Sender
	FrontendURL        string
}

func NewAuthService(
	userRepo *identityrepo.UserRepository,
	tokenRepo *authrepo.TokenRepository,
	resetRepo *authrepo.PasswordResetRepository,
	verificationRepo *authrepo.EmailVerificationRepository,
	jwtManager *jwt.Manager,
	cfg Config,
) *AuthService {
	svc := &AuthService{
		userRepo:         userRepo,
		tokenRepo:        tokenRepo,
		resetRepo:        resetRepo,
		verificationRepo: verificationRepo,
		jwtManager:       jwtManager,
		emailSender:      cfg.EmailSender,
		frontendURL:      cfg.FrontendURL,
	}
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		svc.googleOAuth = &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
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

	// Send verification email asynchronously — a failure here should not
	// block the registration response. The user can request a resend later.
	go func() {
		ctx2 := context.Background()
		if err := s.sendVerificationEmail(ctx2, user); err != nil {
			// Log but don't surface — registration already succeeded.
			_ = err
		}
	}()

	pair, err := s.issuePair(ctx, user.ID, nil)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string, deviceID *googleuuid.UUID) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// If not found as active, check whether it's a soft-deleted account within grace period.
	if user == nil {
		user, err = s.userRepo.FindByEmailUnscoped(ctx, email)
		if err != nil {
			return nil, err
		}
		if user == nil || user.DeletedAt == nil {
			return nil, ErrInvalidCredentials
		}
		// Outside the 7-day grace period — treat as gone.
		if time.Since(*user.DeletedAt) > 7*24*time.Hour {
			return nil, ErrInvalidCredentials
		}
		// Inside grace period — verify password then restore.
		if user.PasswordHash == nil {
			return nil, ErrInvalidCredentials
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
			return nil, ErrInvalidCredentials
		}
		if err := s.userRepo.Restore(ctx, user.ID); err != nil {
			return nil, fmt.Errorf("restore account: %w", err)
		}
		return s.issuePair(ctx, user.ID, deviceID)
	}

	if user.PasswordHash == nil {
		// OAuth-only account — no password set.
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

	displayName := profile.Name
	if displayName == "" {
		if idx := strings.Index(profile.Email, "@"); idx > 0 {
			displayName = profile.Email[:idx]
		} else {
			displayName = profile.Email
		}
	}

	user, isNew, err := s.userRepo.FindOrCreateByOAuth(ctx, "google", profile.ID, profile.Email, displayName)
	if err != nil {
		return nil, nil, false, err
	}

	// If the matched account is soft-deleted and within the grace period, restore it.
	if user != nil && user.DeletedAt != nil {
		if time.Since(*user.DeletedAt) <= 7*24*time.Hour {
			_ = s.userRepo.Restore(ctx, user.ID)
			user.DeletedAt = nil
		} else {
			return nil, nil, false, ErrInvalidCredentials
		}
	}

	// Google has confirmed this email — mark it verified if not already.
	needsSave := false
	if user.AvatarURL == nil && profile.Picture != "" {
		user.AvatarURL = &profile.Picture
		needsSave = true
	}
	if !user.IsEmailVerified() {
		now := time.Now().UTC()
		user.EmailVerifiedAt = &now
		needsSave = true
	}
	if !isNew && user.DisplayName == "" && displayName != "" {
		user.DisplayName = displayName
		needsSave = true
	}
	if needsSave {
		_ = s.userRepo.Update(ctx, user)
	}

	pair, err := s.issuePair(ctx, user.ID, nil)
	if err != nil {
		return nil, nil, false, err
	}
	return user, pair, isNew, nil
}

// HandleGoogleIDToken verifies a Google ID token issued by the native mobile
// Sign-In SDK and returns a local user + JWT pair.
// This is the mobile-first flow: the device obtains the ID token via the
// Google Sign-In SDK, then sends it to POST /auth/google/token.
func (s *AuthService) HandleGoogleIDToken(ctx context.Context, rawIDToken string) (*identitydomain.User, *TokenPair, bool, error) {
	if s.googleOAuth == nil {
		return nil, nil, false, ErrOAuthNotConfigured
	}

	payload, err := idtoken.Validate(ctx, rawIDToken, s.googleOAuth.ClientID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("invalid google id token: %w", err)
	}

	sub, _ := payload.Claims["sub"].(string)
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	if sub == "" || email == "" {
		return nil, nil, false, errors.New("google id token missing required claims")
	}

	user, isNew, err := s.userRepo.FindOrCreateByOAuth(ctx, "google", sub, email, name)
	if err != nil {
		return nil, nil, false, err
	}

	// Google ID token confirms ownership — auto-verify if not already done.
	needsSave := false
	if user.AvatarURL == nil && picture != "" {
		user.AvatarURL = &picture
		needsSave = true
	}
	if !user.IsEmailVerified() {
		now := time.Now().UTC()
		user.EmailVerifiedAt = &now
		needsSave = true
	}
	if needsSave {
		_ = s.userRepo.Update(ctx, user)
	}

	pair, err := s.issuePair(ctx, user.ID, nil)
	if err != nil {
		return nil, nil, false, err
	}
	return user, pair, isNew, nil
}

// ── Email verification ────────────────────────────────────────────────────────

const verificationTokenTTL = 24 * time.Hour

// VerifyEmail marks the user's email as verified using the token from the link.
func (s *AuthService) VerifyEmail(ctx context.Context, rawToken string) error {
	record, err := s.verificationRepo.FindByHash(ctx, hashToken(rawToken))
	if err != nil {
		return fmt.Errorf("verify email: lookup token: %w", err)
	}
	if record == nil || !record.IsValid() {
		return ErrInvalidVerificationToken
	}

	user, err := s.userRepo.FindByID(ctx, record.UserID)
	if err != nil || user == nil {
		return ErrInvalidVerificationToken
	}
	if user.IsEmailVerified() {
		_ = s.verificationRepo.MarkUsed(ctx, record.ID)
		return ErrAlreadyVerified
	}

	now := time.Now().UTC()
	user.EmailVerifiedAt = &now
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("verify email: update user: %w", err)
	}
	return s.verificationRepo.MarkUsed(ctx, record.ID)
}

// ResendVerification sends a fresh verification email to the authenticated user.
// Returns ErrAlreadyVerified if the email is already confirmed.
func (s *AuthService) ResendVerification(ctx context.Context, userID googleuuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return fmt.Errorf("resend verification: user not found")
	}
	if user.IsEmailVerified() {
		return ErrAlreadyVerified
	}
	return s.sendVerificationEmail(ctx, user)
}

// sendVerificationEmail generates a token and emails the verification link.
// Called on registration and on resend requests.
func (s *AuthService) sendVerificationEmail(ctx context.Context, user *identitydomain.User) error {
	_ = s.verificationRepo.InvalidatePrevious(ctx, user.ID)

	raw, err := generateResetToken() // reuse the same crypto/rand helper
	if err != nil {
		return fmt.Errorf("send verification email: generate token: %w", err)
	}

	id, _ := googleuuid.NewV7()
	record := &authdomain.EmailVerificationToken{
		ID:        id,
		UserID:    user.ID,
		TokenHash: hashToken(raw),
		ExpiresAt: time.Now().Add(verificationTokenTTL).UTC(),
	}
	if err := s.verificationRepo.Create(ctx, record); err != nil {
		return fmt.Errorf("send verification email: save token: %w", err)
	}

	link := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, raw)
	return s.emailSender.Send(ctx, email.Message{
		To:      user.Email,
		Subject: "Verify your Atmos email",
		HTML:    verificationEmailHTML(user.DisplayName, link),
		Text:    verificationEmailText(user.DisplayName, link),
	})
}

// ── Password reset ────────────────────────────────────────────────────────────

const resetTokenTTL = time.Hour

// ForgotPassword generates a reset token and emails the link to the user.
// Always returns nil — we never reveal whether the email exists.
func (s *AuthService) ForgotPassword(ctx context.Context, emailAddr string) error {
	user, err := s.userRepo.FindByEmail(ctx, emailAddr)
	if err != nil || user == nil {
		// Return nil so callers can't enumerate registered emails.
		return nil
	}
	// OAuth-only accounts have no password — silently skip.
	if user.PasswordHash == nil {
		return nil
	}

	// Invalidate any existing unused tokens for this user.
	_ = s.resetRepo.InvalidatePrevious(ctx, user.ID)

	// Generate a 32-byte cryptographically random token.
	raw, err := generateResetToken()
	if err != nil {
		return fmt.Errorf("forgot password: generate token: %w", err)
	}

	record := &authdomain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: hashToken(raw),
		ExpiresAt: time.Now().Add(resetTokenTTL).UTC(),
	}
	if err := s.resetRepo.Create(ctx, record); err != nil {
		return fmt.Errorf("forgot password: save token: %w", err)
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, raw)
	return s.emailSender.Send(ctx, email.Message{
		To:      emailAddr,
		Subject: "Reset your Atmos password",
		HTML:    passwordResetHTML(user.DisplayName, resetLink),
		Text:    passwordResetText(user.DisplayName, resetLink),
	})
}

// ResetPassword validates the token and sets a new password.
// On success, all refresh tokens for the user are revoked (force re-login everywhere).
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	record, err := s.resetRepo.FindByHash(ctx, hashToken(rawToken))
	if err != nil {
		return fmt.Errorf("reset password: lookup token: %w", err)
	}
	if record == nil || !record.IsValid() {
		return ErrInvalidResetToken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("reset password: hash password: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, record.UserID)
	if err != nil || user == nil {
		return ErrInvalidResetToken
	}

	hashStr := string(hash)
	user.PasswordHash = &hashStr
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("reset password: update user: %w", err)
	}

	// Consume the token so it cannot be reused.
	if err := s.resetRepo.MarkUsed(ctx, record.ID); err != nil {
		return fmt.Errorf("reset password: mark token used: %w", err)
	}

	// Revoke all active refresh tokens — user must log in again on all devices.
	_ = s.tokenRepo.RevokeAllForUser(ctx, record.UserID)

	return nil
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

func generateResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func passwordResetHTML(name, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:520px;margin:40px auto;color:#1a1a1a">
  <h2 style="margin-bottom:8px">Reset your password</h2>
  <p>Hi %s,</p>
  <p>We received a request to reset your Atmos password. Click the button below to set a new one. This link expires in <strong>1 hour</strong>.</p>
  <p style="margin:32px 0">
    <a href="%s" style="background:#16a34a;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:600">
      Reset password
    </a>
  </p>
  <p style="color:#666;font-size:13px">If you didn't request this, you can safely ignore this email — your password won't change.</p>
  <p style="color:#666;font-size:13px">Or copy this link: %s</p>
</body>
</html>`, name, link, link)
}

func passwordResetText(name, link string) string {
	return fmt.Sprintf(
		"Hi %s,\n\nReset your Atmos password here (link expires in 1 hour):\n%s\n\nIf you didn't request this, ignore this email.",
		name, link,
	)
}

func verificationEmailHTML(name, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:520px;margin:40px auto;color:#1a1a1a">
  <h2 style="margin-bottom:8px">Verify your email</h2>
  <p>Hi %s,</p>
  <p>Thanks for joining Atmos! Click the button below to verify your email address. This link expires in <strong>24 hours</strong>.</p>
  <p style="margin:32px 0">
    <a href="%s" style="background:#16a34a;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:600">
      Verify email
    </a>
  </p>
  <p style="color:#666;font-size:13px">If you didn't create an Atmos account, you can safely ignore this email.</p>
  <p style="color:#666;font-size:13px">Or copy this link: %s</p>
</body>
</html>`, name, link, link)
}

func verificationEmailText(name, link string) string {
	return fmt.Sprintf(
		"Hi %s,\n\nVerify your Atmos email here (link expires in 24 hours):\n%s\n\nIf you didn't create an account, ignore this email.",
		name, link,
	)
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
