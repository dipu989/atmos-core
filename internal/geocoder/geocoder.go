// Package geocoder provides address-to-coordinate resolution via the Google
// Maps Geocoding API. It degrades gracefully: when no API key is configured,
// all calls return ErrNoAPIKey and callers proceed without coordinates.
package geocoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var ErrNoAPIKey = errors.New("geocoder: no Google Maps API key configured")
var ErrNoResults = errors.New("geocoder: no results for address")

// Geocoder resolves a plain-text address to a lat/lng coordinate pair.
type Geocoder interface {
	Geocode(ctx context.Context, address string) (lat, lng float64, err error)
}

// New returns a real geocoder when apiKey is non-empty, or a no-op that
// returns ErrNoAPIKey on every call.
func New(apiKey string) Geocoder {
	if apiKey == "" {
		return &noop{}
	}
	return &googleGeocoder{
		apiKey: apiKey,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// ── No-op implementation ─────────────────────────────────────────────────────

type noop struct{}

func (n *noop) Geocode(_ context.Context, _ string) (float64, float64, error) {
	return 0, 0, ErrNoAPIKey
}

// ── Google Maps Geocoding API ─────────────────────────────────────────────────

const geocodeURL = "https://maps.googleapis.com/maps/api/geocode/json"

type googleGeocoder struct {
	apiKey string
	client *http.Client
}

type geocodeResponse struct {
	Status  string `json:"status"`
	Results []struct {
		Geometry struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
}

func (g *googleGeocoder) Geocode(ctx context.Context, address string) (lat, lng float64, err error) {
	if address == "" {
		return 0, 0, ErrNoResults
	}

	reqURL := fmt.Sprintf("%s?address=%s&key=%s",
		geocodeURL,
		url.QueryEscape(address),
		g.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("geocoder: build request: %w", err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("geocoder: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, 0, fmt.Errorf("geocoder: http status %d", resp.StatusCode)
	}

	var body geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, 0, fmt.Errorf("geocoder: decode: %w", err)
	}

	if body.Status != "OK" || len(body.Results) == 0 {
		return 0, 0, ErrNoResults
	}

	loc := body.Results[0].Geometry.Location
	return loc.Lat, loc.Lng, nil
}
