package service

import (
	"context"
	"time"

	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	"github.com/dipu/atmos-core/internal/insight/rules"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InsightService struct {
	repo       *insightrepo.InsightRepository
	streakRule *rules.StreakRule
}

func NewInsightService(repo *insightrepo.InsightRepository) *InsightService {
	return &InsightService{
		repo:       repo,
		streakRule: rules.NewStreakRule(repo),
	}
}

// HandleEmissionCalculated is subscribed to EventEmissionCalculated.
func (s *InsightService) HandleEmissionCalculated(ctx context.Context, event eventbus.Event) {
	payload, ok := event.Payload.(emidomain.EmissionCalculatedPayload)
	if !ok {
		return
	}

	s.evaluateRules(ctx, payload.UserID, payload.DateLocal)
}

func (s *InsightService) evaluateRules(ctx context.Context, userID uuid.UUID, date time.Time) {
	insight, err := s.streakRule.Evaluate(ctx, userID, date)
	if err != nil {
		logger.L().Warn("streak rule evaluation failed", zap.Error(err))
		return
	}
	if insight != nil {
		if err := s.repo.Create(ctx, insight); err != nil {
			logger.L().Error("failed to create insight", zap.Error(err))
		}
	}
}

func (s *InsightService) ListInsights(ctx context.Context, userID uuid.UUID, onlyUnread bool, limit, offset int) ([]insightdomain.Insight, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListForUser(ctx, userID, onlyUnread, limit, offset)
}

func (s *InsightService) GetInsight(ctx context.Context, id, userID uuid.UUID) (*insightdomain.Insight, error) {
	return s.repo.FindByID(ctx, id, userID)
}

func (s *InsightService) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.MarkRead(ctx, id, userID)
}
