package service

import (
	"context"
	"math"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/dipu/atmos-core/internal/activity/dto"
	actrepo "github.com/dipu/atmos-core/internal/activity/repository"
	"github.com/dipu/atmos-core/internal/emission/calculator"
	"github.com/dipu/atmos-core/internal/emission/constants"
	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	emirepo "github.com/dipu/atmos-core/internal/emission/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RegionFetcher returns the emission-factor region code for a user (e.g. "IN", "global").
// Returning "IN" as the fallback keeps existing behaviour for users without preferences.
type RegionFetcher func(ctx context.Context, userID uuid.UUID) string

type EmissionService struct {
	emissionRepo *emirepo.EmissionRepository
	activityRepo *actrepo.ActivityRepository
	bus          eventbus.Bus
	regionFn     RegionFetcher
}

func NewEmissionService(
	emissionRepo *emirepo.EmissionRepository,
	activityRepo *actrepo.ActivityRepository,
	bus eventbus.Bus,
	regionFn RegionFetcher,
) *EmissionService {
	return &EmissionService{
		emissionRepo: emissionRepo,
		activityRepo: activityRepo,
		bus:          bus,
		regionFn:     regionFn,
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

	region := s.regionFn(ctx, payload.UserID)
	factor, err := s.emissionRepo.ResolveFactor(ctx, string(payload.ActivityType), modeStr, payload.FuelType, region, payload.StartedAt)
	if err != nil {
		log.Warn("no emission factor found, skipping", zap.Error(err))
		reason := "no matching emission factor"
		_ = s.activityRepo.UpdateStatus(ctx, payload.ActivityID, actdomain.StatusSkipped, &reason)
		return
	}

	kgCO2e, err := calculator.Calculate(factor, string(payload.ActivityType), payload.DistanceKM, payload.EnergyKWH)
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

// ComputeImpactContext translates an activity's kg CO2e into relatable
// comparisons (trees, LED-hours, % of global daily average) and, where a
// greener alternative mode exists and is actually lower-emission for this
// trip's distance, a savings comparison. All factors are resolved through the
// same ResolveFactor path used for the activity's own emission — never a
// separate hardcoded table — so the comparison can't drift from the
// authoritative factor used to calculate the activity in the first place.
func (s *EmissionService) ComputeImpactContext(ctx context.Context, activity *actdomain.Activity) dto.ImpactContext {
	if activity.KgCO2e == nil {
		return dto.ImpactContext{} // not yet processed — nothing to compare yet
	}
	kgCO2e := *activity.KgCO2e

	impact := dto.ImpactContext{Approximate: true}
	if kgCO2e > 0 {
		impact.TreesNeededToOffset = max(1, int(math.Ceil(kgCO2e/constants.TreeKgCO2ePerDay)))
		impact.LedHoursEquivalent = max(1, int(math.Ceil(kgCO2e/constants.LedKgCO2ePerHour)))
		impact.GlobalAveragePct = max(1, int(math.Round(kgCO2e/constants.GlobalAvgKgCO2ePerDay*100)))
	}

	if activity.TransportMode == nil || activity.DistanceKM == nil || kgCO2e <= 0 {
		return impact
	}

	altMode := bestEcoAlternative(*activity.TransportMode)
	if altMode == nil {
		return impact
	}

	log := logger.L().With(zap.String("activity_id", activity.ID.String()))

	// Use the same region as the factor that actually priced this activity,
	// not a freshly-resolved one — a user's region preference can change
	// between ingestion and viewing, and re-deriving it here would silently
	// compare two different regions' factors in the savings figure below.
	region, err := s.emissionRepo.FindFactorRegionByActivityID(ctx, activity.ID)
	if err != nil {
		log.Debug("could not resolve original factor region, omitting alternative", zap.Error(err))
		return impact
	}

	altModeStr := string(*altMode)
	factor, err := s.emissionRepo.ResolveFactor(ctx, string(actdomain.ActivityTransport), &altModeStr, nil, region, activity.StartedAt)
	if err != nil {
		log.Debug("no emission factor found for alternative mode, omitting comparison",
			zap.String("alternative_mode", altModeStr), zap.Error(err))
		return impact // alternative factor not resolvable — omit rather than guess
	}

	altKgCO2e, err := calculator.Calculate(factor, string(actdomain.ActivityTransport), activity.DistanceKM, nil)
	if err != nil {
		log.Warn("failed to calculate alternative emission, omitting comparison",
			zap.String("alternative_mode", altModeStr), zap.Error(err))
		return impact
	}

	savings := kgCO2e - altKgCO2e
	if savings <= 0 {
		return impact // alternative isn't actually greener for this trip — don't show it
	}

	savingsPct := int(math.Round(savings / kgCO2e * 100))
	impact.AlternativeMode = altMode
	impact.AlternativeKgCO2e = &altKgCO2e
	impact.SavingsKgCO2e = &savings
	impact.SavingsPct = &savingsPct
	return impact
}
