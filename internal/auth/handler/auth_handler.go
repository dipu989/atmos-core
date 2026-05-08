package handler

import (
	"errors"

	"github.com/dipu/atmos-core/internal/auth/service"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "email and password are required")
	}
	if len(req.Password) < 8 {
		return response.BadRequest(c, "password must be at least 8 characters")
	}

	user, pair, err := h.svc.Register(c.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return response.Conflict(c, "email already registered")
		}
		return response.InternalError(c, "registration failed")
	}

	return response.Created(c, fiber.Map{
		"user":          user,
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "email and password are required")
	}

	pair, err := h.svc.Login(c.Context(), req.Email, req.Password, nil)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return response.Unauthorized(c, "invalid email or password")
		}
		return response.InternalError(c, "login failed")
	}

	return response.OK(c, pair)
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil || req.RefreshToken == "" {
		return response.BadRequest(c, "refresh_token is required")
	}

	pair, err := h.svc.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return response.Unauthorized(c, "invalid or expired refresh token")
	}

	return response.OK(c, pair)
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil || req.RefreshToken == "" {
		return response.BadRequest(c, "refresh_token is required")
	}

	_ = h.svc.Logout(c.Context(), req.RefreshToken)
	return response.NoContent(c)
}
