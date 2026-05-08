package service

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/device/domain"
	"github.com/dipu/atmos-core/internal/device/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeviceService struct {
	repo *repository.DeviceRepository
}

func NewDeviceService(repo *repository.DeviceRepository) *DeviceService {
	return &DeviceService{repo: repo}
}

type RegisterInput struct {
	UserID          uuid.UUID
	DeviceToken     string
	Platform        domain.Platform
	PushProvider    domain.PushProvider
	APNsEnvironment *domain.APNsEnvironment
	DeviceName      *string
	OSVersion       *string
	AppVersion      *string
	PushToken       *string
}

func (s *DeviceService) Register(ctx context.Context, input RegisterInput) (*domain.Device, error) {
	// Upsert: if the device_token already exists for this user, update it
	existing, err := s.repo.FindByToken(ctx, input.DeviceToken)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	device := &domain.Device{
		UserID:          input.UserID,
		DeviceToken:     input.DeviceToken,
		Platform:        input.Platform,
		PushProvider:    input.PushProvider,
		APNsEnvironment: input.APNsEnvironment,
		DeviceName:      input.DeviceName,
		OSVersion:       input.OSVersion,
		AppVersion:      input.AppVersion,
		PushToken:       input.PushToken,
		IsActive:        true,
	}

	if err := device.Validate(); err != nil {
		return nil, err
	}

	if existing != nil {
		device.ID = existing.ID
		device.CreatedAt = existing.CreatedAt
		if err := s.repo.Update(ctx, device); err != nil {
			return nil, err
		}
		return device, nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	device.ID = id

	if err := s.repo.Create(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *DeviceService) ListDevices(ctx context.Context, userID uuid.UUID) ([]domain.Device, error) {
	return s.repo.ListActiveByUser(ctx, userID)
}

func (s *DeviceService) Deregister(ctx context.Context, deviceID, userID uuid.UUID) error {
	return s.repo.Deactivate(ctx, deviceID, userID)
}

func (s *DeviceService) UpdatePushToken(ctx context.Context, deviceID, userID uuid.UUID, pushToken string, appVersion *string) (*domain.Device, error) {
	device, err := s.repo.FindByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if device.UserID != userID {
		return nil, errors.New("device not found")
	}
	device.PushToken = &pushToken
	if appVersion != nil {
		device.AppVersion = appVersion
	}
	if err := s.repo.Update(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}
