// Package service handles outbound push notifications triggered by domain events.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	devdomain "github.com/dipu/atmos-core/internal/device/domain"
	devrepo "github.com/dipu/atmos-core/internal/device/repository"
	idrepo "github.com/dipu/atmos-core/internal/identity/repository"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/push"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NotificationService subscribes to domain events and delivers push notifications
// to the user's registered devices.
type NotificationService struct {
	deviceRepo *devrepo.DeviceRepository
	userRepo   *idrepo.UserRepository
	fcm        push.Sender // nil when FCM is not configured
}

func NewNotificationService(deviceRepo *devrepo.DeviceRepository, userRepo *idrepo.UserRepository, fcm push.Sender) *NotificationService {
	return &NotificationService{deviceRepo: deviceRepo, userRepo: userRepo, fcm: fcm}
}

// pushAllowed reports whether a push should be sent for userID, logging the outcome
// with the caller's log context. No preferences row yet (new user) fails open to
// match the column's default of true. Any other lookup error also fails open —
// consistent with this codebase's existing fail-open convention for preference
// reads (see the regionFn closures in cmd/api/main.go and cmd/worker/main.go) —
// but is logged, unlike a missing row, since it signals a real infra problem.
func (s *NotificationService) pushAllowed(ctx context.Context, userID uuid.UUID, log *zap.Logger) bool {
	prefs, err := s.userRepo.GetPreferences(ctx, userID)
	switch {
	case err == nil:
		// fall through to the preference check below
	case errors.Is(err, gorm.ErrRecordNotFound):
		return true
	default:
		log.Warn("notification: preferences lookup failed, sending anyway", zap.Error(err))
		return true
	}

	if !prefs.PushNotificationsEnabled {
		log.Info("notification: push notifications disabled for user, skipping")
		return false
	}
	return true
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

	if !s.pushAllowed(ctx, insight.UserID, log) {
		return
	}

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

// HandleActivityPossibleDuplicate is subscribed to EventActivityPossibleDuplicate.
// It notifies the user when a new activity scores in the review range against an
// existing one, prompting them to check whether it is a duplicate.
func (s *NotificationService) HandleActivityPossibleDuplicate(ctx context.Context, event eventbus.Event) {
	if s.fcm == nil {
		return
	}

	payload, ok := event.Payload.(actdomain.ActivityPossibleDuplicatePayload)
	if !ok {
		return
	}

	log := logger.L().With(
		zap.String("user_id", payload.UserID.String()),
		zap.String("activity_id", payload.ActivityID.String()),
	)

	if !s.pushAllowed(ctx, payload.UserID, log) {
		return
	}

	devices, err := s.deviceRepo.ListActiveByUser(ctx, payload.UserID)
	if err != nil {
		log.Warn("notification: list devices failed", zap.Error(err))
		return
	}

	for _, device := range devices {
		if !s.shouldPush(device) {
			continue
		}

		loc := time.UTC
		if payload.UserTimezone != "" {
			if l, err := time.LoadLocation(payload.UserTimezone); err == nil {
				loc = l
			}
		}
		msg := push.Message{
			Token: *device.PushToken,
			Title: "Possible duplicate trip",
			Body:  fmt.Sprintf("A trip logged at %s may already exist. Tap to review.", payload.StartedAt.In(loc).Format("3:04 PM")),
			Data: map[string]string{
				"type":        "possible_duplicate",
				"activity_id": payload.ActivityID.String(),
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
