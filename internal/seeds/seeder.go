package seeds

import (
	"context"

	"github.com/dipu/atmos-core/platform/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Seeder is implemented by each seed file.
type Seeder interface {
	Name() string
	Run(ctx context.Context, db *gorm.DB) error
}

// Run executes all registered seeders in order.
// Each seeder is idempotent — re-running is safe.
func Run(ctx context.Context, db *gorm.DB, seeders []Seeder) {
	log := logger.L()
	for _, s := range seeders {
		log.Info("running seeder", zap.String("name", s.Name()))
		if err := s.Run(ctx, db); err != nil {
			log.Fatal("seeder failed", zap.String("name", s.Name()), zap.Error(err))
		}
		log.Info("seeder complete", zap.String("name", s.Name()))
	}
}

// All returns all seeders in the order they should be applied.
func All() []Seeder {
	return []Seeder{
		&EmissionFactorsSeeder{},
	}
}
