package handler

import (
	"github.com/dipu/atmos-core/internal/gmail/repository"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

// ProviderHandler exposes the list of email-ingestion providers.
// These endpoints are public — no auth needed.
// The frontend uses them to render "Connect Gmail", "Ola — coming soon", etc.
type ProviderHandler struct {
	repo *repository.ProviderRepository
}

func NewProviderHandler(repo *repository.ProviderRepository) *ProviderHandler {
	return &ProviderHandler{repo: repo}
}

// ListActive godoc
// @Summary     List active providers
// @Description Returns providers that have at least one active email type.
//
//	These are the sources the user can connect right now.
//
// @Tags        providers
// @Produce     json
// @Success     200 {array}  domain.Provider
// @Failure     500 {object} map[string]interface{}
// @Router      /providers [get]
func (h *ProviderHandler) ListActive(c *fiber.Ctx) error {
	providers, err := h.repo.ActiveProviders(c.Context())
	if err != nil {
		return response.InternalError(c, "could not fetch providers")
	}
	return response.OK(c, providers)
}

// ListAll godoc
// @Summary     List all providers
// @Description Returns every provider including inactive ("coming soon") ones.
//
//	Use is_active to distinguish available from upcoming.
//
// @Tags        providers
// @Produce     json
// @Success     200 {array}  domain.Provider
// @Failure     500 {object} map[string]interface{}
// @Router      /providers/all [get]
func (h *ProviderHandler) ListAll(c *fiber.Ctx) error {
	providers, err := h.repo.AllProviders(c.Context())
	if err != nil {
		return response.InternalError(c, "could not fetch providers")
	}
	return response.OK(c, providers)
}
