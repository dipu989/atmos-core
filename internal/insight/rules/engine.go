package rules

import (
	"context"
	"time"

	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Rule is implemented by every insight rule.
type Rule interface {
	Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) (*insightdomain.Insight, error)
}

// Engine runs all registered rules and persists the resulting insights.
type Engine struct {
	repo  *insightrepo.InsightRepository
	bus   eventbus.Bus
	rules []Rule
}

func NewEngine(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository, bus eventbus.Bus) *Engine {
	return &Engine{
		repo: repo,
		bus:  bus,
		rules: []Rule{
			NewStreakRule(repo),
			NewWeeklyComparisonRule(repo, summaryRepo),
			NewModeSpikeRule(repo, summaryRepo),
			NewModeSummaryRule(repo, summaryRepo),
		},
	}
}

// Evaluate runs every rule for the given user and date, persisting non-nil results.
// Publishes EventInsightCreated for each insight so the notification service can push to devices.
func (e *Engine) Evaluate(ctx context.Context, userID uuid.UUID, date time.Time) {
	for _, rule := range e.rules {
		insight, err := rule.Evaluate(ctx, userID, date)
		if err != nil {
			logger.L().Warn("insight rule failed", zap.Error(err))
			continue
		}
		if insight == nil {
			continue
		}
		if err := e.repo.Create(ctx, insight); err != nil {
			logger.L().Error("failed to persist insight", zap.Error(err))
			continue
		}
		e.bus.Publish(ctx, eventbus.Event{
			Type:    insightdomain.EventInsightCreated,
			Payload: insightdomain.InsightCreatedPayload{Insight: insight},
		})
	}
}
