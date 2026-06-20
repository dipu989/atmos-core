package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicAPIURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	llmMaxRetries    = 3
)

var llmSystemPrompt = `You are a ride-receipt email parser. Extract ride information from the email and return ONLY a JSON object.

Fields to extract (use null for unknown values):
{
  "pickup_address": "full pickup address string or null",
  "drop_address": "full destination address string or null",
  "distance_km": 5.2,
  "duration_minutes": 18,
  "fare_amount": 120.0,
  "currency": "INR",
  "vehicle_type": "Bike|Auto|Cab|UberGo|RapidoBike or null",
  "provider": "uber|rapido|ola|etc",
  "transport_mode": "two_wheeler|cab|auto_rickshaw",
  "started_at": "2024-01-15T10:30:00Z or null"
}

Rules:
- transport_mode must be one of: two_wheeler, cab, auto_rickshaw
- currency defaults to "INR" for Indian receipts
- started_at must be RFC3339 format or null
- If this is NOT a ride receipt, return: {"not_a_receipt": true}
- Output ONLY the JSON object. No markdown, no explanation.`

// LLMParser calls the Anthropic API to extract ride details from email bodies.
// It satisfies the Parser interface; TrySnippet always returns false because
// the LLM needs the full body to be useful.
type LLMParser struct {
	apiKey    string
	model     string
	semaphore chan struct{}
	client    *http.Client
}

// NewLLMParser creates an LLMParser. maxConcurrent caps parallel API calls.
func NewLLMParser(apiKey, model string, maxConcurrent int) *LLMParser {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &LLMParser{
		apiKey:    apiKey,
		model:     model,
		semaphore: make(chan struct{}, maxConcurrent),
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// IsEnabled reports whether an API key is configured.
func (p *LLMParser) IsEnabled() bool { return p.apiKey != "" }

// TrySnippet always returns false — LLM parsing requires the full body.
func (p *LLMParser) TrySnippet(_, _ string) (*ParsedRide, bool) { return nil, false }

// Parse satisfies the Parser interface using context.Background internally.
func (p *LLMParser) Parse(subject, body string) (*ParsedRide, error) {
	return p.ParseWithContext(context.Background(), subject, body)
}

// ParseWithContext is like Parse but respects the provided context for
// cancellation and timeout during semaphore acquisition and HTTP calls.
func (p *LLMParser) ParseWithContext(ctx context.Context, subject, body string) (*ParsedRide, error) {
	if !p.IsEnabled() {
		return nil, ErrUnrecognisedFormat
	}

	// Acquire semaphore slot (respects context cancellation).
	select {
	case p.semaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.semaphore }()

	return p.callWithRetry(ctx, subject, body)
}

// ── internal ────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type llmRideJSON struct {
	NotAReceipt     bool     `json:"not_a_receipt"`
	PickupAddress   *string  `json:"pickup_address"`
	DropAddress     *string  `json:"drop_address"`
	DistanceKM      *float64 `json:"distance_km"`
	DurationMinutes *int     `json:"duration_minutes"`
	FareAmount      *float64 `json:"fare_amount"`
	Currency        string   `json:"currency"`
	VehicleType     *string  `json:"vehicle_type"`
	Provider        string   `json:"provider"`
	TransportMode   string   `json:"transport_mode"`
	StartedAt       *string  `json:"started_at"`
}

func (p *LLMParser) callWithRetry(ctx context.Context, subject, body string) (*ParsedRide, error) {
	delay := 2 * time.Second
	for attempt := 0; attempt < llmMaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ride, retryable, err := p.call(ctx, subject, body)
		if err == nil {
			return ride, nil
		}
		if !retryable || attempt == llmMaxRetries-1 {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return nil, ErrUnrecognisedFormat // unreachable: loop always returns on last attempt
}

// call makes one Anthropic API request. Returns (ride, retryable, error).
func (p *LLMParser) call(ctx context.Context, subject, body string) (*ParsedRide, bool, error) {
	content := fmt.Sprintf("Subject: %s\n\n%s", subject, llmTruncate(body, 8000))

	payload := anthropicRequest{
		Model:     p.model,
		MaxTokens: 512,
		System:    llmSystemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: content}},
	}
	reqBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, true, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("anthropic %d: %s", resp.StatusCode, llmTruncate(string(body), 200))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read response: %w", err)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil || len(apiResp.Content) == 0 {
		return nil, false, ErrUnrecognisedFormat
	}

	text := llmStripFence(strings.TrimSpace(apiResp.Content[0].Text))

	var data llmRideJSON
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, false, fmt.Errorf("parse llm json: %w", err)
	}
	if data.NotAReceipt {
		return nil, false, ErrUnrecognisedFormat
	}

	ride := &ParsedRide{
		TransportMode:   llmNormaliseMode(data.TransportMode),
		VehicleType:     data.VehicleType,
		DurationMinutes: data.DurationMinutes,
		FareAmount:      data.FareAmount,
		Currency:        data.Currency,
		Metadata: map[string]any{
			"provider": data.Provider,
			"source":   "llm",
		},
	}
	if data.DistanceKM != nil {
		ride.DistanceKM = *data.DistanceKM
	}
	if data.PickupAddress != nil {
		ride.PickupAddress = *data.PickupAddress
	}
	if data.DropAddress != nil {
		ride.DropAddress = *data.DropAddress
	}
	if data.VehicleType != nil {
		ride.Metadata["vehicle_type"] = *data.VehicleType
	}
	if data.StartedAt != nil {
		if t, err := time.Parse(time.RFC3339, *data.StartedAt); err == nil {
			ride.StartedAt = t.UTC()
		}
	}

	return ride, false, nil
}

func llmNormaliseMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "bike", "motorcycle", "two_wheeler":
		return "two_wheeler"
	case "auto", "auto_rickshaw", "autorickshaw", "rickshaw":
		return "auto_rickshaw"
	case "car", "cab", "taxi":
		return "cab"
	default:
		if mode == "" {
			return "cab"
		}
		return strings.ToLower(mode)
	}
}

func llmTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func llmStripFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
