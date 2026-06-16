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
