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

// FindFactorRegionByActivityID returns the region of the emission factor that
// was actually used to calculate the given activity's emission, in a single
// joined query — used to pin alternative-mode comparisons to the same region
// rather than re-resolving the user's current preference.
func (r *EmissionRepository) FindFactorRegionByActivityID(ctx context.Context, activityID uuid.UUID) (string, error) {
	var region string
	err := r.db.WithContext(ctx).
		Table("emissions").
		Joins("JOIN emission_factors ON emission_factors.id = emissions.emission_factor_id").
		Where("emissions.activity_id = ?", activityID).
		Select("emission_factors.region").
		Scan(&region).Error
	if err != nil {
		return "", err
	}
	if region == "" {
		return "", gorm.ErrRecordNotFound
	}
	return region, nil
}

// ResolveFactor finds the best-matching emission factor using specificity priority.
// Waterfall: fuel-type-specific candidates first (when fuelType is non-nil),
// then canonical (no fuel type), falling back through region → global.
func (r *EmissionRepository) ResolveFactor(ctx context.Context, activityType string, transportMode *string, fuelType *string, region string, on time.Time) (*domain.EmissionFactor, error) {
	dateStr := on.Format("2006-01-02")

	type candidate struct {
		mode     *string
		fuelType *string
		region   string
	}

	var candidates []candidate
	// When fuel type is known, prefer fuel-specific rows first.
	if fuelType != nil {
		candidates = append(candidates,
			candidate{transportMode, fuelType, region},
			candidate{transportMode, fuelType, "global"},
		)
	}
	// Canonical (no fuel type) and progressively less specific fallbacks.
	candidates = append(candidates,
		candidate{transportMode, nil, region},
		candidate{transportMode, nil, "global"},
		candidate{nil, nil, region},
		candidate{nil, nil, "global"},
	)

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
		if c.fuelType != nil {
			query = query.Where("fuel_type = ?", *c.fuelType)
		} else {
			query = query.Where("fuel_type IS NULL")
		}
		query = query.Where("region = ?", c.region)

		var factor domain.EmissionFactor
		if err := query.Order("effective_from DESC").First(&factor).Error; err == nil {
			return &factor, nil
		}
	}

	return nil, gorm.ErrRecordNotFound
}
