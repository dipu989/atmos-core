package handler

import (
	"errors"

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

// DeleteAccount godoc
// @Summary     Delete account
// @Description Soft-deletes the authenticated user's account.
//
//	The account enters a 7-day grace period during which it can be recovered
//	by simply logging in again. After 7 days, all data is permanently erased.
//
//	Email+password accounts: supply current password in "password" field.
//	OAuth-only accounts: supply the exact string "delete my account" in "confirmation" field.
//
// @Tags        users
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.DeleteAccountRequest true "Confirmation payload"
// @Success     200  {object} map[string]interface{}
// @Failure     400  {object} map[string]interface{}
// @Failure     401  {object} map[string]interface{}
// @Router      /users/me [delete]
func (h *IdentityHandler) DeleteAccount(c *fiber.Ctx) error {
	var req dto.DeleteAccountRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	if err := h.svc.DeleteAccount(c.Context(), userID, req.Confirmation); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidConfirmation):
			return response.BadRequest(c, `type the word "delete" to confirm`)
		case errors.Is(err, service.ErrNotFound):
			return response.NotFound(c, "user not found")
		default:
			return response.InternalError(c, "could not delete account")
		}
	}

	return response.OK(c, fiber.Map{
		"message": "your account has been scheduled for deletion — you have 7 days to log back in and recover it",
	})
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
		HomeAddress:              req.HomeAddress,
		HomeLat:                  req.HomeLat,
		HomeLng:                  req.HomeLng,
		WorkAddress:              req.WorkAddress,
		WorkLat:                  req.WorkLat,
		WorkLng:                  req.WorkLng,
		DefaultTransport:         req.DefaultTransport,
	})
	if err != nil {
		return response.InternalError(c, "failed to update preferences")
	}
	return response.OK(c, prefs)
}
