package handler

import (
	"strconv"
	"time"

	"github.com/dipu/atmos-core/internal/timeline/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

type TimelineHandler struct {
	svc *service.TimelineService
}

func NewTimelineHandler(svc *service.TimelineService) *TimelineHandler {
	return &TimelineHandler{svc: svc}
}

func (h *TimelineHandler) GetDay(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	date, err := time.Parse("2006-01-02", c.Params("date"))
	if err != nil {
		return response.BadRequest(c, "invalid date format, use YYYY-MM-DD")
	}

	summary, err := h.svc.GetDay(c.Context(), userID, date)
	if err != nil {
		// Return an empty summary rather than a 404 for days with no data
		return response.OK(c, fiber.Map{
			"date_local":       date.Format("2006-01-02"),
			"total_kg_co2e":    0,
			"total_distance_km": 0,
			"activity_count":   0,
			"breakdown":        fiber.Map{},
		})
	}
	return response.OK(c, summary)
}

func (h *TimelineHandler) GetWeek(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	weekStart, err := time.Parse("2006-01-02", c.Params("week_start"))
	if err != nil {
		return response.BadRequest(c, "invalid week_start format, use YYYY-MM-DD (Monday)")
	}

	summary, err := h.svc.GetWeek(c.Context(), userID, weekStart)
	if err != nil {
		return response.NotFound(c, "no data for this week")
	}
	return response.OK(c, summary)
}

func (h *TimelineHandler) GetMonth(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	year, err := strconv.Atoi(c.Params("year"))
	if err != nil {
		return response.BadRequest(c, "invalid year")
	}
	month, err := strconv.Atoi(c.Params("month"))
	if err != nil || month < 1 || month > 12 {
		return response.BadRequest(c, "invalid month")
	}

	summary, err := h.svc.GetMonth(c.Context(), userID, year, month)
	if err != nil {
		return response.NotFound(c, "no data for this month")
	}
	return response.OK(c, summary)
}

func (h *TimelineHandler) GetRange(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	from, err := time.Parse("2006-01-02", c.Query("from"))
	if err != nil {
		return response.BadRequest(c, "from is required in YYYY-MM-DD format")
	}
	to, err := time.Parse("2006-01-02", c.Query("to"))
	if err != nil {
		return response.BadRequest(c, "to is required in YYYY-MM-DD format")
	}
	if to.Before(from) {
		return response.BadRequest(c, "to must be after from")
	}
	if to.Sub(from) > 90*24*time.Hour {
		return response.BadRequest(c, "range cannot exceed 90 days")
	}

	summaries, err := h.svc.GetRange(c.Context(), userID, from, to)
	if err != nil {
		return response.InternalError(c, "failed to fetch timeline")
	}
	return response.OK(c, summaries)
}
