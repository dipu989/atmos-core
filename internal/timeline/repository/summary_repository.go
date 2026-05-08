package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/timeline/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SummaryRepository struct {
	db *gorm.DB
}

func NewSummaryRepository(db *gorm.DB) *SummaryRepository {
	return &SummaryRepository{db: db}
}

// --- Daily ---

func (r *SummaryRepository) UpsertDaily(ctx context.Context, s *domain.DailySummary) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "date_local"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_kg_co2e", "total_distance_km", "activity_count", "breakdown", "computed_at", "updated_at",
			}),
		}).
		Create(s).Error
}

func (r *SummaryRepository) GetDaily(ctx context.Context, userID uuid.UUID, date time.Time) (*domain.DailySummary, error) {
	var s domain.DailySummary
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND date_local = ?", userID, date.Format("2006-01-02")).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SummaryRepository) ListDailyRange(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]domain.DailySummary, error) {
	var summaries []domain.DailySummary
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND date_local BETWEEN ? AND ?", userID, from.Format("2006-01-02"), to.Format("2006-01-02")).
		Order("date_local DESC").
		Find(&summaries).Error
	return summaries, err
}

// --- Weekly ---

func (r *SummaryRepository) UpsertWeekly(ctx context.Context, s *domain.WeeklySummary) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "week_start"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_kg_co2e", "total_distance_km", "activity_count", "breakdown", "computed_at", "updated_at",
			}),
		}).
		Create(s).Error
}

func (r *SummaryRepository) GetWeekly(ctx context.Context, userID uuid.UUID, weekStart time.Time) (*domain.WeeklySummary, error) {
	var s domain.WeeklySummary
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND week_start = ?", userID, weekStart.Format("2006-01-02")).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// --- Monthly ---

func (r *SummaryRepository) UpsertMonthly(ctx context.Context, s *domain.MonthlySummary) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "year"}, {Name: "month"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_kg_co2e", "total_distance_km", "activity_count", "breakdown", "computed_at", "updated_at",
			}),
		}).
		Create(s).Error
}

func (r *SummaryRepository) GetMonthly(ctx context.Context, userID uuid.UUID, year, month int) (*domain.MonthlySummary, error) {
	var s domain.MonthlySummary
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND year = ? AND month = ?", userID, year, month).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// AggregateDaily re-computes daily totals directly from the emissions table.
// This is the source of truth for upserts — not incremental arithmetic.
type DailyAggregate struct {
	TransportMode   *string
	TotalKgCO2e     float64
	TotalDistanceKM float64
	ActivityCount   int
}

func (r *SummaryRepository) AggregateDailyFromDB(ctx context.Context, userID uuid.UUID, date time.Time) ([]DailyAggregate, error) {
	type row struct {
		TransportMode   *string `gorm:"column:transport_mode"`
		TotalKgCO2e     float64 `gorm:"column:total_kg_co2e"`
		TotalDistanceKM float64 `gorm:"column:total_distance_km"`
		ActivityCount   int     `gorm:"column:activity_count"`
	}

	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			a.transport_mode,
			COALESCE(SUM(e.kg_co2e), 0)        AS total_kg_co2e,
			COALESCE(SUM(a.distance_km), 0)     AS total_distance_km,
			COUNT(*)                             AS activity_count
		FROM activities a
		JOIN emissions e ON e.activity_id = a.id
		WHERE a.user_id = ?
		  AND a.date_local = ?
		  AND a.status = 'processed'
		GROUP BY a.transport_mode
	`, userID, date.Format("2006-01-02")).Scan(&rows).Error

	if err != nil {
		return nil, err
	}

	result := make([]DailyAggregate, len(rows))
	for i, r := range rows {
		result[i] = DailyAggregate{
			TransportMode:   r.TransportMode,
			TotalKgCO2e:     r.TotalKgCO2e,
			TotalDistanceKM: r.TotalDistanceKM,
			ActivityCount:   r.ActivityCount,
		}
	}
	return result, nil
}
