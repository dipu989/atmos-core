// Package shortaddress resolves a lat/lng coordinate to a short, human-friendly
// display address (e.g. "Kaggadasapura, Bengaluru") via Google's Places API
// (New) shortFormattedAddress field. It degrades gracefully: when no API key
// is configured, every call returns ErrUnavailable and callers fall back to
// whatever long-form address they already have.
//
// No single Google endpoint maps a coordinate straight to shortFormattedAddress,
// so a resolve is two calls: reverse-geocode the coordinate to a place ID via
// the legacy Geocoding API, then fetch shortFormattedAddress for that place ID
// via Place Details (New). Results are cached by rounded coordinate so repeat
// trips to the same place (e.g. a daily commute) don't re-pay for both calls.
package shortaddress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/dipu/atmos-core/platform/logger"
	"go.uber.org/zap"
)

var ErrUnavailable = errors.New("shortaddress: unavailable")

// Resolver resolves a coordinate to a short display address.
type Resolver interface {
	Resolve(ctx context.Context, lat, lng float64) (string, error)
}

// New returns a real resolver when apiKey is non-empty, or a no-op that
// returns ErrUnavailable on every call.
func New(apiKey string, cache Cache) Resolver {
	if apiKey == "" {
		return &noop{}
	}
	return &googleResolver{
		apiKey:            apiKey,
		cache:             cache,
		client:            &http.Client{Timeout: 5 * time.Second},
		reverseGeocodeURL: reverseGeocodeURL,
		placeDetailsURL:   placeDetailsURL,
	}
}

// ── No-op implementation ─────────────────────────────────────────────────────

type noop struct{}

func (n *noop) Resolve(_ context.Context, _, _ float64) (string, error) {
	return "", ErrUnavailable
}

// ── Cache ────────────────────────────────────────────────────────────────────

// Cache stores reverse-geocode results keyed by rounded coordinate.
// roundCoord should be used by implementations and callers to derive keys
// consistently.
type Cache interface {
	Get(ctx context.Context, latRounded, lngRounded float64) (displayAddress string, ok bool, err error)
	Put(ctx context.Context, latRounded, lngRounded float64, placeID, displayAddress string) error
}

// roundCoord rounds a coordinate to 4 decimal places (~11m) — tight enough to
// distinguish nearby buildings, loose enough that repeat trips to the same
// place hit the cache.
func roundCoord(f float64) float64 {
	return math.Round(f*10000) / 10000
}

// ── Google Geocoding API (reverse) + Places API (New) Place Details ────────

const reverseGeocodeURL = "https://maps.googleapis.com/maps/api/geocode/json"
const placeDetailsURL = "https://places.googleapis.com/v1/places/"

type googleResolver struct {
	apiKey            string
	cache             Cache
	client            *http.Client
	reverseGeocodeURL string
	placeDetailsURL   string
}

func (g *googleResolver) Resolve(ctx context.Context, lat, lng float64) (string, error) {
	latR, lngR := roundCoord(lat), roundCoord(lng)

	display, ok, err := g.cache.Get(ctx, latR, lngR)
	if err != nil {
		// A broken cache backend degrades to a miss rather than failing the
		// resolve — the caller still gets a usable result via the API calls
		// below — but it's worth surfacing, since it silently doubles Google
		// API spend until whatever's wrong with the cache is fixed.
		logger.L().Warn("shortaddress: cache lookup failed, falling back to API", zap.Error(err))
	} else if ok {
		return display, nil
	}

	placeID, err := g.reverseGeocode(ctx, lat, lng)
	if err != nil {
		return "", err
	}
	display, err = g.placeDetails(ctx, placeID)
	if err != nil {
		return "", err
	}

	// Best-effort: a cache write failure should not fail the resolve — the
	// caller already has a usable result, and the next lookup just re-pays
	// for the API calls instead of hitting a corrupt cache.
	_ = g.cache.Put(ctx, latR, lngR, placeID, display)

	return display, nil
}

type reverseGeocodeResponse struct {
	Status  string `json:"status"`
	Results []struct {
		PlaceID string `json:"place_id"`
	} `json:"results"`
}

func (g *googleResolver) reverseGeocode(ctx context.Context, lat, lng float64) (string, error) {
	params := url.Values{}
	params.Set("latlng", fmt.Sprintf("%f,%f", lat, lng))
	params.Set("key", g.apiKey)
	reqURL := g.reverseGeocodeURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("shortaddress: build reverse geocode request: %w", err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("shortaddress: reverse geocode http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("shortaddress: reverse geocode http status %d", resp.StatusCode)
	}

	var body reverseGeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("shortaddress: decode reverse geocode: %w", err)
	}

	if body.Status != "OK" || len(body.Results) == 0 || body.Results[0].PlaceID == "" {
		return "", ErrUnavailable
	}
	return body.Results[0].PlaceID, nil
}

type placeDetailsResponse struct {
	ShortFormattedAddress string `json:"shortFormattedAddress"`
}

func (g *googleResolver) placeDetails(ctx context.Context, placeID string) (string, error) {
	reqURL := g.placeDetailsURL + url.PathEscape(placeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("shortaddress: build place details request: %w", err)
	}
	req.Header.Set("X-Goog-Api-Key", g.apiKey)
	req.Header.Set("X-Goog-FieldMask", "shortFormattedAddress")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("shortaddress: place details http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("shortaddress: place details http status %d", resp.StatusCode)
	}

	var body placeDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("shortaddress: decode place details: %w", err)
	}

	if body.ShortFormattedAddress == "" {
		return "", ErrUnavailable
	}
	return body.ShortFormattedAddress, nil
}
