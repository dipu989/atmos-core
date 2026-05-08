package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Platform string
type PushProvider string
type APNsEnvironment string

const (
	PlatformIOS     Platform = "ios"
	PlatformIPadOS  Platform = "ipados"
	PlatformAndroid Platform = "android"
	PlatformWeb     Platform = "web"
	PlatformUnknown Platform = "unknown"

	PushProviderAPNs PushProvider = "apns"
	PushProviderFCM  PushProvider = "fcm"
	PushProviderNone PushProvider = "none"

	APNsEnvironmentSandbox    APNsEnvironment = "sandbox"
	APNsEnvironmentProduction APNsEnvironment = "production"
)

type Device struct {
	ID              uuid.UUID        `gorm:"type:uuid;primaryKey"  json:"id"`
	UserID          uuid.UUID        `gorm:"type:uuid;not null"    json:"user_id"`
	DeviceToken     string           `gorm:"uniqueIndex;not null"  json:"device_token"`
	Platform        Platform         `gorm:"not null"              json:"platform"`
	PushProvider    PushProvider     `gorm:"not null;default:'none'" json:"push_provider"`
	APNsEnvironment *APNsEnvironment `gorm:"column:apns_environment" json:"apns_environment,omitempty"`
	DeviceName      *string          `json:"device_name,omitempty"`
	OSVersion       *string          `json:"os_version,omitempty"`
	AppVersion      *string          `json:"app_version,omitempty"`
	PushToken       *string          `json:"-"`
	LastSeenAt      *time.Time       `json:"last_seen_at,omitempty"`
	IsActive        bool             `gorm:"not null;default:true" json:"is_active"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

func (Device) TableName() string { return "devices" }

func (d *Device) Validate() error {
	switch d.Platform {
	case PlatformIOS, PlatformIPadOS:
		if d.PushProvider != PushProviderAPNs {
			return errors.New("ios/ipados devices must use apns push provider")
		}
		if d.APNsEnvironment == nil {
			return errors.New("ios/ipados devices must specify apns_environment")
		}
	case PlatformAndroid:
		if d.PushProvider != PushProviderFCM && d.PushProvider != PushProviderNone {
			return errors.New("android devices must use fcm or none push provider")
		}
		if d.APNsEnvironment != nil {
			return errors.New("android devices must not set apns_environment")
		}
	case PlatformWeb, PlatformUnknown:
		if d.PushProvider != PushProviderNone {
			return errors.New("web/unknown devices must use none push provider")
		}
	}
	return nil
}
