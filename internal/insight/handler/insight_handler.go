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

func (h *InsightHandler) ListInsights(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	onlyUnread := c.QueryBool("unread", false)
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	insights, err := h.svc.ListInsights(c.Context(), userID, onlyUnread, limit, offset)
	if err != nil {
		return response.InternalError(c, "failed to fetch insights")
	}
	return response.OK(c, insights)
}

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
