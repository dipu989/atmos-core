package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/dipu/atmos-core/internal/gmail/domain"
	"gorm.io/gorm"
)

// RoutingMap maps a lower-cased sender email to all active ProviderEmailTypes
// that handle it. Built once per sync call from the DB.
type RoutingMap map[string][]domain.ProviderEmailType

// ProviderRepository loads reference data used to route incoming emails
// to the correct parser and to build the Gmail search query.
type ProviderRepository struct {
	db *gorm.DB
}

func NewProviderRepository(db *gorm.DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

// BuildRoutingMap loads all active ProviderEmailTypes and groups them by
// lower-cased sender_email. Called once at the start of each sync.
//
// Example result:
//
//	"noreply@uber.com"     → [{code:"uber_ride", …}]
//	"shoutout@rapido.bike" → [{code:"rapido_bike", …}, {code:"rapido_auto", …}]
func (r *ProviderRepository) BuildRoutingMap(ctx context.Context) (RoutingMap, error) {
	var types []domain.ProviderEmailType
	err := r.db.WithContext(ctx).
		Where("is_active = true").
		Find(&types).Error
	if err != nil {
		return nil, fmt.Errorf("provider_repository: load active types: %w", err)
	}

	m := make(RoutingMap, len(types))
	for _, t := range types {
		key := strings.ToLower(t.SenderEmail)
		m[key] = append(m[key], t)
	}
	return m, nil
}

// GmailQuery returns a Gmail search query string that covers all active senders.
// e.g. "from:(noreply@uber.com OR shoutout@rapido.bike)"
// Returns empty string if no active types exist.
func (r *ProviderRepository) GmailQuery(ctx context.Context) (string, error) {
	var senders []string
	err := r.db.WithContext(ctx).
		Model(&domain.ProviderEmailType{}).
		Where("is_active = true").
		Distinct("sender_email").
		Pluck("sender_email", &senders).Error
	if err != nil {
		return "", fmt.Errorf("provider_repository: load senders: %w", err)
	}
	if len(senders) == 0 {
		return "", nil
	}
	return "from:(" + strings.Join(senders, " OR ") + ")", nil
}

// ActiveProviders returns all providers that have at least one active email type.
// Used by the frontend to show which sources are supported.
func (r *ProviderRepository) ActiveProviders(ctx context.Context) ([]domain.Provider, error) {
	var providers []domain.Provider
	err := r.db.WithContext(ctx).
		Where(`code IN (
			SELECT DISTINCT provider_code
			FROM provider_email_types
			WHERE is_active = true
		)`).
		Find(&providers).Error
	return providers, err
}

// AllProviders returns every provider row (active and inactive).
// Used by the frontend to show "coming soon" providers.
func (r *ProviderRepository) AllProviders(ctx context.Context) ([]domain.Provider, error) {
	var providers []domain.Provider
	err := r.db.WithContext(ctx).Order("display_name").Find(&providers).Error
	return providers, err
}
