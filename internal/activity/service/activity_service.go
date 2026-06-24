package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/dipu/atmos-core/internal/activity/repository"
	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	"github.com/dipu/atmos-core/internal/shortaddress"
	"github.com/dipu/atmos-core/internal/tripmatcher"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ActivityService struct {
	repo      *repository.ActivityRepository
	bus       eventbus.Bus
	shortAddr shortaddress.Resolver
}

func NewActivityService(repo *repository.ActivityRepository, bus eventbus.Bus, shortAddr shortaddress.Resolver) *ActivityService {
	return &ActivityService{repo: repo, bus: bus, shortAddr: shortAddr}
}

type IngestInput struct {
	UserID          uuid.UUID
	DeviceID        *uuid.UUID
	ActivityType    actdomain.ActivityType
	TransportMode   *actdomain.TransportMode
	DistanceKM      *float64
	EnergyKWH       *float64
	FuelType        *string
	DurationMinutes *int
	Source          actdomain.ActivitySource
	Provider        *string
	RawMetadata     actdomain.RawMetadata
	StartedAt       time.Time
	EndedAt         *time.Time
	UserTimezone    string
	IdempotencyKey  string
	// Dedup fields
	Origin             *string
	Destination        *string
	DisplayOrigin      *string
	DisplayDestination *string
	OriginLat          *float64
	OriginLng          *float64
	DestLat            *float64
	DestLng            *float64
	ReceiptID          *string
	FareAmount         *float64
	FareCurrency       *string
}

func (s *ActivityService) Ingest(ctx context.Context, input IngestInput) (*actdomain.Activity, error) {
	// Idempotency: if no key provided, derive one from stable fields.
	key := input.IdempotencyKey
	if key == "" {
		key = deriveIdempotencyKey(input.UserID, input.Source, input.Provider, input.StartedAt, input.TransportMode)
	}
	exists, err := s.repo.ExistsByIdempotencyKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrDuplicate
	}

	// GPS dedup (Task 5): before creating a new GPS row, check whether a
	// receipt activity already covers this trip and merge into it instead.
	if input.Source == actdomain.SourceGPS {
		enriched, matchConf, err := s.tryMergeGPSWithReceipt(ctx, input)
		if err != nil {
			return nil, err
		}
		if enriched != nil {
			return enriched, nil
		}
		if matchConf != nil {
			// Review-range match: create the GPS activity and notify the user.
			act, err := s.createActivity(ctx, input, key, matchConf)
			if err != nil {
				return nil, err
			}
			s.bus.Publish(ctx, eventbus.Event{
				Type: actdomain.EventActivityPossibleDuplicate,
				Payload: actdomain.ActivityPossibleDuplicatePayload{
					ActivityID:      act.ID,
					UserID:          act.UserID,
					MatchConfidence: *matchConf,
					StartedAt:       act.StartedAt,
					UserTimezone:    input.UserTimezone,
				},
			})
			return act, nil
		}
	}

	return s.createActivity(ctx, input, key, nil)
}

func (s *ActivityService) GetActivity(ctx context.Context, id, userID uuid.UUID) (*actdomain.Activity, error) {
	return s.repo.FindByID(ctx, id, userID)
}

func (s *ActivityService) BackfillRouteLabels(ctx context.Context, id uuid.UUID, origin, destination string) error {
	return s.repo.BackfillRouteLabels(ctx, id, origin, destination)
}

// HasRouteLabels reports whether the activity already has origin or destination
// populated. Used to skip expensive Gmail API fetches when backfill is not needed.
func (s *ActivityService) HasRouteLabels(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.repo.HasRouteLabels(ctx, id)
}

// dedupCandidateWindow is how far on each side of a receipt's time window we
// search for existing GPS activities. Accounts for GPS waking up late and
// minor clock drift between providers.
const dedupCandidateWindow = 15 // minutes

// IngestWithDedup ingests a receipt-sourced activity with trip deduplication.
// Before creating a new row it queries existing GPS activities in the time
// window and runs TripMatcher against each:
//
//   - score ≥ ThresholdAutoMerge (0.65)      → enrich the GPS activity; return it
//   - score ≥ ThresholdAutoMergeNoCoord (0.75 when no coords)
//   - ThresholdReview ≤ score < auto-merge   → create receipt activity with match_confidence set
//   - score < ThresholdReview (0.45)         → create receipt activity normally
//
// Returns (activity, enriched, error). enriched=true means a GPS activity
// was updated in place rather than a new row being created.
func (s *ActivityService) IngestWithDedup(ctx context.Context, input IngestInput) (*actdomain.Activity, bool, error) {
	// Layer 1: receipt_id idempotency (faster check before the window query).
	if input.ReceiptID != nil {
		existing, err := s.repo.FindByReceiptID(ctx, *input.ReceiptID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		if existing != nil {
			// Backfill origin/destination when the existing row is missing them
			// (e.g. re-sync after the snippet-enrichment fix was deployed).
			if existing.Origin == nil && existing.Destination == nil {
				origin, destination := "", ""
				if input.Origin != nil {
					origin = *input.Origin
				}
				if input.Destination != nil {
					destination = *input.Destination
				}
				_ = s.repo.BackfillRouteLabels(ctx, existing.ID, origin, destination)
			}
			return nil, false, ErrDuplicate
		}
	}

	// Layer 2: idempotency_key backstop.
	key := input.IdempotencyKey
	if key == "" {
		key = deriveIdempotencyKey(input.UserID, input.Source, input.Provider, input.StartedAt, input.TransportMode)
	}
	if exists, err := s.repo.ExistsByIdempotencyKey(ctx, key); err != nil {
		return nil, false, err
	} else if exists {
		return nil, false, ErrDuplicate
	}

	// Compute effective end time for the matcher.
	endedAt := input.EndedAt
	if endedAt == nil && input.DurationMinutes != nil {
		end := input.StartedAt.Add(time.Duration(*input.DurationMinutes) * time.Minute)
		endedAt = &end
	}

	// Find GPS activities in the time window as dedup candidates.
	windowEnd := input.StartedAt
	if endedAt != nil {
		windowEnd = *endedAt
	}
	candidates, err := s.repo.FindCandidatesInWindow(ctx, input.UserID, input.StartedAt, windowEnd, dedupCandidateWindow)
	if err != nil {
		return nil, false, err
	}

	// Build a TripCandidate from the incoming receipt input.
	receiptCandidate := tripCandidateFromInput(input, endedAt)

	// Score all candidates and track the best.
	type scoredCandidate struct {
		activity actdomain.Activity
		result   tripmatcher.MatchResult
	}
	var best *scoredCandidate

	for _, c := range candidates {
		// Only score against GPS-sourced activities — don't merge receipt with receipt.
		if c.Source != actdomain.SourceGPS {
			continue
		}
		r := tripmatcher.Score(receiptCandidate, tripCandidateFromActivity(c))
		if best == nil || r.Confidence > best.result.Confidence {
			cp := c
			best = &scoredCandidate{activity: cp, result: r}
		}
	}

	// Stricter threshold when destination coords or end time are unavailable.
	autoMergeThreshold := tripmatcher.ThresholdAutoMerge
	if best != nil && (!best.result.HasCoords || !best.result.HasEndTime) {
		autoMergeThreshold = tripmatcher.ThresholdAutoMergeNoCoord
	}

	// Auto-merge: enrich the GPS activity with receipt data.
	if best != nil && best.result.Confidence >= autoMergeThreshold {
		enrichInput := buildEnrichInput(input, best.activity, best.result.Confidence)
		if err := s.repo.EnrichFromReceipt(ctx, best.activity.ID, enrichInput); err != nil {
			return nil, false, fmt.Errorf("enrich activity: %w", err)
		}
		// Re-fetch to return the updated state.
		enriched, err := s.repo.FindByID(ctx, best.activity.ID, input.UserID)
		if err != nil {
			return nil, false, err
		}
		// Re-trigger emission recalculation so CO₂ reflects any distance update.
		s.bus.Publish(ctx, eventbus.Event{
			Type: actdomain.EventActivityIngested,
			Payload: actdomain.ActivityIngestedPayload{
				ActivityID:    enriched.ID,
				UserID:        enriched.UserID,
				ActivityType:  enriched.ActivityType,
				TransportMode: enriched.TransportMode,
				DistanceKM:    enriched.DistanceKM,
				EnergyKWH:     enriched.EnergyKWH,
				FuelType:      input.FuelType,
				StartedAt:     enriched.StartedAt,
				DateLocal:     enriched.DateLocal,
				RawMetadata:   enriched.RawMetadata,
			},
		})
		return enriched, true, nil
	}

	// Below auto-merge: create a receipt activity.
	var matchConf *float64
	if best != nil && best.result.Confidence >= tripmatcher.ThresholdReview {
		c := best.result.Confidence
		matchConf = &c
	}

	activity, err := s.createActivity(ctx, input, key, matchConf)
	if err != nil {
		return nil, false, err
	}
	if matchConf != nil {
		s.bus.Publish(ctx, eventbus.Event{
			Type: actdomain.EventActivityPossibleDuplicate,
			Payload: actdomain.ActivityPossibleDuplicatePayload{
				ActivityID:      activity.ID,
				UserID:          activity.UserID,
				MatchConfidence: *matchConf,
				StartedAt:       activity.StartedAt,
				UserTimezone:    input.UserTimezone,
			},
		})
	}
	return activity, false, nil
}

// createActivity is the shared path for creating a new activity row.
func (s *ActivityService) createActivity(ctx context.Context, input IngestInput, key string, matchConfidence *float64) (*actdomain.Activity, error) {
	dateLocal := localDate(input.StartedAt, input.UserTimezone)
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	metadata := input.RawMetadata
	if metadata == nil {
		metadata = actdomain.RawMetadata{}
	}
	activity := &actdomain.Activity{
		ID:                 id,
		UserID:             input.UserID,
		DeviceID:           input.DeviceID,
		ActivityType:       input.ActivityType,
		TransportMode:      input.TransportMode,
		DistanceKM:         input.DistanceKM,
		EnergyKWH:          input.EnergyKWH,
		DurationMinutes:    input.DurationMinutes,
		Source:             input.Source,
		Provider:           input.Provider,
		RawMetadata:        metadata,
		StartedAt:          input.StartedAt,
		EndedAt:            input.EndedAt,
		DateLocal:          dateLocal,
		IdempotencyKey:     key,
		Status:             actdomain.StatusPending,
		Origin:             input.Origin,
		Destination:        input.Destination,
		DisplayOrigin:      input.DisplayOrigin,
		DisplayDestination: input.DisplayDestination,
		OriginLat:          input.OriginLat,
		OriginLng:          input.OriginLng,
		DestLat:            input.DestLat,
		DestLng:            input.DestLng,
		ReceiptID:          input.ReceiptID,
		FareAmount:         input.FareAmount,
		FareCurrency:       input.FareCurrency,
		MatchConfidence:    matchConfidence,
	}
	if err := s.repo.Create(ctx, activity); err != nil {
		return nil, err
	}
	s.bus.Publish(ctx, eventbus.Event{
		Type: actdomain.EventActivityIngested,
		Payload: actdomain.ActivityIngestedPayload{
			ActivityID:    activity.ID,
			UserID:        activity.UserID,
			ActivityType:  activity.ActivityType,
			TransportMode: activity.TransportMode,
			DistanceKM:    activity.DistanceKM,
			EnergyKWH:     activity.EnergyKWH,
			FuelType:      input.FuelType,
			StartedAt:     activity.StartedAt,
			DateLocal:     activity.DateLocal,
			RawMetadata:   activity.RawMetadata,
		},
	})
	return activity, nil
}

// tripCandidateFromInput projects an IngestInput into a TripCandidate for scoring.
func tripCandidateFromInput(input IngestInput, endedAt *time.Time) tripmatcher.TripCandidate {
	mode := ""
	if input.TransportMode != nil {
		mode = string(*input.TransportMode)
	}
	return tripmatcher.TripCandidate{
		StartedAt:       input.StartedAt,
		EndedAt:         endedAt,
		OriginLat:       input.OriginLat,
		OriginLng:       input.OriginLng,
		DestLat:         input.DestLat,
		DestLng:         input.DestLng,
		TransportMode:   mode,
		DurationMinutes: input.DurationMinutes,
	}
}

// tripCandidateFromActivity projects a stored Activity into a TripCandidate.
func tripCandidateFromActivity(a actdomain.Activity) tripmatcher.TripCandidate {
	mode := ""
	if a.TransportMode != nil {
		mode = string(*a.TransportMode)
	}
	return tripmatcher.TripCandidate{
		StartedAt:       a.StartedAt,
		EndedAt:         a.EndedAt,
		OriginLat:       a.OriginLat,
		OriginLng:       a.OriginLng,
		DestLat:         a.DestLat,
		DestLng:         a.DestLng,
		TransportMode:   mode,
		DurationMinutes: a.DurationMinutes,
	}
}

// buildEnrichInput decides which fields from the receipt input to apply to the
// existing GPS activity, honoring the merge priority rules:
//   - Receipt wins: fare, distance, duration, provider, receipt_id
//   - GPS wins: coords (only filled when GPS activity had nil)
func buildEnrichInput(input IngestInput, gps actdomain.Activity, confidence float64) repository.EnrichReceiptInput {
	e := repository.EnrichReceiptInput{
		MatchConfidence: confidence,
		FareAmount:      input.FareAmount,
		FareCurrency:    input.FareCurrency,
		DistanceKM:      input.DistanceKM,
		DurationMinutes: input.DurationMinutes,
	}
	if input.ReceiptID != nil {
		e.ReceiptID = *input.ReceiptID
	}
	if input.Provider != nil {
		e.Provider = *input.Provider
	}
	// Receipt wins for human-readable addresses — GPS has none.
	if input.Origin != nil {
		e.Origin = *input.Origin
	}
	if input.Destination != nil {
		e.Destination = *input.Destination
	}
	e.DisplayOrigin = input.DisplayOrigin
	e.DisplayDestination = input.DisplayDestination
	// Only copy receipt coords into GPS activity when the GPS row has no value for that column.
	// Each column is guarded independently — a partial GPS fix (Lat set, Lng nil) must not
	// block the receipt from filling in the missing Lng.
	if gps.OriginLat == nil && input.OriginLat != nil {
		e.OriginLat = input.OriginLat
	}
	if gps.OriginLng == nil && input.OriginLng != nil {
		e.OriginLng = input.OriginLng
	}
	if gps.DestLat == nil && input.DestLat != nil {
		e.DestLat = input.DestLat
	}
	if gps.DestLng == nil && input.DestLng != nil {
		e.DestLng = input.DestLng
	}
	return e
}

// tryMergeGPSWithReceipt searches for a receipt-sourced activity that matches
// the incoming GPS trip and merges GPS coordinates into it if the confidence
// is high enough. It mirrors IngestWithDedup but in the GPS→receipt direction.
//
// Returns:
//   - (enriched, nil, nil)   — auto-merged; caller should return enriched
//   - (nil, &conf, nil)      — review-range match; caller creates GPS with this confidence
//   - (nil, nil, nil)        — no match; caller creates GPS normally
func (s *ActivityService) tryMergeGPSWithReceipt(ctx context.Context, input IngestInput) (*actdomain.Activity, *float64, error) {
	endedAt := input.EndedAt
	if endedAt == nil && input.DurationMinutes != nil {
		end := input.StartedAt.Add(time.Duration(*input.DurationMinutes) * time.Minute)
		endedAt = &end
	}
	windowEnd := input.StartedAt
	if endedAt != nil {
		windowEnd = *endedAt
	}

	candidates, err := s.repo.FindCandidatesInWindow(ctx, input.UserID, input.StartedAt, windowEnd, dedupCandidateWindow)
	if err != nil {
		return nil, nil, err
	}

	gpsCandidate := tripCandidateFromInput(input, endedAt)

	type scored struct {
		activity actdomain.Activity
		result   tripmatcher.MatchResult
	}
	var best *scored

	for _, c := range candidates {
		if !isReceiptSource(c.Source) {
			continue
		}
		r := tripmatcher.Score(gpsCandidate, tripCandidateFromActivity(c))
		if best == nil || r.Confidence > best.result.Confidence {
			cp := c
			best = &scored{activity: cp, result: r}
		}
	}

	if best == nil {
		return nil, nil, nil
	}

	// Stricter threshold when destination coords or end time are unavailable.
	autoMergeThreshold := tripmatcher.ThresholdAutoMerge
	if !best.result.HasCoords || !best.result.HasEndTime {
		autoMergeThreshold = tripmatcher.ThresholdAutoMergeNoCoord
	}

	if best.result.Confidence >= autoMergeThreshold {
		enrichInput := buildGPSEnrichInput(input, best.result.Confidence)
		if err := s.repo.EnrichFromReceipt(ctx, best.activity.ID, enrichInput); err != nil {
			return nil, nil, fmt.Errorf("enrich receipt with GPS: %w", err)
		}
		enriched, err := s.repo.FindByID(ctx, best.activity.ID, input.UserID)
		if err != nil {
			return nil, nil, err
		}
		s.bus.Publish(ctx, eventbus.Event{
			Type: actdomain.EventActivityIngested,
			Payload: actdomain.ActivityIngestedPayload{
				ActivityID:    enriched.ID,
				UserID:        enriched.UserID,
				ActivityType:  enriched.ActivityType,
				TransportMode: enriched.TransportMode,
				DistanceKM:    enriched.DistanceKM,
				EnergyKWH:     enriched.EnergyKWH,
				FuelType:      input.FuelType,
				StartedAt:     enriched.StartedAt,
				DateLocal:     enriched.DateLocal,
				RawMetadata:   enriched.RawMetadata,
			},
		})
		return enriched, nil, nil
	}

	if best.result.Confidence >= tripmatcher.ThresholdReview {
		c := best.result.Confidence
		return nil, &c, nil
	}

	return nil, nil, nil
}

// isReceiptSource reports whether an activity came from a receipt-based ingestion
// path. These are the candidates that an incoming GPS trip can be merged into.
// SourceGPSReceipt is included so that an already-merged row remains visible on
// retry (GPS idempotency key not stored) and on GPS session split (app restart
// mid-trip) — in both cases the second GPS event re-merges into the same row
// rather than creating a duplicate.
func isReceiptSource(src actdomain.ActivitySource) bool {
	switch src {
	case actdomain.SourceGmail, actdomain.SourceUber, actdomain.SourceOla,
		actdomain.SourceRapido, actdomain.SourceNammaYatri, actdomain.SourceGPSReceipt:
		return true
	}
	return false
}

// buildGPSEnrichInput constructs the update payload for a GPS→receipt merge.
// GPS wins for coordinates (ground-truth tracking); receipt keeps its fare,
// distance, duration, and provider.
func buildGPSEnrichInput(input IngestInput, confidence float64) repository.EnrichReceiptInput {
	e := repository.EnrichReceiptInput{
		MatchConfidence: confidence,
	}
	if input.OriginLat != nil {
		e.OriginLat = input.OriginLat
		e.OriginLng = input.OriginLng
	}
	if input.DestLat != nil {
		e.DestLat = input.DestLat
		e.DestLng = input.DestLng
	}
	return e
}

// UpdateInput carries the fields that can be changed on an existing activity.
// Nil fields are left unchanged.
type UpdateInput struct {
	TransportMode   *actdomain.TransportMode
	DistanceKM      *float64
	EnergyKWH       *float64
	FuelType        *string
	DurationMinutes *int
	StartedAt       *time.Time
	UserTimezone    string
}

// UpdateActivity applies a partial update to an existing activity and triggers
// emission recalculation for any affected dates.
// Returns ErrNotFound when the activity does not exist or belongs to another user.
// Returns the unchanged activity when no fields are provided.
func (s *ActivityService) UpdateActivity(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*actdomain.Activity, error) {
	// Fast path: nothing to do.
	if input.TransportMode == nil && input.DistanceKM == nil &&
		input.EnergyKWH == nil && input.DurationMinutes == nil && input.StartedAt == nil {
		activity, err := s.repo.FindByID(ctx, id, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return activity, nil
	}

	activity, err := s.repo.FindByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Remember old date so we can recompute it if started_at changes.
	oldDateLocal := activity.DateLocal

	if input.TransportMode != nil {
		activity.TransportMode = input.TransportMode
		// Keep activity_type in sync.
		if *input.TransportMode == actdomain.ModeFlight {
			activity.ActivityType = actdomain.ActivityFlight
		} else {
			activity.ActivityType = actdomain.ActivityTransport
		}
	}
	if input.DistanceKM != nil {
		activity.DistanceKM = input.DistanceKM
	}
	if input.EnergyKWH != nil {
		activity.EnergyKWH = input.EnergyKWH
	}
	if input.DurationMinutes != nil {
		activity.DurationMinutes = input.DurationMinutes
	}
	if input.StartedAt != nil {
		activity.StartedAt = *input.StartedAt
		tz := input.UserTimezone
		if tz == "" {
			tz = "UTC"
		}
		activity.DateLocal = localDate(*input.StartedAt, tz)
	}

	if err := s.repo.Update(ctx, activity); err != nil {
		return nil, err
	}

	// Re-fire ingestion event so the emission is recalculated and the new
	// date's timeline summary is rebuilt via the event chain.
	s.bus.Publish(ctx, eventbus.Event{
		Type: actdomain.EventActivityIngested,
		Payload: actdomain.ActivityIngestedPayload{
			ActivityID:    activity.ID,
			UserID:        activity.UserID,
			ActivityType:  activity.ActivityType,
			TransportMode: activity.TransportMode,
			DistanceKM:    activity.DistanceKM,
			EnergyKWH:     activity.EnergyKWH,
			FuelType:      input.FuelType,
			StartedAt:     activity.StartedAt,
			DateLocal:     activity.DateLocal,
			RawMetadata:   activity.RawMetadata,
		},
	})

	// If the date changed, also trigger a recompute of the old date so that
	// day's summary no longer includes this activity's old contribution.
	if !oldDateLocal.Equal(activity.DateLocal) {
		s.bus.Publish(ctx, eventbus.Event{
			Type: emidomain.EventEmissionCalculated,
			Payload: emidomain.EmissionCalculatedPayload{
				UserID:    userID,
				DateLocal: oldDateLocal,
			},
		})
	}

	return activity, nil
}

// DeleteActivity removes an activity and triggers timeline recomputation
// for the affected date.
func (s *ActivityService) DeleteActivity(ctx context.Context, id, userID uuid.UUID) error {
	activity, err := s.repo.FindByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	dateLocal := activity.DateLocal

	deleted, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}

	// Trigger timeline recompute for the affected date (the activity and its
	// cascaded emission are now gone from the DB, so RecomputeDay will produce
	// the correct reduced totals).
	s.bus.Publish(ctx, eventbus.Event{
		Type: emidomain.EventEmissionCalculated,
		Payload: emidomain.EmissionCalculatedPayload{
			UserID:    userID,
			DateLocal: dateLocal,
		},
	})

	return nil
}

func (s *ActivityService) ListActivities(ctx context.Context, userID uuid.UUID, from, to *time.Time, limit, offset int) ([]actdomain.Activity, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	total, err := s.repo.CountByUser(ctx, userID, from, to)
	if err != nil {
		return nil, 0, err
	}
	activities, err := s.repo.ListByUser(ctx, userID, from, to, limit, offset)
	return activities, total, err
}

// BackfillDisplayAddressSweep resolves and persists display addresses for up
// to limit historical activities (system-wide, oldest first) that have
// coordinates but no display address yet. Intended to run on a worker cron,
// not on a request path — this keeps ListActivities/GetActivity pure, fast
// reads with no third-party dependency, and converges deterministically
// regardless of which rows users happen to read. Returns the number of rows
// that had at least one field successfully backfilled.
func (s *ActivityService) BackfillDisplayAddressSweep(ctx context.Context, limit int) (int, error) {
	activities, err := s.repo.FindNeedingDisplayBackfill(ctx, limit)
	if err != nil {
		return 0, err
	}

	resolved := 0
	for _, a := range activities {
		var displayOrigin, displayDestination *string

		if a.DisplayOrigin == nil && a.OriginLat != nil && a.OriginLng != nil {
			if addr, err := s.shortAddr.Resolve(ctx, *a.OriginLat, *a.OriginLng); err == nil {
				displayOrigin = &addr
			}
		}
		if a.DisplayDestination == nil && a.DestLat != nil && a.DestLng != nil {
			if addr, err := s.shortAddr.Resolve(ctx, *a.DestLat, *a.DestLng); err == nil {
				displayDestination = &addr
			}
		}
		if displayOrigin == nil && displayDestination == nil {
			continue
		}
		if err := s.repo.BackfillDisplayAddress(ctx, a.ID, displayOrigin, displayDestination); err != nil {
			logger.L().Warn("backfill display address failed", zap.String("activity_id", a.ID.String()), zap.Error(err))
			continue
		}
		resolved++
	}
	return resolved, nil
}

// ExportRowCap is the maximum number of rows returned by ExportActivities.
const ExportRowCap = 5000

// ExportActivities returns up to ExportRowCap activities for CSV export.
func (s *ActivityService) ExportActivities(ctx context.Context, userID uuid.UUID, from, to *time.Time) ([]actdomain.Activity, error) {
	return s.repo.ListAllByUser(ctx, userID, from, to, ExportRowCap)
}

func localDate(t time.Time, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func deriveIdempotencyKey(userID uuid.UUID, source actdomain.ActivitySource, provider *string, startedAt time.Time, mode *actdomain.TransportMode) string {
	p := ""
	if provider != nil {
		p = *provider
	}
	m := ""
	if mode != nil {
		m = string(*mode)
	}
	raw := fmt.Sprintf("%s:%s:%s:%s:%s", userID, source, p, startedAt.UTC().Format(time.RFC3339), m)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
