package rules

import (
	"context"
	"errors"
	"fmt"
	"time"

	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	timedomain "github.com/dipu/atmos-core/internal/timeline/domain"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// highEmissionModes are tracked for spike detection.
var highEmissionModes = []string{"cab", "car", "rideshare"}

// ModeSpikeRule fires when cab/car usage this week is ≥50% above the 4-week average.
type ModeSpikeRule struct {
	repo        *insightrepo.InsightRepository
	summaryRepo *timerepo.SummaryRepository
}

func NewModeSpikeRule(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository) *ModeSpikeRule {
	return &ModeSpikeRule{repo: repo, summaryRepo: summaryRepo}
}

func (r *ModeSpikeRule) Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) (*insightdomain.Insight, error) {
	currStart := isoWeekStart(date)

	curr, err := r.summaryRepo.GetWeekly(ctx, userID, currStart)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if curr.ActivityCount == 0 {
		return nil, nil
	}

	currCabKg := modeKgSum(curr.Breakdown, highEmissionModes)
	if currCabKg == 0 {
		return nil, nil
	}

	// Average the same modes over the previous 4 weeks
	historyStart := currStart.AddDate(0, 0, -28)
	historyEnd := currStart.AddDate(0, 0, -7)

	weeks, err := r.summaryRepo.ListWeeklyRange(ctx, userID, historyStart, historyEnd)
	if err != nil {
		return nil, err
	}
	if len(weeks) == 0 {
		return nil, nil
	}

	var histTotal float64
	for _, w := range weeks {
		histTotal += modeKgSum(w.Breakdown, highEmissionModes)
	}
	histAvg := histTotal / float64(len(weeks))

	if histAvg == 0 || currCabKg < histAvg*1.5 {
		return nil, nil
	}

	exists, err := r.repo.ExistsForPeriod(ctx, userID, insightdomain.InsightModeSpike, currStart)
	if err != nil || exists {
		return nil, err
	}

	spikePct := (currCabKg - histAvg) / histAvg * 100
	weekEnd := currStart.AddDate(0, 0, 6)
	id, _ := uuid.NewV7()

	return &insightdomain.Insight{
		ID:          id,
		UserID:      userID,
		InsightType: insightdomain.InsightModeSpike,
		PeriodType:  insightdomain.PeriodWeekly,
		PeriodStart: currStart,
		PeriodEnd:   weekEnd,
		Title:       fmt.Sprintf("High-emission travel up %.0f%% this week", spikePct),
		Body: fmt.Sprintf(
			"You spent %.2f kg CO₂e on cab or car rides this week — %.0f%% above your 4-week average of %.2f kg. Switching even one trip to metro or walking can make a difference.",
			currCabKg, spikePct, histAvg,
		),
		Metadata: insightdomain.InsightMetadata{
			"current_kg_co2e":   currCabKg,
			"avg_4week_kg_co2e": histAvg,
			"spike_pct":         spikePct,
			"weeks_in_history":  len(weeks),
		},
	}, nil
}

func modeKgSum(b timedomain.Breakdown, modes []string) float64 {
	var total float64
	for _, mode := range modes {
		if mb, ok := b[mode]; ok {
			total += mb.KgCO2e
		}
	}
	return total
}
