package service

import (
	"context"
	"errors"
	"time"

	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	"github.com/dipu/atmos-core/internal/timeline/aggregator"
	timedomain "github.com/dipu/atmos-core/internal/timeline/domain"
	"github.com/dipu/atmos-core/internal/timeline/dto"
	timerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
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

// --- Legacy methods used by path-param routes (kept for backward compat) ---

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

// --- Trend-enriched methods used by query-param routes ---

func (s *TimelineService) GetDailySummary(ctx context.Context, userID uuid.UUID, date time.Time) (*dto.DailySummaryResponse, error) {
	current, err := s.repo.GetDaily(ctx, userID, date)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			current = emptyDaily(userID, date)
		} else {
			return nil, err
		}
	}

	prevDate := date.AddDate(0, 0, -1)
	var prevKg float64
	if prev, err := s.repo.GetDaily(ctx, userID, prevDate); err == nil {
		prevKg = prev.TotalKgCO2e
	}

	return &dto.DailySummaryResponse{
		DailySummary: *current,
		Trend:        buildTrend(current.TotalKgCO2e, prevKg),
	}, nil
}

func (s *TimelineService) GetWeeklySummary(ctx context.Context, userID uuid.UUID, weekStart time.Time) (*dto.WeeklySummaryResponse, error) {
	current, err := s.repo.GetWeekly(ctx, userID, weekStart)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			current = emptyWeekly(userID, weekStart)
		} else {
			return nil, err
		}
	}

	prevWeekStart := weekStart.AddDate(0, 0, -7)
	var prevKg float64
	if prev, err := s.repo.GetWeekly(ctx, userID, prevWeekStart); err == nil {
		prevKg = prev.TotalKgCO2e
	}

	return &dto.WeeklySummaryResponse{
		WeeklySummary: *current,
		Trend:         buildTrend(current.TotalKgCO2e, prevKg),
	}, nil
}

func (s *TimelineService) GetMonthlySummary(ctx context.Context, userID uuid.UUID, year, month int) (*dto.MonthlySummaryResponse, error) {
	current, err := s.repo.GetMonthly(ctx, userID, year, month)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			current = emptyMonthly(userID, year, month)
		} else {
			return nil, err
		}
	}

	prevYear, prevMonth := prevMonthOf(year, month)
	var prevKg float64
	if prev, err := s.repo.GetMonthly(ctx, userID, prevYear, prevMonth); err == nil {
		prevKg = prev.TotalKgCO2e
	}

	return &dto.MonthlySummaryResponse{
		MonthlySummary: *current,
		Trend:          buildTrend(current.TotalKgCO2e, prevKg),
	}, nil
}

// --- helpers ---

func buildTrend(current, prev float64) dto.TrendData {
	t := dto.TrendData{PrevTotalKgCO2e: prev}
	if prev == 0 {
		t.Direction = "flat"
		return t
	}
	pct := ((current - prev) / prev) * 100
	t.ChangePct = &pct
	switch {
	case pct > 0.5:
		t.Direction = "up"
	case pct < -0.5:
		t.Direction = "down"
	default:
		t.Direction = "flat"
	}
	return t
}

func prevMonthOf(year, month int) (int, int) {
	if month == 1 {
		return year - 1, 12
	}
	return year, month - 1
}

func emptyDaily(userID uuid.UUID, date time.Time) *timedomain.DailySummary {
	return &timedomain.DailySummary{
		UserID:    userID,
		DateLocal: date,
		Breakdown: timedomain.Breakdown{},
	}
}

func emptyWeekly(userID uuid.UUID, weekStart time.Time) *timedomain.WeeklySummary {
	return &timedomain.WeeklySummary{
		UserID:    userID,
		WeekStart: weekStart,
		WeekEnd:   weekStart.AddDate(0, 0, 6),
		Breakdown: timedomain.Breakdown{},
	}
}

func emptyMonthly(userID uuid.UUID, year, month int) *timedomain.MonthlySummary {
	return &timedomain.MonthlySummary{
		UserID:    userID,
		Year:      year,
		Month:     month,
		Breakdown: timedomain.Breakdown{},
	}
}
