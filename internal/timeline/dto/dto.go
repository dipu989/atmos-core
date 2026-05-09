package dto

import "github.com/dipu/atmos-core/internal/timeline/domain"

// TrendData compares the current period against the immediately preceding one.
// ChangePct is nil when there is no previous period data (division by zero avoided).
// Direction is "up", "down", or "flat".
type TrendData struct {
	PrevTotalKgCO2e float64  `json:"prev_total_kg_co2e"`
	ChangePct       *float64 `json:"change_pct"`
	Direction       string   `json:"direction"`
}

type DailySummaryResponse struct {
	domain.DailySummary
	Trend TrendData `json:"trend"`
}

type WeeklySummaryResponse struct {
	domain.WeeklySummary
	Trend TrendData `json:"trend"`
}

type MonthlySummaryResponse struct {
	domain.MonthlySummary
	Trend TrendData `json:"trend"`
}
