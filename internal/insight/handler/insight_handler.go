package handler

import (
	"github.com/dipu/atmos-core/internal/insight/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type InsightHandler struct {
	svc *service.InsightService
}

func NewInsightHandler(svc *service.InsightService) *InsightHandler {
	return &InsightHandler{svc: svc}
}

// ListInsights godoc
// @Summary     List insights
// @Description Returns a paginated list of insights for the authenticated user
// @Tags        insights
// @Produce     json
// @Param       unread query    bool   false "Return only unread insights"
// @Param       limit  query    int    false "Page size (1-50, default 20)"
// @Param       offset query    int    false "Offset for pagination"
// @Success     200    {object} dto.InsightsPage
// @Failure     401    {object} map[string]interface{}
// @Failure     500    {object} map[string]interface{}
// @Security    BearerAuth
// @Router      /insights [get]
func (h *InsightHandler) ListInsights(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	onlyUnread := c.QueryBool("unread", false)
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	page, err := h.svc.ListInsights(c.Context(), userID, onlyUnread, limit, offset)
	if err != nil {
		return response.InternalError(c, "failed to fetch insights")
	}
	return response.OK(c, page)
}

// GetInsight godoc
// @Summary     Get insight
// @Description Returns a single insight by ID
// @Tags        insights
// @Produce     json
// @Param       id  path     string true "Insight UUID"
// @Success     200 {object} domain.Insight
// @Failure     400 {object} map[string]interface{}
// @Failure     404 {object} map[string]interface{}
// @Security    BearerAuth
// @Router      /insights/{id} [get]
func (h *InsightHandler) GetInsight(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid insight id")
	}
	userID := middleware.CurrentUserID(c)
	insight, err := h.svc.GetInsight(c.Context(), id, userID)
	if err != nil {
		return response.NotFound(c, "insight not found")
	}
	return response.OK(c, insight)
}

// MarkRead godoc
// @Summary     Mark insight as read
// @Description Marks the given insight as read for the authenticated user
// @Tags        insights
// @Produce     json
// @Param       id  path string true "Insight UUID"
// @Success     204
// @Failure     400 {object} map[string]interface{}
// @Failure     500 {object} map[string]interface{}
// @Security    BearerAuth
// @Router      /insights/{id}/read [patch]
func (h *InsightHandler) MarkRead(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid insight id")
	}
	userID := middleware.CurrentUserID(c)
	if err := h.svc.MarkRead(c.Context(), id, userID); err != nil {
		return response.InternalError(c, "failed to mark insight as read")
	}
	return response.NoContent(c)
}
