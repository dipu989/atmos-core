package handler

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/dipu/atmos-core/internal/auth/dto"
	"github.com/dipu/atmos-core/internal/auth/service"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type AuthHandler struct {
	svc         *service.AuthService
	frontendURL string // where to redirect after OAuth (e.g. https://atmosapp.dev)
}

func NewAuthHandler(svc *service.AuthService, frontendURL string) *AuthHandler {
	return &AuthHandler{svc: svc, frontendURL: frontendURL}
}

// Register godoc
// @Summary     Register a new user
// @Description Create a new account with email and password
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.RegisterRequest true "Registration payload"
// @Success     201  {object} dto.AuthResponse
// @Failure     400  {object} map[string]interface{}
// @Failure     409  {object} map[string]interface{}
// @Router      /auth/register [post]
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	user, pair, err := h.svc.Register(c.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return response.Conflict(c, "email already registered")
		}
		return response.InternalError(c, "registration failed")
	}

	return response.Created(c, dto.AuthResponse{
		User:         user,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	})
}

// Login godoc
// @Summary     Login
// @Description Authenticate with email and password
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.LoginRequest true "Login payload"
// @Success     200  {object} dto.TokenPairResponse
// @Failure     400  {object} map[string]interface{}
// @Failure     401  {object} map[string]interface{}
// @Router      /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req dto.LoginRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	pair, err := h.svc.Login(c.Context(), req.Email, req.Password, nil)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return response.Unauthorized(c, "invalid email or password")
		}
		return response.InternalError(c, "login failed")
	}

	return response.OK(c, dto.TokenPairResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	})
}

// Refresh godoc
// @Summary     Refresh access token
// @Description Rotate refresh token and issue a new token pair
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.RefreshRequest true "Refresh payload"
// @Success     200  {object} dto.TokenPairResponse
// @Failure     400  {object} map[string]interface{}
// @Failure     401  {object} map[string]interface{}
// @Router      /auth/refresh [post]
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req dto.RefreshRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	pair, err := h.svc.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return response.Unauthorized(c, "invalid or expired refresh token")
	}

	return response.OK(c, dto.TokenPairResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	})
}

// Logout godoc
// @Summary     Logout
// @Description Revoke the provided refresh token
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body dto.LogoutRequest true "Logout payload"
// @Success     204
// @Failure     400 {object} map[string]interface{}
// @Router      /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req dto.LogoutRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	_ = h.svc.Logout(c.Context(), req.RefreshToken)
	return response.NoContent(c)
}

// GoogleTokenLogin godoc
// @Summary     Google Sign-In (mobile)
// @Description Verifies a Google ID token from the native mobile SDK and returns a JWT pair
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.GoogleTokenRequest true "Google ID token"
// @Success     200  {object} dto.GoogleCallbackResponse
// @Failure     400  {object} map[string]interface{}
// @Failure     401  {object} map[string]interface{}
// @Router      /auth/google/token [post]
func (h *AuthHandler) GoogleTokenLogin(c *fiber.Ctx) error {
	var req dto.GoogleTokenRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	user, pair, isNew, err := h.svc.HandleGoogleIDToken(c.Context(), req.IDToken)
	if err != nil {
		if errors.Is(err, service.ErrOAuthNotConfigured) {
			return response.ServiceUnavailable(c, "Google OAuth is not configured")
		}
		return response.Unauthorized(c, "Google authentication failed")
	}

	return response.OK(c, dto.GoogleCallbackResponse{
		User:         user,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		IsNewUser:    isNew,
	})
}

// VerifyEmail godoc
// @Summary     Verify email address
// @Description Confirms ownership of the email address using the token from the
//
//	verification link. Safe to call multiple times — returns 200 even if already verified.
//
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.VerifyEmailRequest true "Verification token"
// @Success     200  {object} map[string]interface{}
// @Failure     400  {object} map[string]interface{}
// @Router      /auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(c *fiber.Ctx) error {
	var req dto.VerifyEmailRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.svc.VerifyEmail(c.Context(), req.Token); err != nil {
		if errors.Is(err, service.ErrAlreadyVerified) {
			return response.OK(c, fiber.Map{"message": "email already verified"})
		}
		if errors.Is(err, service.ErrInvalidVerificationToken) {
			return response.BadRequest(c, "invalid or expired verification token")
		}
		return response.InternalError(c, "could not verify email")
	}
	return response.OK(c, fiber.Map{"message": "email verified successfully"})
}

// ResendVerification godoc
// @Summary     Resend verification email
// @Description Sends a fresh verification link to the authenticated user's email.
//
//	Returns 200 immediately if the email is already verified.
//
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Router      /auth/resend-verification [post]
func (h *AuthHandler) ResendVerification(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	if err := h.svc.ResendVerification(c.Context(), userID); err != nil {
		if errors.Is(err, service.ErrAlreadyVerified) {
			return response.OK(c, fiber.Map{"message": "email already verified"})
		}
		return response.InternalError(c, "could not send verification email")
	}
	return response.OK(c, fiber.Map{"message": "verification email sent"})
}

// ForgotPassword godoc
// @Summary     Request a password reset email
// @Description Sends a reset link to the given email if an account exists.
//
//	Always returns 200 — the response never reveals whether the email is registered.
//
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.ForgotPasswordRequest true "Email address"
// @Success     200  {object} map[string]interface{}
// @Failure     400  {object} map[string]interface{}
// @Router      /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	var req dto.ForgotPasswordRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}
	// Ignore the error — we never reveal whether the email exists.
	_ = h.svc.ForgotPassword(c.Context(), req.Email)
	return response.OK(c, fiber.Map{"message": "if that email is registered, a reset link is on its way"})
}

// ResetPassword godoc
// @Summary     Reset password using a token
// @Description Validates the token from the reset email and sets a new password.
//
//	On success, all existing sessions are revoked — the user must log in again.
//
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     dto.ResetPasswordRequest true "Token and new password"
// @Success     200  {object} map[string]interface{}
// @Failure     400  {object} map[string]interface{}
// @Router      /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req dto.ResetPasswordRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.svc.ResetPassword(c.Context(), req.Token, req.Password); err != nil {
		if errors.Is(err, service.ErrInvalidResetToken) {
			return response.BadRequest(c, "invalid or expired reset token")
		}
		return response.InternalError(c, "could not reset password")
	}
	return response.OK(c, fiber.Map{"message": "password updated — please log in again"})
}

// GoogleLogin godoc
// @Summary     Google OAuth — initiate login
// @Description Redirects the client to Google's OAuth consent screen
// @Tags        auth
// @Produce     json
// @Success     302
// @Failure     503 {object} map[string]interface{}
// @Router      /auth/google/login [get]
func (h *AuthHandler) GoogleLogin(c *fiber.Ctx) error {
	url, state, err := h.svc.GoogleAuthURL()
	if err != nil {
		return response.ServiceUnavailable(c, "Google OAuth is not configured")
	}

	// Store CSRF state in a short-lived cookie (10 min, HttpOnly, Secure)
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		MaxAge:   600,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	return c.Redirect(url, fiber.StatusTemporaryRedirect)
}

// GoogleCallback godoc
// @Summary     Google OAuth — callback
// @Description Handles the OAuth callback, exchanges the code, and returns a JWT pair
// @Tags        auth
// @Produce     json
// @Param       code  query    string true "Authorization code from Google"
// @Param       state query    string true "CSRF state"
// @Success     200   {object} dto.GoogleCallbackResponse
// @Failure     400   {object} map[string]interface{}
// @Failure     401   {object} map[string]interface{}
// @Router      /auth/google/callback [get]
func (h *AuthHandler) GoogleCallback(c *fiber.Ctx) error {
	log := logger.L()
	errRedirect := fmt.Sprintf("%s/login?error=oauth_failed", h.frontendURL)

	code := c.Query("code")
	state := c.Query("state")
	expectedState := c.Cookies("oauth_state")

	log.Info("google oauth callback",
		zap.Bool("has_code", code != ""),
		zap.Bool("has_state", state != ""),
		zap.Bool("has_cookie", expectedState != ""),
		zap.Bool("state_match", state == expectedState),
	)

	if code == "" || state == "" || state != expectedState {
		log.Warn("google oauth state check failed",
			zap.Bool("code_empty", code == ""),
			zap.Bool("state_empty", state == ""),
			zap.Bool("state_mismatch", state != expectedState),
		)
		return c.Redirect(errRedirect, fiber.StatusTemporaryRedirect)
	}

	// Clear the state cookie
	c.Cookie(&fiber.Cookie{
		Name:   "oauth_state",
		Value:  "",
		MaxAge: -1,
	})

	_, pair, _, err := h.svc.HandleGoogleCallback(c.Context(), code)
	if err != nil {
		log.Error("google oauth code exchange failed", zap.Error(err))
		return c.Redirect(errRedirect, fiber.StatusTemporaryRedirect)
	}

	// Redirect to frontend callback page with tokens in query params.
	// Tokens are short-lived JWTs — the frontend strips them from the URL immediately.
	redirectURL := fmt.Sprintf(
		"%s/auth/callback?access_token=%s&refresh_token=%s",
		h.frontendURL,
		url.QueryEscape(pair.AccessToken),
		url.QueryEscape(pair.RefreshToken),
	)
	return c.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
}
