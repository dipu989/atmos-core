package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const llmMaxRetries = 3
const groqEndpoint = "https://api.groq.com/openai/v1/chat/completions"

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

// LLMParser calls the Groq API to extract ride details from email bodies.
// It satisfies the Parser interface; TrySnippet always returns false because
// the full body is needed.
type LLMParser struct {
	apiKey     string
	model      string
	httpClient *http.Client
	semaphore  chan struct{}
}

// NewLLMParser creates an LLMParser backed by the Groq API.
// apiKey is the GROQ_API_KEY value. model defaults to "llama-3.1-8b-instant".
// maxConcurrent caps parallel in-flight HTTP requests.
func NewLLMParser(apiKey, model string, maxConcurrent int) *LLMParser {
	if model == "" {
		model = "llama-3.1-8b-instant"
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &LLMParser{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		semaphore:  make(chan struct{}, maxConcurrent),
	}
}

// IsEnabled reports whether a Groq API key is configured.
func (p *LLMParser) IsEnabled() bool {
	return p.apiKey != ""
}

// TrySnippet always returns false — Groq parsing requires the full body.
func (p *LLMParser) TrySnippet(_, _ string) (*ParsedRide, bool) { return nil, false }

// Parse satisfies the Parser interface using context.Background internally.
func (p *LLMParser) Parse(subject, body string) (*ParsedRide, error) {
	return p.ParseWithContext(context.Background(), subject, body)
}

// ParseWithContext is like Parse but respects the provided context for
// cancellation and timeout during semaphore acquisition and HTTP execution.
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

	return p.invokeWithRetry(ctx, subject, body)
}

// ── internal ────────────────────────────────────────────────────────────────

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

// groqFatalError wraps a non-retryable Groq API response (4xx except 429).
type groqFatalError struct{ err error }

func (e *groqFatalError) Error() string { return e.err.Error() }
func (e *groqFatalError) Unwrap() error { return e.err }

func (p *LLMParser) invokeWithRetry(ctx context.Context, subject, body string) (*ParsedRide, error) {
	delay := 2 * time.Second
	for attempt := 0; attempt < llmMaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ride, err := p.invoke(ctx, subject, body)
		if err == nil {
			return ride, nil
		}
		// Don't retry definitive rejections or non-retryable HTTP errors.
		if errors.Is(err, ErrUnrecognisedFormat) {
			return nil, err
		}
		var fatal *groqFatalError
		if errors.As(err, &fatal) {
			return nil, err
		}
		if attempt == llmMaxRetries-1 {
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

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model       string        `json:"model"`
	Temperature int           `json:"temperature"`
	Messages    []groqMessage `json:"messages"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// invoke calls the Groq API once and parses its JSON output.
func (p *LLMParser) invoke(ctx context.Context, subject, body string) (*ParsedRide, error) {
	userContent := fmt.Sprintf("Email to parse:\nSubject: %s\n\n%s",
		subject, llmTruncate(body, 2000))

	reqBody := groqRequest{
		Model:       p.model,
		Temperature: 0,
		Messages: []groqMessage{
			{Role: "system", Content: llmSystemPrompt},
			{Role: "user", Content: userContent},
		},
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("groq: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqEndpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("groq: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groq: http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("groq: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		wrapped := fmt.Errorf("groq: status %d: %s", resp.StatusCode, llmTruncate(string(respBytes), 200))
		// 4xx errors other than 429 (Too Many Requests) are not transient — don't retry.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, &groqFatalError{err: wrapped}
		}
		return nil, wrapped
	}

	var groqResp groqResponse
	if err := json.Unmarshal(respBytes, &groqResp); err != nil {
		return nil, fmt.Errorf("groq: parse response: %w", err)
	}
	if len(groqResp.Choices) == 0 {
		return nil, fmt.Errorf("groq: empty choices in response")
	}

	text := llmStripFence(strings.TrimSpace(groqResp.Choices[0].Message.Content))

	var data llmRideJSON
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("groq: parse llm json: %w (output: %s)", err, llmTruncate(text, 200))
	}
	if data.NotAReceipt {
		return nil, ErrUnrecognisedFormat
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

	return ride, nil
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
