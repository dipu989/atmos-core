package handler

import (
	"errors"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/dipu/atmos-core/internal/activity/dto"
	"github.com/dipu/atmos-core/internal/activity/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ActivityHandler struct {
	svc *service.ActivityService
}

func NewActivityHandler(svc *service.ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

// Ingest godoc
// @Summary     Ingest an activity
// @Description Records a transport or flight activity and triggers async emission calculation.
// @Description activity_type is derived automatically from transport_mode — do not send it.
// @Tags        activities
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.IngestActivityRequest true "Activity payload"
// @Success     201  {object} domain.Activity
// @Failure     400  {object} map[string]interface{}
// @Failure     409  {object} map[string]interface{}
// @Router      /activities [post]
func (h *ActivityHandler) Ingest(c *fiber.Ctx) error {
	var req dto.IngestActivityRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)

	timezone, _ := c.Locals("userTimezone").(string)
	if timezone == "" {
		timezone = "UTC"
	}

	mode := actdomain.TransportMode(req.TransportMode)
	input := service.IngestInput{
		UserID:          userID,
		ActivityType:    modeToActivityType(req.TransportMode),
		TransportMode:   &mode,
		DistanceKM:      req.DistanceKM,
		DurationMinutes: req.DurationMinutes,
		Source:          actdomain.ActivitySource(req.Source),
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

// GetActivity godoc
// @Summary     Get an activity
// @Description Returns a single activity by ID (must belong to the authenticated user)
// @Tags        activities
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Activity UUID"
// @Success     200 {object} domain.Activity
// @Failure     400 {object} map[string]interface{}
// @Failure     404 {object} map[string]interface{}
// @Router      /activities/{id} [get]
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

// ListActivities godoc
// @Summary     List activities
// @Description Returns a paginated list of activities for the authenticated user.
// @Description Defaults to the last 30 days when from/to are omitted.
// @Tags        activities
// @Produce     json
// @Security    BearerAuth
// @Param       from   query    string false "Start date (YYYY-MM-DD)"
// @Param       to     query    string false "End date (YYYY-MM-DD)"
// @Param       limit  query    int    false "Page size (default 50, max 100)"
// @Param       offset query    int    false "Page offset (default 0)"
// @Success     200    {object} dto.ActivitiesPage
// @Failure     500    {object} map[string]interface{}
// @Router      /activities [get]
func (h *ActivityHandler) ListActivities(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()

	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	if limit < 1 || limit > 100 {
		limit = 50
	}

	activities, total, err := h.svc.ListActivities(c.Context(), userID, from, to, limit, offset)
	if err != nil {
		return response.InternalError(c, "failed to list activities")
	}

	return response.OK(c, dto.ActivitiesPage{
		Activities: activities,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	})
}

// UpdateActivity godoc
// @Summary     Update an activity
// @Description Partially updates a manual activity. Only transport_mode, distance_km,
//
//	duration_minutes, and started_at can be changed. Source and provider are immutable.
//	Emission and timeline are recalculated automatically.
//
// @Tags        activities
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path     string                     true "Activity UUID"
// @Param       body body     dto.UpdateActivityRequest  true "Fields to update"
// @Success     200  {object} domain.Activity
// @Failure     400  {object} map[string]interface{}
// @Failure     404  {object} map[string]interface{}
// @Router      /activities/{id} [patch]
func (h *ActivityHandler) UpdateActivity(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid activity id")
	}

	var req dto.UpdateActivityRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	timezone, _ := c.Locals("userTimezone").(string)
	if timezone == "" {
		timezone = "UTC"
	}

	var mode *actdomain.TransportMode
	if req.TransportMode != nil {
		m := actdomain.TransportMode(*req.TransportMode)
		mode = &m
	}

	activity, err := h.svc.UpdateActivity(c.Context(), id, userID, service.UpdateInput{
		TransportMode:   mode,
		DistanceKM:      req.DistanceKM,
		DurationMinutes: req.DurationMinutes,
		StartedAt:       req.StartedAt,
		UserTimezone:    timezone,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return response.NotFound(c, "activity not found")
		}
		return response.InternalError(c, "failed to update activity")
	}
	return response.OK(c, activity)
}

// DeleteActivity godoc
// @Summary     Delete an activity
// @Description Permanently removes an activity and recalculates the affected day's
//
//	emission summary. This action cannot be undone.
//
// @Tags        activities
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Activity UUID"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} map[string]interface{}
// @Failure     404 {object} map[string]interface{}
// @Router      /activities/{id} [delete]
func (h *ActivityHandler) DeleteActivity(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid activity id")
	}
	userID := middleware.CurrentUserID(c)
	if err := h.svc.DeleteActivity(c.Context(), id, userID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return response.NotFound(c, "activity not found")
		}
		return response.InternalError(c, "failed to delete activity")
	}
	return response.OK(c, fiber.Map{"message": "activity deleted"})
}

// modeToActivityType derives ActivityType from the transport mode.
// "flight" is its own type; everything else is "transport".
func modeToActivityType(mode string) actdomain.ActivityType {
	if mode == "flight" {
		return actdomain.ActivityFlight
	}
	return actdomain.ActivityTransport
}
