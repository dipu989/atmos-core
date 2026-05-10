package dto

import "github.com/dipu/atmos-core/internal/insight/domain"

// InsightsPage is the paginated response envelope for the list-insights endpoint.
type InsightsPage struct {
	Items  []domain.Insight `json:"items"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}
