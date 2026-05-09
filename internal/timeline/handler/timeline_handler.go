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

// GetDaily godoc
// @Summary     Get daily timeline summary
// @Description Returns the emission and distance summary for a given day, with trend vs. the previous day.
// @Description Defaults to today when the date query param is omitted. Returns zeros (not 404) for days with no recorded activities.
// @Tags        timeline
// @Produce     json
// @Security    BearerAuth
// @Param       date query    string false "Date in YYYY-MM-DD format (default: today in UTC)"
// @Success     200  {object} dto.DailySummaryResponse
// @Failure     400  {object} map[string]interface{}
// @Router      /timeline/daily [get]
func (h *TimelineHandler) GetDaily(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if d := c.Query("date"); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			return response.BadRequest(c, "invalid date format, use YYYY-MM-DD")
		}
		date = parsed
	}

	res, err := h.svc.GetDailySummary(c.Context(), userID, date)
	if err != nil {
		return response.InternalError(c, "failed to fetch daily summary")
	}
	return response.OK(c, res)
}

// GetWeekly godoc
// @Summary     Get weekly timeline summary
// @Description Returns the emission and distance summary for a given week (Mon–Sun), with trend vs. the previous week.
// @Description week_start must be a Monday. Defaults to the current week's Monday when omitted.
// @Tags        timeline
// @Produce     json
// @Security    BearerAuth
// @Param       week_start query    string false "Week start date in YYYY-MM-DD format (Monday)"
// @Success     200        {object} dto.WeeklySummaryResponse
// @Failure     400        {object} map[string]interface{}
// @Router      /timeline/weekly [get]
func (h *TimelineHandler) GetWeekly(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	weekStart := currentMonday()
	if ws := c.Query("week_start"); ws != "" {
		parsed, err := time.Parse("2006-01-02", ws)
		if err != nil {
			return response.BadRequest(c, "invalid week_start format, use YYYY-MM-DD (must be a Monday)")
		}
		if parsed.Weekday() != time.Monday {
			return response.BadRequest(c, "week_start must be a Monday")
		}
		weekStart = parsed
	}

	res, err := h.svc.GetWeeklySummary(c.Context(), userID, weekStart)
	if err != nil {
		return response.InternalError(c, "failed to fetch weekly summary")
	}
	return response.OK(c, res)
}

// GetMonthly godoc
// @Summary     Get monthly timeline summary
// @Description Returns the emission and distance summary for a given month, with trend vs. the previous month.
// @Description Defaults to the current month when year/month are omitted.
// @Tags        timeline
// @Produce     json
// @Security    BearerAuth
// @Param       year  query    int false "Year (e.g. 2025)"
// @Param       month query    int false "Month (1–12)"
// @Success     200   {object} dto.MonthlySummaryResponse
// @Failure     400   {object} map[string]interface{}
// @Router      /timeline/monthly [get]
func (h *TimelineHandler) GetMonthly(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	now := time.Now().UTC()
	year := now.Year()
	month := int(now.Month())

	if y := c.Query("year"); y != "" {
		v, err := strconv.Atoi(y)
		if err != nil || v < 2000 || v > 2100 {
			return response.BadRequest(c, "invalid year")
		}
		year = v
	}
	if m := c.Query("month"); m != "" {
		v, err := strconv.Atoi(m)
		if err != nil || v < 1 || v > 12 {
			return response.BadRequest(c, "month must be between 1 and 12")
		}
		month = v
	}

	res, err := h.svc.GetMonthlySummary(c.Context(), userID, year, month)
	if err != nil {
		return response.InternalError(c, "failed to fetch monthly summary")
	}
	return response.OK(c, res)
}

// --- legacy path-param handlers (kept for backward compatibility) ---

func (h *TimelineHandler) GetDay(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	date, err := time.Parse("2006-01-02", c.Params("date"))
	if err != nil {
		return response.BadRequest(c, "invalid date format, use YYYY-MM-DD")
	}

	summary, err := h.svc.GetDay(c.Context(), userID, date)
	if err != nil {
		return response.OK(c, fiber.Map{
			"date_local":        date.Format("2006-01-02"),
			"total_kg_co2e":     0,
			"total_distance_km": 0,
			"activity_count":    0,
			"breakdown":         fiber.Map{},
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

// currentMonday returns Monday of the current UTC week.
func currentMonday() time.Time {
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday → 7 so Monday offset is weekday-1
	}
	return now.AddDate(0, 0, -(weekday - 1)).Truncate(24 * time.Hour)
}
