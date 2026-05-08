package service

import (
	"context"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	actrepo "github.com/dipu/atmos-core/internal/activity/repository"
	"github.com/dipu/atmos-core/internal/emission/calculator"
	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	emirepo "github.com/dipu/atmos-core/internal/emission/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EmissionService struct {
	emissionRepo *emirepo.EmissionRepository
	activityRepo *actrepo.ActivityRepository
	bus          eventbus.Bus
}

func NewEmissionService(
	emissionRepo *emirepo.EmissionRepository,
	activityRepo *actrepo.ActivityRepository,
	bus eventbus.Bus,
) *EmissionService {
	return &EmissionService{
		emissionRepo: emissionRepo,
		activityRepo: activityRepo,
		bus:          bus,
	}
}

// HandleActivityIngested is subscribed to EventActivityIngested.
// It resolves the factor, calculates kg_co2e, persists the emission, marks
// the activity processed, and emits EventEmissionCalculated.
func (s *EmissionService) HandleActivityIngested(ctx context.Context, event eventbus.Event) {
	payload, ok := event.Payload.(actdomain.ActivityIngestedPayload)
	if !ok {
		return
	}

	log := logger.L().With(zap.String("activity_id", payload.ActivityID.String()))

	var modeStr *string
	if payload.TransportMode != nil {
		m := string(*payload.TransportMode)
		modeStr = &m
	}

	factor, err := s.emissionRepo.ResolveFactor(ctx, string(payload.ActivityType), modeStr, "IN", payload.StartedAt)
	if err != nil {
		log.Warn("no emission factor found, skipping", zap.Error(err))
		reason := "no matching emission factor"
		_ = s.activityRepo.UpdateStatus(ctx, payload.ActivityID, actdomain.StatusSkipped, &reason)
		return
	}

	kgCO2e, err := calculator.Calculate(factor, string(payload.ActivityType), payload.DistanceKM, nil)
	if err != nil {
		log.Error("emission calculation failed", zap.Error(err))
		reason := err.Error()
		_ = s.activityRepo.UpdateStatus(ctx, payload.ActivityID, actdomain.StatusFailed, &reason)
		return
	}

	id, _ := uuid.NewV7()
	emission := &emidomain.Emission{
		ID:                 id,
		ActivityID:         payload.ActivityID,
		UserID:             payload.UserID,
		EmissionFactorID:   factor.ID,
		KgCO2e:             kgCO2e,
		CalculationVersion: 1,
		CalculatedAt:       time.Now(),
	}

	if err := s.emissionRepo.Upsert(ctx, emission); err != nil {
		log.Error("failed to persist emission", zap.Error(err))
		reason := "db error persisting emission"
		_ = s.activityRepo.UpdateStatus(ctx, payload.ActivityID, actdomain.StatusFailed, &reason)
		return
	}

	_ = s.activityRepo.UpdateStatus(ctx, payload.ActivityID, actdomain.StatusProcessed, nil)

	s.bus.Publish(ctx, eventbus.Event{
		Type: emidomain.EventEmissionCalculated,
		Payload: emidomain.EmissionCalculatedPayload{
			EmissionID: emission.ID,
			ActivityID: payload.ActivityID,
			UserID:     payload.UserID,
			KgCO2e:     kgCO2e,
			DateLocal:  payload.DateLocal,
		},
	})

	log.Info("emission calculated",
		zap.Float64("kg_co2e", kgCO2e),
		zap.String("factor_id", factor.ID.String()),
	)
}

func (s *EmissionService) GetByActivityID(ctx context.Context, activityID uuid.UUID) (*emidomain.Emission, error) {
	return s.emissionRepo.FindByActivityID(ctx, activityID)
}
