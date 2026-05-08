package handler

import (
	"github.com/dipu/atmos-core/internal/device/domain"
	"github.com/dipu/atmos-core/internal/device/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type DeviceHandler struct {
	svc *service.DeviceService
}

func NewDeviceHandler(svc *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

func (h *DeviceHandler) Register(c *fiber.Ctx) error {
	var req struct {
		DeviceToken     string  `json:"device_token"`
		Platform        string  `json:"platform"`
		PushProvider    string  `json:"push_provider"`
		APNsEnvironment *string `json:"apns_environment"`
		DeviceName      *string `json:"device_name"`
		OSVersion       *string `json:"os_version"`
		AppVersion      *string `json:"app_version"`
		PushToken       *string `json:"push_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.DeviceToken == "" || req.Platform == "" {
		return response.BadRequest(c, "device_token and platform are required")
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

func (h *DeviceHandler) List(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	devices, err := h.svc.ListDevices(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "failed to list devices")
	}
	return response.OK(c, devices)
}

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

func (h *DeviceHandler) UpdatePushToken(c *fiber.Ctx) error {
	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}
	var req struct {
		PushToken  string  `json:"push_token"`
		AppVersion *string `json:"app_version"`
	}
	if err := c.BodyParser(&req); err != nil || req.PushToken == "" {
		return response.BadRequest(c, "push_token is required")
	}

	userID := middleware.CurrentUserID(c)
	device, err := h.svc.UpdatePushToken(c.Context(), deviceID, userID, req.PushToken, req.AppVersion)
	if err != nil {
		return response.InternalError(c, "failed to update push token")
	}
	return response.OK(c, device)
}
