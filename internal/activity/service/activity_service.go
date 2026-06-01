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
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ActivityService struct {
	repo *repository.ActivityRepository
	bus  eventbus.Bus
}

func NewActivityService(repo *repository.ActivityRepository, bus eventbus.Bus) *ActivityService {
	return &ActivityService{repo: repo, bus: bus}
}

type IngestInput struct {
	UserID          uuid.UUID
	DeviceID        *uuid.UUID
	ActivityType    actdomain.ActivityType
	TransportMode   *actdomain.TransportMode
	DistanceKM      *float64
	DurationMinutes *int
	Source          actdomain.ActivitySource
	Provider        *string
	RawMetadata     actdomain.RawMetadata
	StartedAt       time.Time
	EndedAt         *time.Time
	UserTimezone    string
	IdempotencyKey  string
}

func (s *ActivityService) Ingest(ctx context.Context, input IngestInput) (*actdomain.Activity, error) {
	// Idempotency: if no key provided, derive one from stable fields
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
		ID:              id,
		UserID:          input.UserID,
		DeviceID:        input.DeviceID,
		ActivityType:    input.ActivityType,
		TransportMode:   input.TransportMode,
		DistanceKM:      input.DistanceKM,
		DurationMinutes: input.DurationMinutes,
		Source:          input.Source,
		Provider:        input.Provider,
		RawMetadata:     metadata,
		StartedAt:       input.StartedAt,
		EndedAt:         input.EndedAt,
		DateLocal:       dateLocal,
		IdempotencyKey:  key,
		Status:          actdomain.StatusPending,
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
			StartedAt:     activity.StartedAt,
			DateLocal:     activity.DateLocal,
			RawMetadata:   activity.RawMetadata,
		},
	})

	return activity, nil
}

func (s *ActivityService) GetActivity(ctx context.Context, id, userID uuid.UUID) (*actdomain.Activity, error) {
	return s.repo.FindByID(ctx, id, userID)
}

// UpdateInput carries the fields that can be changed on an existing activity.
// Nil fields are left unchanged.
type UpdateInput struct {
	TransportMode   *actdomain.TransportMode
	DistanceKM      *float64
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
		input.DurationMinutes == nil && input.StartedAt == nil {
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

func (s *ActivityService) ListActivities(ctx context.Context, userID uuid.UUID, from, to time.Time, limit, offset int) ([]actdomain.Activity, int64, error) {
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
