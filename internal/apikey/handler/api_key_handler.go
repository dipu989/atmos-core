package handler

import (
	"errors"

	"github.com/dipu/atmos-core/internal/apikey/dto"
	"github.com/dipu/atmos-core/internal/apikey/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type APIKeyHandler struct {
	svc *service.APIKeyService
}

func NewAPIKeyHandler(svc *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

// Create godoc
// @Summary     Create an API key
// @Description Generates a new API key for the authenticated user. The raw key is returned
// @Description once in this response and cannot be retrieved again. Store it securely.
// @Tags        api-keys
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.CreateAPIKeyRequest true "Key name"
// @Success     201  {object} dto.CreateAPIKeyResponse
// @Failure     400  {object} map[string]interface{}
// @Failure     409  {object} map[string]interface{}
// @Router      /users/me/api-keys [post]
func (h *APIKeyHandler) Create(c *fiber.Ctx) error {
	var req dto.CreateAPIKeyRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	resp, err := h.svc.Create(c.Context(), userID, req.Name)
	if err != nil {
		if errors.Is(err, service.ErrLimitReached) {
			return response.Conflict(c, "api key limit reached (max 10)")
		}
		return response.InternalError(c, "failed to create api key")
	}
	return response.Created(c, resp)
}

// List godoc
// @Summary     List API keys
// @Description Returns all active API keys for the authenticated user. Raw keys are never returned.
// @Tags        api-keys
// @Produce     json
// @Security    BearerAuth
// @Success     200 {array}  dto.APIKeyItem
// @Failure     500 {object} map[string]interface{}
// @Router      /users/me/api-keys [get]
func (h *APIKeyHandler) List(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	items, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "failed to list api keys")
	}
	return response.OK(c, items)
}

// Revoke godoc
// @Summary     Revoke an API key
// @Description Permanently revokes an API key. This cannot be undone.
// @Tags        api-keys
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "API key UUID"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} map[string]interface{}
// @Failure     404 {object} map[string]interface{}
// @Router      /users/me/api-keys/{id} [delete]
func (h *APIKeyHandler) Revoke(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid api key id")
	}

	userID := middleware.CurrentUserID(c)
	if err := h.svc.Revoke(c.Context(), id, userID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return response.NotFound(c, "api key not found")
		}
		return response.InternalError(c, "failed to revoke api key")
	}
	return response.OK(c, fiber.Map{"message": "api key revoked"})
}
