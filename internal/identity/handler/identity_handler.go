package handler

import (
	"github.com/dipu/atmos-core/internal/identity/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

type IdentityHandler struct {
	svc *service.IdentityService
}

func NewIdentityHandler(svc *service.IdentityService) *IdentityHandler {
	return &IdentityHandler{svc: svc}
}

func (h *IdentityHandler) GetMe(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	user, err := h.svc.GetProfile(c.Context(), userID)
	if err != nil {
		return response.NotFound(c, "user not found")
	}
	return response.OK(c, user)
}

func (h *IdentityHandler) UpdateMe(c *fiber.Ctx) error {
	var req struct {
		DisplayName string  `json:"display_name"`
		Timezone    string  `json:"timezone"`
		Locale      string  `json:"locale"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	userID := middleware.CurrentUserID(c)
	user, err := h.svc.UpdateProfile(c.Context(), userID, req.DisplayName, req.Timezone, req.Locale, req.AvatarURL)
	if err != nil {
		return response.InternalError(c, "failed to update profile")
	}
	return response.OK(c, user)
}
