package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/insight/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InsightRepository struct {
	db *gorm.DB
}

func NewInsightRepository(db *gorm.DB) *InsightRepository {
	return &InsightRepository{db: db}
}

func (r *InsightRepository) Create(ctx context.Context, insight *domain.Insight) error {
	return r.db.WithContext(ctx).Create(insight).Error
}

func (r *InsightRepository) ExistsForPeriod(ctx context.Context, userID uuid.UUID, insightType domain.InsightType, periodStart time.Time) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Insight{}).
		Where("user_id = ? AND insight_type = ? AND period_start = ?", userID, insightType, periodStart.Format("2006-01-02")).
		Count(&count).Error
	return count > 0, err
}

func (r *InsightRepository) ListForUser(ctx context.Context, userID uuid.UUID, onlyUnread bool, limit, offset int) ([]domain.Insight, error) {
	var insights []domain.Insight
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if onlyUnread {
		q = q.Where("is_read = FALSE")
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&insights).Error
	return insights, err
}

func (r *InsightRepository) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&domain.Insight{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true).Error
}

func (r *InsightRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.Insight, error) {
	var insight domain.Insight
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&insight).Error
	if err != nil {
		return nil, err
	}
	return &insight, nil
}

func (r *InsightRepository) CountForUser(ctx context.Context, userID uuid.UUID, onlyUnread bool) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&domain.Insight{}).Where("user_id = ?", userID)
	if onlyUnread {
		q = q.Where("is_read = FALSE")
	}
	err := q.Count(&count).Error
	return count, err
}

// CountConsecutiveDaysWithActivity returns the number of consecutive days (ending on or before endDate)
// that the user has at least one processed activity.
func (r *InsightRepository) CountConsecutiveDaysWithActivity(ctx context.Context, userID uuid.UUID, endDate time.Time) (int, error) {
	type row struct {
		DateLocal time.Time `gorm:"column:date_local"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT date_local
		FROM activities
		WHERE user_id = ? AND status = 'processed' AND date_local <= ?
		ORDER BY date_local DESC
		LIMIT 365
	`, userID, endDate.Format("2006-01-02")).Scan(&rows).Error
	if err != nil {
		return 0, err
	}

	streak := 0
	expected := endDate.Truncate(24 * time.Hour)
	for _, r := range rows {
		d := r.DateLocal.Truncate(24 * time.Hour)
		if !d.Equal(expected) {
			break
		}
		streak++
		expected = expected.AddDate(0, 0, -1)
	}
	return streak, nil
}
