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

// ModeSummaryRule generates a weekly transport pattern summary once per week
// when the user has at least 3 activities.
type ModeSummaryRule struct {
	repo        *insightrepo.InsightRepository
	summaryRepo *timerepo.SummaryRepository
}

func NewModeSummaryRule(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository) *ModeSummaryRule {
	return &ModeSummaryRule{repo: repo, summaryRepo: summaryRepo}
}

func (r *ModeSummaryRule) Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) (*insightdomain.Insight, error) {
	currStart := isoWeekStart(date)

	weekly, err := r.summaryRepo.GetWeekly(ctx, userID, currStart)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if weekly.ActivityCount < 3 || len(weekly.Breakdown) == 0 {
		return nil, nil
	}

	exists, err := r.repo.ExistsForPeriod(ctx, userID, insightdomain.InsightModeSummary, currStart)
	if err != nil || exists {
		return nil, err
	}

	top, topKg := dominantMode(weekly.Breakdown)
	lowEmissionKg := lowEmissionTotal(weekly.Breakdown)
	weekEnd := currStart.AddDate(0, 0, 6)
	id, _ := uuid.NewV7()

	title := fmt.Sprintf("This week's top mode: %s", top)
	body := fmt.Sprintf(
		"Your most-used mode this week was %s (%.2f kg CO₂e). "+
			"Low-emission travel (walking, cycling, metro) accounted for %.2f kg out of %.2f kg total.",
		top, topKg, lowEmissionKg, weekly.TotalKgCO2e,
	)

	return &insightdomain.Insight{
		ID:          id,
		UserID:      userID,
		InsightType: insightdomain.InsightModeSummary,
		PeriodType:  insightdomain.PeriodWeekly,
		PeriodStart: currStart,
		PeriodEnd:   weekEnd,
		Title:       title,
		Body:        body,
		Metadata: insightdomain.InsightMetadata{
			"top_mode":         top,
			"top_mode_kg_co2e": topKg,
			"low_emission_kg":  lowEmissionKg,
			"total_kg_co2e":    weekly.TotalKgCO2e,
			"activity_count":   weekly.ActivityCount,
		},
	}, nil
}

func dominantMode(b timedomain.Breakdown) (string, float64) {
	var topMode string
	var topKg float64
	for mode, mb := range b {
		if mb.KgCO2e > topKg {
			topKg = mb.KgCO2e
			topMode = mode
		}
	}
	return topMode, topKg
}

var lowEmissionSet = map[string]bool{
	"walking": true,
	"cycling": true,
	"metro":   true,
	"train":   true,
	"bus":     true,
	"tram":    true,
}

func lowEmissionTotal(b timedomain.Breakdown) float64 {
	var total float64
	for mode, mb := range b {
		if lowEmissionSet[mode] {
			total += mb.KgCO2e
		}
	}
	return total
}
