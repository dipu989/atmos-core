// Package service handles outbound push notifications triggered by domain events.
package service

import (
	"context"

	devdomain "github.com/dipu/atmos-core/internal/device/domain"
	devrepo "github.com/dipu/atmos-core/internal/device/repository"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/push"
	"go.uber.org/zap"
)

// NotificationService subscribes to domain events and delivers push notifications
// to the user's registered devices.
type NotificationService struct {
	deviceRepo *devrepo.DeviceRepository
	fcm        push.Sender // nil when FCM is not configured
}

func NewNotificationService(deviceRepo *devrepo.DeviceRepository, fcm push.Sender) *NotificationService {
	return &NotificationService{deviceRepo: deviceRepo, fcm: fcm}
}

// HandleInsightCreated is subscribed to EventInsightCreated.
// It fans out a push notification to every active FCM device the user has registered.
func (s *NotificationService) HandleInsightCreated(ctx context.Context, event eventbus.Event) {
	if s.fcm == nil {
		return // push not configured — skip silently
	}

	payload, ok := event.Payload.(insightdomain.InsightCreatedPayload)
	if !ok || payload.Insight == nil {
		return
	}

	insight := payload.Insight
	log := logger.L().With(
		zap.String("user_id", insight.UserID.String()),
		zap.String("insight_id", insight.ID.String()),
	)

	devices, err := s.deviceRepo.ListActiveByUser(ctx, insight.UserID)
	if err != nil {
		log.Warn("notification: list devices failed", zap.Error(err))
		return
	}

	for _, device := range devices {
		if !s.shouldPush(device) {
			continue
		}

		msg := push.Message{
			Token: *device.PushToken,
			Title: insight.Title,
			Body:  insight.Body,
			Data: map[string]string{
				"insight_id": insight.ID.String(),
				"type":       string(insight.InsightType),
			},
		}

		if err := s.fcm.Send(ctx, msg); err != nil {
			log.Warn("notification: FCM send failed",
				zap.String("device_id", device.ID.String()),
				zap.Error(err),
			)
		} else {
			log.Info("notification: push sent",
				zap.String("device_id", device.ID.String()),
			)
		}
	}
}

// shouldPush returns true when the device is configured for FCM and has a push token.
func (s *NotificationService) shouldPush(d devdomain.Device) bool {
	return d.IsActive &&
		d.PushProvider == devdomain.PushProviderFCM &&
		d.PushToken != nil &&
		*d.PushToken != ""
}
