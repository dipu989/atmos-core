package service

import (
	"context"
	"time"

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

func NewInsightService(repo *insightrepo.InsightRepository, summaryRepo *timerepo.SummaryRepository, bus eventbus.Bus) *InsightService {
	return &InsightService{
		repo:   repo,
		engine: rules.NewEngine(repo, summaryRepo, bus),
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

func (s *InsightService) ListInsights(ctx context.Context, userID uuid.UUID, onlyUnread bool, limit, offset int, period string) (*dto.InsightsPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	from, to := periodRange(period, time.Now())

	items, err := s.repo.ListForUser(ctx, userID, onlyUnread, limit, offset, from, to)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountForUser(ctx, userID, onlyUnread, from, to)
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

// periodRange returns the [from, to] date window for a Week/Month/Year filter,
// anchored on now. Unrecognized period values fall back to "week" rather than
// erroring, so the endpoint stays permissive for older/unknown clients.
//
// "week" uses a Monday-start window to match the boundary every insight rule
// already writes to PeriodStart/PeriodEnd (see internal/insight/rules/helpers.go's
// isoWeekStart) — duplicated here rather than imported, since rules.isoWeekStart
// is unexported and this is the only place outside that package that needs it.
func periodRange(period string, now time.Time) (from, to time.Time) {
	now = now.UTC().Truncate(24 * time.Hour)

	switch period {
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 1, 0).AddDate(0, 0, -1)
	case "year":
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		to = time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	default: // "week", or anything unrecognized
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		from = now.AddDate(0, 0, -(weekday - 1))
		to = from.AddDate(0, 0, 6)
	}
	return from, to
}
