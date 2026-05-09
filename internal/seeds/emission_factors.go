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
	ptr := func(v string) *string { return &v }
	flt := func(v float64) *float64 { return &v }

	// ── Specific factors (2023) ───────────────────────────────────────────────
	// These are fuel/vehicle-specific. They lose to canonical factors in the
	// effective_from ordering, but will be selected once the app passes fuel
	// type information through to the resolution query.
	specificFrom, _ := time.Parse("2006-01-02", "2023-01-01")

	specific := []emidomain.EmissionFactor{
		// Cab / Taxi
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("petrol"), Region: "IN", KgCO2ePerKM: flt(0.17100), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("cng"), Region: "IN", KgCO2ePerKM: flt(0.10200), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), VehicleType: ptr("sedan"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.05500), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Auto rickshaw
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("auto_rickshaw"), FuelType: ptr("cng"), Region: "IN", KgCO2ePerKM: flt(0.07200), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Bus
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("bus"), FuelType: ptr("diesel"), Region: "IN", KgCO2ePerKM: flt(0.08900), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Metro
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("metro"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.03100), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Train
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("train"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.04100), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Two Wheeler
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("two_wheeler"), FuelType: ptr("petrol"), Region: "IN", KgCO2ePerKM: flt(0.11300), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("two_wheeler"), FuelType: ptr("electric"), Region: "IN", KgCO2ePerKM: flt(0.04200), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		// Zero emission (legacy mode strings)
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("walk"), Region: "global", KgCO2ePerKM: flt(0.0), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("bicycle"), Region: "global", KgCO2ePerKM: flt(0.0), SourceName: "DEFRA_2023", EffectiveFrom: specificFrom},
	}

	// ── Canonical factors (2024) ──────────────────────────────────────────────
	// One factor per mode, no fuel/vehicle type. effective_from is newer so they
	// win over the specific 2023 entries when no fuel info is available.
	// Values per the product spec: emissions = distance_km × factor.
	// Flight factor is pre-RFI; the calculator applies the 1.9× multiplier.
	canonicalFrom, _ := time.Parse("2006-01-02", "2024-01-01")

	// Region="IN" so these win over the 2023 fuel-specific IN entries in the
	// effective_from DESC ordering (candidate 1: mode+region).
	// Walking, cycling, and flight have no regional variation → "global".
	canonical := []emidomain.EmissionFactor{
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("walking"), Region: "global", KgCO2ePerKM: flt(0.000), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cycling"), Region: "global", KgCO2ePerKM: flt(0.000), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("metro"), Region: "IN", KgCO2ePerKM: flt(0.040), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("bus"), Region: "IN", KgCO2ePerKM: flt(0.080), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("train"), Region: "IN", KgCO2ePerKM: flt(0.050), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("car"), Region: "IN", KgCO2ePerKM: flt(0.190), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "transport", TransportMode: ptr("cab"), Region: "IN", KgCO2ePerKM: flt(0.210), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
		{ID: uuid.New(), ActivityType: "flight", TransportMode: ptr("flight"), Region: "global", KgCO2ePerKM: flt(0.255), SourceName: "Atmos_2024", EffectiveFrom: canonicalFrom},
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(specific, 20).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(canonical, 20).Error
}
