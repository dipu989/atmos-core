package handler

import (
	"github.com/dipu/atmos-core/internal/device/domain"
	"github.com/dipu/atmos-core/internal/device/dto"
	"github.com/dipu/atmos-core/internal/device/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/dipu/atmos-core/platform/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type DeviceHandler struct {
	svc *service.DeviceService
}

func NewDeviceHandler(svc *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// Register godoc
// @Summary     Register or update a device
// @Description Registers a new device or updates an existing one (upsert by device_token).
// @Description iOS/iPadOS require push_provider=apns and apns_environment.
// @Description Android requires push_provider=fcm or none.
// @Tags        devices
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     dto.RegisterDeviceRequest true "Device registration payload"
// @Success     201  {object} domain.Device
// @Failure     400  {object} map[string]interface{}
// @Router      /devices/register [post]
func (h *DeviceHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterDeviceRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	var apnsEnv *domain.APNsEnvironment
	if req.APNsEnvironment != nil {
		e := domain.APNsEnvironment(*req.APNsEnvironment)
		apnsEnv = &e
	}

	pushProvider := domain.PushProvider(req.PushProvider)
	if pushProvider == "" {
		pushProvider = domain.PushProviderNone
	}

	input := service.RegisterInput{
		UserID:          middleware.CurrentUserID(c),
		DeviceToken:     req.DeviceToken,
		Platform:        domain.Platform(req.Platform),
		PushProvider:    pushProvider,
		APNsEnvironment: apnsEnv,
		DeviceName:      req.DeviceName,
		OSVersion:       req.OSVersion,
		AppVersion:      req.AppVersion,
		PushToken:       req.PushToken,
	}

	device, err := h.svc.Register(c.Context(), input)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}
	return response.Created(c, device)
}

// List godoc
// @Summary     List active devices
// @Description Returns all active devices registered by the authenticated user
// @Tags        devices
// @Produce     json
// @Security    BearerAuth
// @Success     200 {array}  domain.Device
// @Failure     500 {object} map[string]interface{}
// @Router      /devices [get]
func (h *DeviceHandler) List(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	devices, err := h.svc.ListDevices(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "failed to list devices")
	}
	return response.OK(c, devices)
}

// Deregister godoc
// @Summary     Deregister a device
// @Description Marks a device as inactive (soft deactivation)
// @Tags        devices
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Device UUID"
// @Success     204
// @Failure     400 {object} map[string]interface{}
// @Failure     500 {object} map[string]interface{}
// @Router      /devices/{id} [delete]
func (h *DeviceHandler) Deregister(c *fiber.Ctx) error {
	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}
	userID := middleware.CurrentUserID(c)
	if err := h.svc.Deregister(c.Context(), deviceID, userID); err != nil {
		return response.InternalError(c, "failed to deregister device")
	}
	return response.NoContent(c)
}

// UpdatePushToken godoc
// @Summary     Update device push token
// @Description Updates the push token (and optionally app version) for a device
// @Tags        devices
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path     string                    true "Device UUID"
// @Param       body body     dto.UpdatePushTokenRequest true "Push token payload"
// @Success     200  {object} domain.Device
// @Failure     400  {object} map[string]interface{}
// @Failure     500  {object} map[string]interface{}
// @Router      /devices/{id} [patch]
func (h *DeviceHandler) UpdatePushToken(c *fiber.Ctx) error {
	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	var req dto.UpdatePushTokenRequest
	if err := validator.ParseAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.CurrentUserID(c)
	device, err := h.svc.UpdatePushToken(c.Context(), deviceID, userID, req.PushToken, req.AppVersion)
	if err != nil {
		return response.InternalError(c, "failed to update push token")
	}
	return response.OK(c, device)
}
