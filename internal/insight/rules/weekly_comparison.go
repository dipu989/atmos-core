package rules

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WeeklyComparisonRule fires when this week's emissions differ from last week's by ≥10%.
type WeeklyComparisonRule struct {
	repo        *insightrepo.InsightRepository
	summaryRepo *timerepo.SummaryRepository
}

func NewWeeklyComparisonRule(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository) *WeeklyComparisonRule {
	return &WeeklyComparisonRule{repo: repo, summaryRepo: summaryRepo}
}

func (r *WeeklyComparisonRule) Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) (*insightdomain.Insight, error) {
	currStart := isoWeekStart(date)
	prevStart := currStart.AddDate(0, 0, -7)

	curr, err := r.summaryRepo.GetWeekly(ctx, userID, currStart)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	prev, err := r.summaryRepo.GetWeekly(ctx, userID, prevStart)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if prev.TotalKgCO2e == 0 || curr.ActivityCount == 0 {
		return nil, nil
	}

	changePct := (curr.TotalKgCO2e - prev.TotalKgCO2e) / prev.TotalKgCO2e * 100
	if math.Abs(changePct) < 10 {
		return nil, nil
	}

	exists, err := r.repo.ExistsForPeriod(ctx, userID, insightdomain.InsightWeeklyComparison, currStart)
	if err != nil || exists {
		return nil, err
	}

	id, _ := uuid.NewV7()
	weekEnd := currStart.AddDate(0, 0, 6)

	var title, body string
	if changePct > 0 {
		title = fmt.Sprintf("Emissions up %.0f%% this week", changePct)
		body = fmt.Sprintf(
			"You emitted %.2f kg CO₂e this week, %.0f%% more than last week's %.2f kg. Consider swapping one cab ride for metro or cycling.",
			curr.TotalKgCO2e, changePct, prev.TotalKgCO2e,
		)
	} else {
		title = fmt.Sprintf("Emissions down %.0f%% this week!", math.Abs(changePct))
		body = fmt.Sprintf(
			"Great work — you emitted %.2f kg CO₂e this week, %.0f%% less than last week's %.2f kg. Keep it up!",
			curr.TotalKgCO2e, math.Abs(changePct), prev.TotalKgCO2e,
		)
	}

	return &insightdomain.Insight{
		ID:          id,
		UserID:      userID,
		InsightType: insightdomain.InsightWeeklyComparison,
		PeriodType:  insightdomain.PeriodWeekly,
		PeriodStart: currStart,
		PeriodEnd:   weekEnd,
		Title:       title,
		Body:        body,
		Metadata: insightdomain.InsightMetadata{
			"current_kg_co2e":  curr.TotalKgCO2e,
			"previous_kg_co2e": prev.TotalKgCO2e,
			"change_pct":       math.Round(changePct*10) / 10,
		},
	}, nil
}
