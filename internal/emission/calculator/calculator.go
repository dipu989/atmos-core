package calculator

import (
	"errors"

	"github.com/dipu/atmos-core/internal/emission/domain"
)

// aviationRFI is the Radiative Forcing Index applied to flight emissions.
// It accounts for the additional warming effect of contrails and other
// non-CO₂ aviation impacts at altitude. Source: IPCC AR6.
const aviationRFI = 1.9

// Calculate applies the correct emission formula based on the factor's available unit rates.
// Priority: per-km → per-kwh → flat.
// For flights the aviationRFI multiplier is applied on top of per-km.
func Calculate(factor *domain.EmissionFactor, activityType string, distanceKM *float64, kwh *float64) (float64, error) {
	if factor.KgCO2ePerKM != nil {
		if distanceKM == nil {
			return 0, errors.New("emission factor requires distance but activity has no distance_km")
		}
		kg := *factor.KgCO2ePerKM * *distanceKM
		if activityType == "flight" {
			kg *= aviationRFI
		}
		return kg, nil
	}

	if factor.KgCO2ePerKWH != nil {
		if kwh == nil {
			return 0, errors.New("emission factor requires energy consumption but activity has no energy_kwh")
		}
		return *factor.KgCO2ePerKWH * *kwh, nil
	}

	if factor.KgCO2eFlat != nil {
		return *factor.KgCO2eFlat, nil
	}

	return 0, errors.New("emission factor has no applicable calculation unit for the provided activity data")
}
