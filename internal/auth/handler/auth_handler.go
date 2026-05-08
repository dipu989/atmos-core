package handler

import (
	"errors"

	"github.com/dipu/atmos-core/internal/auth/dto"
	"github.com/dipu/atmos-core/internal/auth/service"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
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

	// Store CSRF state in a short-lived cookie (10 min, HttpOnly)
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		MaxAge:   600,
		HTTPOnly: true,
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
	code := c.Query("code")
	state := c.Query("state")
	expectedState := c.Cookies("oauth_state")

	if code == "" || state == "" {
		return response.BadRequest(c, "missing code or state parameter")
	}
	if state != expectedState {
		return response.Unauthorized(c, "invalid OAuth state — possible CSRF attempt")
	}

	// Clear the state cookie
	c.Cookie(&fiber.Cookie{
		Name:   "oauth_state",
		Value:  "",
		MaxAge: -1,
	})

	user, pair, isNew, err := h.svc.HandleGoogleCallback(c.Context(), code)
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
