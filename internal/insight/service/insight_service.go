package service

import (
	"context"

	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	"github.com/dipu/atmos-core/internal/insight/dto"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	"github.com/dipu/atmos-core/internal/insight/rules"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/google/uuid"
)

type InsightService struct {
	repo   *insightrepo.InsightRepository
	engine *rules.Engine
}

func NewInsightService(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository) *InsightService {
	return &InsightService{
		repo:   repo,
		engine: rules.NewEngine(repo, summaryRepo),
	}
}

// HandleEmissionCalculated is subscribed to EventEmissionCalculated.
func (s *InsightService) HandleEmissionCalculated(ctx context.Context, event eventbus.Event) {
	payload, ok := event.Payload.(emidomain.EmissionCalculatedPayload)
	if !ok {
		return
	}
	s.engine.Evaluate(ctx, payload.UserID, payload.DateLocal)
}

func (s *InsightService) ListInsights(ctx context.Context, userID uuid.UUID, onlyUnread bool, limit, offset int) (*dto.InsightsPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	items, err := s.repo.ListForUser(ctx, userID, onlyUnread, limit, offset)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountForUser(ctx, userID, onlyUnread)
	if err != nil {
		return nil, err
	}

	return &dto.InsightsPage{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (s *InsightService) GetInsight(ctx context.Context, id, userID uuid.UUID) (*insightdomain.Insight, error) {
	return s.repo.FindByID(ctx, id, userID)
}

func (s *InsightService) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.MarkRead(ctx, id, userID)
}
