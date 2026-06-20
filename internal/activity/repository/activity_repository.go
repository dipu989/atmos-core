package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/activity/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ActivityRepository struct {
	db *gorm.DB
}

func NewActivityRepository(db *gorm.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

func (r *ActivityRepository) Create(ctx context.Context, activity *domain.Activity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

func (r *ActivityRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.Activity, error) {
	var a domain.Activity
	err := r.db.WithContext(ctx).
		Joins("LEFT JOIN emissions e ON e.activity_id = activities.id").
		Select("activities.*, e.kg_co2e").
		Where("activities.id = ? AND activities.user_id = ?", id, userID).
		First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *ActivityRepository) ListByUser(ctx context.Context, userID uuid.UUID, from, to *time.Time, limit, offset int) ([]domain.Activity, error) {
	var activities []domain.Activity
	q := r.db.WithContext(ctx).
		Joins("LEFT JOIN emissions e ON e.activity_id = activities.id").
		Select("activities.*, e.kg_co2e").
		Where("activities.user_id = ?", userID)
	if from != nil {
		q = q.Where("activities.date_local >= ?", *from)
	}
	if to != nil {
		q = q.Where("activities.date_local <= ?", *to)
	}
	q = q.Order("activities.started_at DESC").Limit(limit).Offset(offset)
	return activities, q.Find(&activities).Error
}

// ListAllByUser fetches up to cap activities for a user, optionally filtered by date range.
// Used for CSV export; bypasses the pagination limit in ListByUser.
func (r *ActivityRepository) ListAllByUser(ctx context.Context, userID uuid.UUID, from, to *time.Time, cap int) ([]domain.Activity, error) {
	var activities []domain.Activity
	q := r.db.WithContext(ctx).
		Joins("LEFT JOIN emissions e ON e.activity_id = activities.id").
		Select("activities.*, e.kg_co2e").
		Where("activities.user_id = ?", userID)
	if from != nil {
		q = q.Where("activities.date_local >= ?", *from)
	}
	if to != nil {
		q = q.Where("activities.date_local <= ?", *to)
	}
	q = q.Order("activities.started_at DESC").Limit(cap)
	return activities, q.Find(&activities).Error
}

func (r *ActivityRepository) CountByUser(ctx context.Context, userID uuid.UUID, from, to *time.Time) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&domain.Activity{}).Where("user_id = ?", userID)
	if from != nil {
		q = q.Where("date_local >= ?", *from)
	}
	if to != nil {
		q = q.Where("date_local <= ?", *to)
	}
	return count, q.Count(&count).Error
}

func (r *ActivityRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ActivityStatus, reason *string) error {
	updates := map[string]any{"status": status}
	if reason != nil {
		updates["failure_reason"] = *reason
	}
	return r.db.WithContext(ctx).Model(&domain.Activity{}).Where("id = ?", id).Updates(updates).Error
}

// Update persists changes to an existing activity.
func (r *ActivityRepository) Update(ctx context.Context, activity *domain.Activity) error {
	return r.db.WithContext(ctx).Save(activity).Error
}

// Delete hard-deletes an activity owned by userID.
// Returns (true, nil) when found and deleted, (false, nil) when not found.
func (r *ActivityRepository) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&domain.Activity{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *ActivityRepository) ExistsByIdempotencyKey(ctx context.Context, key string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Activity{}).Where("idempotency_key = ?", key).Count(&count).Error
	return count > 0, err
}

func (r *ActivityRepository) ExistsByReceiptID(ctx context.Context, receiptID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Activity{}).Where("receipt_id = ?", receiptID).Count(&count).Error
	return count > 0, err
}

func (r *ActivityRepository) FindByReceiptID(ctx context.Context, receiptID string) (*domain.Activity, error) {
	var a domain.Activity
	err := r.db.WithContext(ctx).Where("receipt_id = ?", receiptID).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// HasRouteLabels returns true if the activity already has origin or destination
// populated. Used as a cheap pre-check before fetching emails for backfill.
func (r *ActivityRepository) HasRouteLabels(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Activity{}).
		Where("id = ? AND (origin IS NOT NULL OR destination IS NOT NULL)", id).
		Count(&count).Error
	return count > 0, err
}

// BackfillRouteLabels sets origin and destination on an activity only when they
// are currently NULL. Used to enrich existing rows on a re-sync without
// overwriting data that was already set.
func (r *ActivityRepository) BackfillRouteLabels(ctx context.Context, id uuid.UUID, origin, destination string) error {
	updates := map[string]any{}
	if origin != "" {
		updates["origin"] = origin
	}
	if destination != "" {
		updates["destination"] = destination
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&domain.Activity{}).
		Where("id = ? AND origin IS NULL AND destination IS NULL", id).
		Updates(updates).Error
}

// FindCandidatesInWindow returns activities for a user that overlap a given time window.
// Used by the TripMatcher to find dedup candidates. The window is expanded by bufferMinutes
// on each side to account for GPS start-time jitter.
func (r *ActivityRepository) FindCandidatesInWindow(ctx context.Context, userID uuid.UUID, from, to time.Time, bufferMinutes int) ([]domain.Activity, error) {
	var activities []domain.Activity
	buf := time.Duration(bufferMinutes) * time.Minute
	// The ended_at IS NULL arm is anchored with a 24-hour look-back on started_at so that
	// stale open sessions (e.g. from an app crash days ago) are not returned as candidates.
	anchor := from.Add(-24 * time.Hour)
	err := r.db.WithContext(ctx).
		Where(
			"user_id = ? AND started_at <= ? AND ("+
				"(ended_at IS NULL AND started_at >= ?) OR "+
				"ended_at >= ?"+
				")",
			userID, to.Add(buf), anchor, from.Add(-buf),
		).
		Find(&activities).Error
	return activities, err
}

// EnrichReceiptInput carries the receipt fields merged into an existing GPS activity.
type EnrichReceiptInput struct {
	ReceiptID       string
	Provider        string
	Origin          string
	Destination     string
	FareAmount      *float64
	FareCurrency    *string
	DistanceKM      *float64
	DurationMinutes *int
	// Coords are only applied when the existing activity has nil values for them.
	OriginLat *float64
	OriginLng *float64
	DestLat   *float64
	DestLng   *float64
	// MatchConfidence records the dedup score for auditability.
	MatchConfidence float64
}

// EnrichFromReceipt merges receipt data into an existing GPS activity.
// Field priority: GPS keeps its coords; receipt wins for fare/distance/duration/provider.
// The activity's source is updated to "gps+receipt".
func (r *ActivityRepository) EnrichFromReceipt(ctx context.Context, id uuid.UUID, input EnrichReceiptInput) error {
	updates := map[string]any{
		"source":           domain.SourceGPSReceipt,
		"match_confidence": input.MatchConfidence,
	}
	if input.ReceiptID != "" {
		updates["receipt_id"] = input.ReceiptID
	}
	if input.Provider != "" {
		updates["provider"] = input.Provider
	}
	if input.Origin != "" {
		updates["origin"] = input.Origin
	}
	if input.Destination != "" {
		updates["destination"] = input.Destination
	}
	if input.FareAmount != nil {
		updates["fare_amount"] = *input.FareAmount
	}
	if input.FareCurrency != nil {
		updates["fare_currency"] = *input.FareCurrency
	}
	if input.DistanceKM != nil {
		updates["distance_km"] = *input.DistanceKM
	}
	if input.DurationMinutes != nil {
		updates["duration_minutes"] = *input.DurationMinutes
	}
	// Coords are applied only when the caller explicitly provides them.
	// The service layer decides which fields to pass based on what the GPS
	// activity already has (non-nil GPS coords are not overwritten).
	if input.OriginLat != nil {
		updates["origin_lat"] = *input.OriginLat
	}
	if input.OriginLng != nil {
		updates["origin_lng"] = *input.OriginLng
	}
	if input.DestLat != nil {
		updates["dest_lat"] = *input.DestLat
	}
	if input.DestLng != nil {
		updates["dest_lng"] = *input.DestLng
	}
	return r.db.WithContext(ctx).
		Model(&domain.Activity{}).
		Where("id = ?", id).
		Updates(updates).Error
}
