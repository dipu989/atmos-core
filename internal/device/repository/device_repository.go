package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/device/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) Create(ctx context.Context, device *domain.Device) error {
	return r.db.WithContext(ctx).Create(device).Error
}

func (r *DeviceRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	var d domain.Device
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeviceRepository) FindByToken(ctx context.Context, deviceToken string) (*domain.Device, error) {
	var d domain.Device
	err := r.db.WithContext(ctx).Where("device_token = ?", deviceToken).First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeviceRepository) ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]domain.Device, error) {
	var devices []domain.Device
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_active = TRUE", userID).
		Order("created_at DESC").
		Find(&devices).Error
	return devices, err
}

func (r *DeviceRepository) Update(ctx context.Context, device *domain.Device) error {
	return r.db.WithContext(ctx).Save(device).Error
}

func (r *DeviceRepository) Deactivate(ctx context.Context, id, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&domain.Device{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_active", false).Error
}

func (r *DeviceRepository) TouchLastSeen(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.Device{}).
		Where("id = ?", id).
		Update("last_seen_at", now).Error
}
