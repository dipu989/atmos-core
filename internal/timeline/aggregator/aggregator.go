package aggregator

import (
	"context"
	"time"

	timedomain "github.com/dipu/atmos-core/internal/timeline/domain"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/google/uuid"
)

type Aggregator struct {
	repo *timerepo.SummaryRepository
}

func NewAggregator(repo *timerepo.SummaryRepository) *Aggregator {
	return &Aggregator{repo: repo}
}

func (a *Aggregator) RecomputeDay(ctx context.Context, userID uuid.UUID, date time.Time) error {
	rows, err := a.repo.AggregateDailyFromDB(ctx, userID, date)
	if err != nil {
		return err
	}

	var totalKg, totalDist float64
	var totalCount int
	breakdown := timedomain.Breakdown{}

	for _, r := range rows {
		modeKey := "other"
		if r.TransportMode != nil {
			modeKey = *r.TransportMode
		}
		breakdown[modeKey] = timedomain.ModeBreakdown{
			KgCO2e:     r.TotalKgCO2e,
			DistanceKM: r.TotalDistanceKM,
			Count:      r.ActivityCount,
		}
		totalKg += r.TotalKgCO2e
		totalDist += r.TotalDistanceKM
		totalCount += r.ActivityCount
	}

	id, _ := uuid.NewV7()
	daily := &timedomain.DailySummary{
		ID:              id,
		UserID:          userID,
		DateLocal:       date,
		TotalKgCO2e:     totalKg,
		TotalDistanceKM: totalDist,
		ActivityCount:   totalCount,
		Breakdown:       breakdown,
		ComputedAt:      time.Now(),
	}
	if err := a.repo.UpsertDaily(ctx, daily); err != nil {
		return err
	}

	// Roll up to weekly
	weekStart := isoWeekStart(date)
	if err := a.recomputeWeek(ctx, userID, weekStart); err != nil {
		return err
	}

	// Roll up to monthly
	return a.recomputeMonth(ctx, userID, date.Year(), int(date.Month()))
}

func (a *Aggregator) recomputeWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) error {
	weekEnd := weekStart.AddDate(0, 0, 6)

	dailies, err := a.repo.ListDailyRange(ctx, userID, weekStart, weekEnd)
	if err != nil {
		return err
	}

	var totalKg, totalDist float64
	var totalCount int
	breakdown := timedomain.Breakdown{}

	for _, d := range dailies {
		totalKg += d.TotalKgCO2e
		totalDist += d.TotalDistanceKM
		totalCount += d.ActivityCount
		for mode, mb := range d.Breakdown {
			existing := breakdown[mode]
			breakdown[mode] = timedomain.ModeBreakdown{
				KgCO2e:     existing.KgCO2e + mb.KgCO2e,
				DistanceKM: existing.DistanceKM + mb.DistanceKM,
				Count:      existing.Count + mb.Count,
			}
		}
	}

	id, _ := uuid.NewV7()
	weekly := &timedomain.WeeklySummary{
		ID:              id,
		UserID:          userID,
		WeekStart:       weekStart,
		WeekEnd:         weekEnd,
		TotalKgCO2e:     totalKg,
		TotalDistanceKM: totalDist,
		ActivityCount:   totalCount,
		Breakdown:       breakdown,
		ComputedAt:      time.Now(),
	}
	return a.repo.UpsertWeekly(ctx, weekly)
}

func (a *Aggregator) recomputeMonth(ctx context.Context, userID uuid.UUID, year, month int) error {
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)

	dailies, err := a.repo.ListDailyRange(ctx, userID, from, to)
	if err != nil {
		return err
	}

	var totalKg, totalDist float64
	var totalCount int
	breakdown := timedomain.Breakdown{}

	for _, d := range dailies {
		totalKg += d.TotalKgCO2e
		totalDist += d.TotalDistanceKM
		totalCount += d.ActivityCount
		for mode, mb := range d.Breakdown {
			existing := breakdown[mode]
			breakdown[mode] = timedomain.ModeBreakdown{
				KgCO2e:     existing.KgCO2e + mb.KgCO2e,
				DistanceKM: existing.DistanceKM + mb.DistanceKM,
				Count:      existing.Count + mb.Count,
			}
		}
	}

	id, _ := uuid.NewV7()
	monthly := &timedomain.MonthlySummary{
		ID:              id,
		UserID:          userID,
		Year:            year,
		Month:           month,
		TotalKgCO2e:     totalKg,
		TotalDistanceKM: totalDist,
		ActivityCount:   totalCount,
		Breakdown:       breakdown,
		ComputedAt:      time.Now(),
	}
	return a.repo.UpsertMonthly(ctx, monthly)
}

// isoWeekStart returns the Monday of the ISO week containing t.
func isoWeekStart(t time.Time) time.Time {
	t = t.UTC().Truncate(24 * time.Hour)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7 in ISO
	}
	return t.AddDate(0, 0, -(weekday - 1))
}
