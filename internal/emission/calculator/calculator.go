package calculator

import (
	"errors"

	"github.com/dipu/atmos-core/internal/emission/domain"
)

// Calculate applies the correct emission formula based on the factor's available unit rates.
// Priority: per-km → per-kwh → flat.
// For flights a Radiative Forcing Index (RFI) multiplier of 1.9 is applied on top of per-km.
func Calculate(factor *domain.EmissionFactor, activityType string, distanceKM *float64, kwh *float64) (float64, error) {
	if factor.KgCO2ePerKM != nil && distanceKM != nil {
		kg := *factor.KgCO2ePerKM * *distanceKM
		if activityType == "flight" {
			kg *= 1.9 // Radiative Forcing Index for aviation
		}
		return kg, nil
	}

	if factor.KgCO2ePerKWH != nil && kwh != nil {
		return *factor.KgCO2ePerKWH * *kwh, nil
	}

	if factor.KgCO2eFlat != nil {
		return *factor.KgCO2eFlat, nil
	}

	return 0, errors.New("emission factor has no applicable calculation unit for the provided activity data")
}
