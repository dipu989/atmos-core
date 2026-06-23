// Package constants holds cited reference figures used to translate a kg CO2e
// value into relatable comparisons (trees, LED-hours, % of a global average).
// These are necessarily regional/climate-dependent approximations, not precise
// measurements — every consumer of these values must surface that caveat to
// users rather than presenting them as exact.
package constants

const (
	// TreeKgCO2ePerYear is the EPA Greenhouse Gas Equivalencies Calculator's
	// weighted average of coniferous/deciduous trees grown in an urban setting.
	// Source: https://www.epa.gov/energy/greenhouse-gas-equivalencies-calculator-calculations-and-references
	// Actual sequestration varies substantially by species, age, and climate.
	TreeKgCO2ePerYear = 60.0
	TreeKgCO2ePerDay  = TreeKgCO2ePerYear / 365.0

	// GlobalAvgKgCO2ePerYear is the global per-capita CO2 emissions figure
	// across all categories (energy, food, transport, goods) — not transport
	// alone. Source: Our World in Data / Global Carbon Budget (2025),
	// https://ourworldindata.org/co2-emissions
	// Varies enormously by country (from ~0.1 t/year in parts of Sub-Saharan
	// Africa to many multiples of the global average in high-income countries).
	GlobalAvgKgCO2ePerYear = 5000.0
	GlobalAvgKgCO2ePerDay  = GlobalAvgKgCO2ePerYear / 365.0

	// IndiaGridKgCO2ePerKWh is India's national grid carbon intensity average
	// (2024, CEA data) — already cited in atmos-mobile's PRODUCT_SPEC.md.
	// State-level variation is significant: ~0.4 in hydro-heavy Karnataka vs
	// ~0.9+ in UP/Bihar.
	IndiaGridKgCO2ePerKWh = 0.71

	// StandardLEDBulbKW approximates a common ~9W LED bulb (60W-incandescent
	// equivalent). LedKgCO2ePerHour is derived from this and the grid figure
	// above, rather than being its own uncited constant.
	StandardLEDBulbKW = 0.009
	LedKgCO2ePerHour  = IndiaGridKgCO2ePerKWh * StandardLEDBulbKW
)
