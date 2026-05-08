package repository

import (
	"context"
	"time"

	"github.com/dipu/atmos-core/internal/emission/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmissionRepository struct {
	db *gorm.DB
}

func NewEmissionRepository(db *gorm.DB) *EmissionRepository {
	return &EmissionRepository{db: db}
}

func (r *EmissionRepository) Upsert(ctx context.Context, emission *domain.Emission) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "activity_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"emission_factor_id", "kg_co2e", "calculation_version", "calculated_at", "updated_at",
			}),
		}).
		Create(emission).Error
}

func (r *EmissionRepository) FindByActivityID(ctx context.Context, activityID uuid.UUID) (*domain.Emission, error) {
	var e domain.Emission
	err := r.db.WithContext(ctx).Where("activity_id = ?", activityID).First(&e).Error
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ResolveFactor finds the best-matching emission factor using specificity priority.
// Order: most specific (all fields match) down to most general (activity_type + global region).
func (r *EmissionRepository) ResolveFactor(ctx context.Context, activityType string, transportMode *string, region string, on time.Time) (*domain.EmissionFactor, error) {
	dateStr := on.Format("2006-01-02")

	candidates := []struct {
		mode   *string
		region string
	}{
		{transportMode, region},
		{transportMode, "global"},
		{nil, region},
		{nil, "global"},
	}

	for _, c := range candidates {
		query := r.db.WithContext(ctx).
			Where("activity_type = ?", activityType).
			Where("effective_from <= ?", dateStr).
			Where("effective_until IS NULL OR effective_until >= ?", dateStr)

		if c.mode != nil {
			query = query.Where("transport_mode = ?", *c.mode)
		} else {
			query = query.Where("transport_mode IS NULL")
		}
		query = query.Where("region = ?", c.region)

		var factor domain.EmissionFactor
		if err := query.Order("effective_from DESC").First(&factor).Error; err == nil {
			return &factor, nil
		}
	}

	return nil, gorm.ErrRecordNotFound
}
