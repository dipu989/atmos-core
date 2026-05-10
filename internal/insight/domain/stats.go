package domain

import "time"

// WeekStats is a lightweight summary of a single ISO week used by insight rules.
type WeekStats struct {
	WeekStart     time.Time
	TotalKgCO2e   float64
	ActivityCount int
	Breakdown     map[string]ModeData
}

// ModeData holds per-mode aggregates extracted from weekly_summaries breakdown JSONB.
type ModeData struct {
	KgCO2e     float64
	DistanceKM float64
	Count      int
}
