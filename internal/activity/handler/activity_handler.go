package handler

import (
	"errors"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/dipu/atmos-core/internal/activity/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ActivityHandler struct {
	svc *service.ActivityService
}

func NewActivityHandler(svc *service.ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

func (h *ActivityHandler) Ingest(c *fiber.Ctx) error {
	var req struct {
		ActivityType    string                 `json:"activity_type"`
		TransportMode   *string                `json:"transport_mode"`
		DistanceKM      *float64               `json:"distance_km"`
		DurationMinutes *int                   `json:"duration_minutes"`
		Source          string                 `json:"source"`
		Provider        *string                `json:"provider"`
		Metadata        map[string]any         `json:"metadata"`
		StartedAt       time.Time              `json:"started_at"`
		EndedAt         *time.Time             `json:"ended_at"`
		IdempotencyKey  string                 `json:"idempotency_key"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.ActivityType == "" || req.StartedAt.IsZero() {
		return response.BadRequest(c, "activity_type and started_at are required")
	}

	userID := middleware.CurrentUserID(c)

	// Pull user timezone from locals (set by a future profile middleware);
	// fall back to UTC for now.
	timezone, _ := c.Locals("userTimezone").(string)
	if timezone == "" {
		timezone = "UTC"
	}

	var mode *actdomain.TransportMode
	if req.TransportMode != nil {
		m := actdomain.TransportMode(*req.TransportMode)
		mode = &m
	}

	input := service.IngestInput{
		UserID:          userID,
		ActivityType:    actdomain.ActivityType(req.ActivityType),
		TransportMode:   mode,
		DistanceKM:      req.DistanceKM,
		DurationMinutes: req.DurationMinutes,
		Source:          actdomain.ActivitySource(req.Source),
		Provider:        req.Provider,
		RawMetadata:     actdomain.RawMetadata(req.Metadata),
		StartedAt:       req.StartedAt,
		EndedAt:         req.EndedAt,
		UserTimezone:    timezone,
		IdempotencyKey:  req.IdempotencyKey,
	}

	activity, err := h.svc.Ingest(c.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDuplicate) {
			return response.Conflict(c, "duplicate activity")
		}
		return response.InternalError(c, "failed to ingest activity")
	}
	return response.Created(c, activity)
}

func (h *ActivityHandler) GetActivity(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid activity id")
	}
	userID := middleware.CurrentUserID(c)
	activity, err := h.svc.GetActivity(c.Context(), id, userID)
	if err != nil {
		return response.NotFound(c, "activity not found")
	}
	return response.OK(c, activity)
}

func (h *ActivityHandler) ListActivities(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	fromStr := c.Query("from")
	toStr := c.Query("to")

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	activities, err := h.svc.ListActivities(c.Context(), userID, from, to, limit, offset)
	if err != nil {
		return response.InternalError(c, "failed to list activities")
	}
	return response.OK(c, activities)
}
