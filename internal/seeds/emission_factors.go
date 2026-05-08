package seeds

import (
	"context"
	"time"

	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	"github.com/dipu/atmos-core/platform/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmissionFactorsSeeder struct{}

func (s *EmissionFactorsSeeder) Name() string { return "EmissionFactors" }

func (s *EmissionFactorsSeeder) Run(ctx context.Context, db *gorm.DB) error {
	effectiveFrom, _ := time.Parse("2006-01-02", "2023-01-01")

	ptr := func(v string) *string { return &v }
	flt := func(v float64) *float64 { return &v }

	factors := []emidomain.EmissionFactor{
		// --- Cab / Taxi ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("petrol"), Region: "IN", KgCO2ePerKM: flt(0.17100), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("cng"), Region: "IN", KgCO2ePerKM: flt(0.10200), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.05500), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Auto Rickshaw ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("auto_rickshaw"), FuelType: ptr("cng"), Region: "IN", KgCO2ePerKM: flt(0.07200), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Bus ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("bus"), FuelType: ptr("diesel"), Region: "IN", KgCO2ePerKM: flt(0.08900), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Metro ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("metro"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.03100), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Train ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("train"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.04100), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Two Wheeler ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("two_wheeler"), FuelType: ptr("petrol"), Region: "IN", KgCO2ePerKM: flt(0.11300), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("two_wheeler"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.04200), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Zero emission ---
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("walk"), Region: "global", KgCO2ePerKM: flt(0.0), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("bicycle"), Region: "global", KgCO2ePerKM: flt(0.0), SourceName: "DEFRA_2023", EffectiveFrom: effectiveFrom},
		// --- Flight ---
		{ID: uuid.New(), ActivityType: "flight", TransportMode: ptr("flight"), Region: "global", KgCO2ePerKM: flt(0.25500), SourceName: "IPCC_2023", EffectiveFrom: effectiveFrom},
	}

	// Upsert on (activity_type, transport_mode, fuel_type, region, effective_from)
	// so re-running is safe and factors can be updated by changing the value.
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(factors, 20).Error
}
