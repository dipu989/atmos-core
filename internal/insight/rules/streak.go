package rules

import (
	"context"
	"fmt"
	"time"

	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	"github.com/google/uuid"
)

type StreakRule struct {
	repo *insightrepo.InsightRepository
}

func NewStreakRule(repo *insightrepo.InsightRepository) *StreakRule {
	return &StreakRule{repo: repo}
}

// Evaluate checks if the user has a notable streak and creates an insight if so.
// Milestones: 3, 7, 14, 30 consecutive days of tracked activity.
var streakMilestones = []int{3, 7, 14, 30}

func (r *StreakRule) Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) (*insightdomain.Insight, error) {
	streak, err := r.repo.CountConsecutiveDaysWithActivity(ctx, userID, date)
	if err != nil || streak == 0 {
		return nil, err
	}

	for _, milestone := range streakMilestones {
		if streak == milestone {
			alreadyExists, _ := r.repo.ExistsForPeriod(ctx, userID, insightdomain.InsightStreak, date)
			if alreadyExists {
				return nil, nil
			}

			id, _ := uuid.NewV7()
			return &insightdomain.Insight{
				ID:          id,
				UserID:      userID,
				InsightType: insightdomain.InsightStreak,
				PeriodType:  insightdomain.PeriodDaily,
				PeriodStart: date,
				PeriodEnd:   date,
				Title:       fmt.Sprintf("%d-day tracking streak!", milestone),
				Body:        fmt.Sprintf("You've logged your environmental impact for %d days in a row. Keep it going!", milestone),
				Metadata: insightdomain.InsightMetadata{
					"streak_days": streak,
				},
			}, nil
		}
	}
	return nil, nil
}
