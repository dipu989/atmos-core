// Package places provides a proxy for the Google Maps Places Text Search API so
// the mobile client can search for place names and receive coordinates without
// embedding the Maps API key in the binary.
package places

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

// PlaceResult is one geocoding match returned to the client.
type PlaceResult struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Handler proxies place-search requests to the Google Maps Places Text Search API.
type Handler struct {
	apiKey string
	client *http.Client
}

func NewHandler(apiKey string) *Handler {
	return &Handler{
		apiKey: apiKey,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Autocomplete godoc
// @Summary     Place autocomplete
// @Description Returns up to 5 place suggestions matching the query string.
// @Description Backed by the Google Maps Places Text Search API; degrades to an
// @Description empty list when no Maps API key is configured.
// @Tags        places
// @Produce     json
// @Security    BearerAuth
// @Param       q  query    string  true  "Partial place name or address"
// @Success     200 {array} PlaceResult
// @Failure     400 {object} map[string]interface{}
// @Router      /places/autocomplete [get]
func (h *Handler) Autocomplete(c *fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		return response.BadRequest(c, "q is required")
	}
	if h.apiKey == "" {
		return response.OK(c, []PlaceResult{})
	}

	places, err := h.search(c.Context(), q)
	if err != nil {
		// Degrade gracefully — the caller shows an empty suggestion list.
		return response.OK(c, []PlaceResult{})
	}
	return response.OK(c, places)
}

const placesTextSearchURL = "https://maps.googleapis.com/maps/api/place/textsearch/json"

func (h *Handler) search(ctx context.Context, query string) ([]PlaceResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("region", "in")
	params.Set("key", h.apiKey)
	reqURL := placesTextSearchURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("places: build request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("places: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("places: http status %d", resp.StatusCode)
	}

	var body struct {
		Status  string `json:"status"`
		Results []struct {
			FormattedAddress string `json:"formatted_address"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("places: decode: %w", err)
	}

	if body.Status != "OK" || len(body.Results) == 0 {
		return []PlaceResult{}, nil
	}

	max := 5
	if len(body.Results) < max {
		max = len(body.Results)
	}
	out := make([]PlaceResult, max)
	for i := range out {
		r := body.Results[i]
		out[i] = PlaceResult{
			Name: r.FormattedAddress,
			Lat:  r.Geometry.Location.Lat,
			Lng:  r.Geometry.Location.Lng,
		}
	}
	return out, nil
}

// DistanceResult is the route distance returned to the client for a given
// origin/destination/mode. Found is false when Google has no route for the
// requested mode (e.g. no transit line between the two points) — the client
// falls back to manual entry rather than treating it as an error.
type DistanceResult struct {
	DistanceKm float64 `json:"distanceKm"`
	Found      bool    `json:"found"`
}

var validDistanceModes = map[string]bool{
	"driving":   true,
	"walking":   true,
	"bicycling": true,
	"transit":   true,
}

// Distance godoc
// @Summary     Route distance
// @Description Returns the route distance in km between two coordinates for a given
// @Description travel mode. Backed by the Google Maps Distance Matrix API.
// @Tags        places
// @Produce     json
// @Security    BearerAuth
// @Param       originLat  query  number  true  "Origin latitude"
// @Param       originLng  query  number  true  "Origin longitude"
// @Param       destLat    query  number  true  "Destination latitude"
// @Param       destLng    query  number  true  "Destination longitude"
// @Param       mode       query  string  true  "One of driving, walking, bicycling, transit"
// @Success     200 {object} DistanceResult
// @Failure     400 {object} map[string]interface{}
// @Router      /places/distance [get]
func (h *Handler) Distance(c *fiber.Ctx) error {
	originLat, err := strconv.ParseFloat(c.Query("originLat"), 64)
	if err != nil {
		return response.BadRequest(c, "originLat is required and must be a number")
	}
	originLng, err := strconv.ParseFloat(c.Query("originLng"), 64)
	if err != nil {
		return response.BadRequest(c, "originLng is required and must be a number")
	}
	destLat, err := strconv.ParseFloat(c.Query("destLat"), 64)
	if err != nil {
		return response.BadRequest(c, "destLat is required and must be a number")
	}
	destLng, err := strconv.ParseFloat(c.Query("destLng"), 64)
	if err != nil {
		return response.BadRequest(c, "destLng is required and must be a number")
	}
	mode := c.Query("mode")
	if !validDistanceModes[mode] {
		return response.BadRequest(c, "mode must be one of driving, walking, bicycling, transit")
	}
	if h.apiKey == "" {
		return response.BadRequest(c, "distance lookup is not configured")
	}

	result, err := h.distance(c.Context(), originLat, originLng, destLat, destLng, mode)
	if err != nil {
		// Degrade gracefully — the caller falls back to manual entry.
		return response.OK(c, DistanceResult{Found: false})
	}
	return response.OK(c, result)
}

const distanceMatrixURL = "https://maps.googleapis.com/maps/api/distancematrix/json"

func (h *Handler) distance(ctx context.Context, originLat, originLng, destLat, destLng float64, mode string) (DistanceResult, error) {
	params := url.Values{}
	params.Set("origins", fmt.Sprintf("%f,%f", originLat, originLng))
	params.Set("destinations", fmt.Sprintf("%f,%f", destLat, destLng))
	params.Set("mode", mode)
	params.Set("region", "in")
	params.Set("key", h.apiKey)
	reqURL := distanceMatrixURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return DistanceResult{}, fmt.Errorf("distance: build request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return DistanceResult{}, fmt.Errorf("distance: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return DistanceResult{}, fmt.Errorf("distance: http status %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
		Rows   []struct {
			Elements []struct {
				Status   string `json:"status"`
				Distance struct {
					Value float64 `json:"value"` // meters
				} `json:"distance"`
			} `json:"elements"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return DistanceResult{}, fmt.Errorf("distance: decode: %w", err)
	}

	if body.Status != "OK" || len(body.Rows) == 0 || len(body.Rows[0].Elements) == 0 {
		return DistanceResult{Found: false}, nil
	}
	element := body.Rows[0].Elements[0]
	if element.Status != "OK" {
		return DistanceResult{Found: false}, nil
	}

	return DistanceResult{DistanceKm: element.Distance.Value / 1000.0, Found: true}, nil
}
