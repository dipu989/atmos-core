package service

import (
	"context"
	"time"

	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	"github.com/dipu/atmos-core/internal/timeline/aggregator"
	timedomain "github.com/dipu/atmos-core/internal/timeline/domain"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TimelineService struct {
	repo       *timerepo.SummaryRepository
	aggregator *aggregator.Aggregator
}

func NewTimelineService(repo *timerepo.SummaryRepository, agg *aggregator.Aggregator) *TimelineService {
	return &TimelineService{repo: repo, aggregator: agg}
}

// HandleEmissionCalculated is subscribed to EventEmissionCalculated.
func (s *TimelineService) HandleEmissionCalculated(ctx context.Context, event eventbus.Event) {
	payload, ok := event.Payload.(emidomain.EmissionCalculatedPayload)
	if !ok {
		return
	}

	if err := s.aggregator.RecomputeDay(ctx, payload.UserID, payload.DateLocal); err != nil {
		logger.L().Error("timeline aggregation failed",
			zap.String("user_id", payload.UserID.String()),
			zap.String("date", payload.DateLocal.Format("2006-01-02")),
			zap.Error(err),
		)
	}
}

func (s *TimelineService) GetDay(ctx context.Context, userID uuid.UUID, date time.Time) (*timedomain.DailySummary, error) {
	return s.repo.GetDaily(ctx, userID, date)
}

func (s *TimelineService) GetWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) (*timedomain.WeeklySummary, error) {
	return s.repo.GetWeekly(ctx, userID, weekStart)
}

func (s *TimelineService) GetMonth(ctx context.Context, userID uuid.UUID, year, month int) (*timedomain.MonthlySummary, error) {
	return s.repo.GetMonthly(ctx, userID, year, month)
}

func (s *TimelineService) GetRange(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]timedomain.DailySummary, error) {
	return s.repo.ListDailyRange(ctx, userID, from, to)
}
