package handler

import (
	"github.com/dipu/atmos-core/internal/identity/dto"
	"github.com/dipu/atmos-core/internal/identity/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
)

type IdentityHandler struct {
	svc *service.IdentityService
}

func NewIdentityHandler(svc *service.IdentityService) *IdentityHandler {
	return &IdentityHandler{svc: svc}
}

// GetMe godoc
// @Summary     Get current user
// @Description Returns the authenticated user's profile
// @Tags        users
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} domain.User
// @Failure     404 {object} map[string]interface{}
// @Router      /users/me [get]
func (h *IdentityHandler) GetMe(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	user, err := h.svc.GetProfile(c.Context(), userID)
	if err != nil {
		return response.NotFound(c, "user not found")
	}
	return response.OK(c, user)
}

// UpdateMe godoc
// @Summary     Update current user profile
// @Description Updates display name, timezone, locale, or avatar URL
// @Tags        users
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.UpdateProfileRequest true "Profile update payload"
// @Success     200  {object} domain.User
// @Failure     400  {object} map[string]interface{}
// @Failure     500  {object} map[string]interface{}
// @Router      /users/me [put]
func (h *IdentityHandler) UpdateMe(c *fiber.Ctx) error {
	var req dto.UpdateProfileRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	user, err := h.svc.UpdateProfile(c.Context(), userID, req.DisplayName, req.Timezone, req.Locale, req.AvatarURL)
	if err != nil {
		return response.InternalError(c, "failed to update profile")
	}
	return response.OK(c, user)
}

// GetPreferences godoc
// @Summary     Get user preferences
// @Description Returns the authenticated user's preferences, creating defaults if none exist
// @Tags        users
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} domain.UserPreferences
// @Failure     500 {object} map[string]interface{}
// @Router      /users/me/preferences [get]
func (h *IdentityHandler) GetPreferences(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	prefs, err := h.svc.GetPreferences(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "failed to get preferences")
	}
	return response.OK(c, prefs)
}

// UpdatePreferences godoc
// @Summary     Update user preferences
// @Description Partially updates preferences — only provided fields are changed
// @Tags        users
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.UpdatePreferencesRequest true "Preferences payload"
// @Success     200  {object} domain.UserPreferences
// @Failure     400  {object} map[string]interface{}
// @Failure     500  {object} map[string]interface{}
// @Router      /users/me/preferences [put]
func (h *IdentityHandler) UpdatePreferences(c *fiber.Ctx) error {
	var req dto.UpdatePreferencesRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	var distanceUnit *string
	if req.DistanceUnit != "" {
		distanceUnit = &req.DistanceUnit
	}
	prefs, err := h.svc.UpdatePreferences(c.Context(), userID, service.UpdatePreferencesInput{
		DistanceUnit:             distanceUnit,
		PushNotificationsEnabled: req.PushNotificationsEnabled,
		WeeklyReportEnabled:      req.WeeklyReportEnabled,
		DailyGoalKgCO2e:          req.DailyGoalKgCO2e,
		DataSharingEnabled:       req.DataSharingEnabled,
	})
	if err != nil {
		return response.InternalError(c, "failed to update preferences")
	}
	return response.OK(c, prefs)
}
