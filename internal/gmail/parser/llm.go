package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const llmMaxRetries = 3

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

// LLMParser invokes the `claude` CLI as a subprocess to extract ride details
// from email bodies. Uses the claude login session already present on the host —
// no separate API key or credits required.
// It satisfies the Parser interface; TrySnippet always returns false because
// the CLI needs the full body to be useful.
type LLMParser struct {
	bin       string // path to claude binary (default: "claude")
	semaphore chan struct{}
}

// NewLLMParser creates an LLMParser. bin is the path to the claude binary
// (empty string defaults to "claude" on PATH). maxConcurrent caps parallel
// CLI invocations.
func NewLLMParser(bin string, maxConcurrent int) *LLMParser {
	if bin == "" {
		bin = "claude"
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &LLMParser{
		bin:       bin,
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// IsEnabled reports whether the claude binary is available on PATH.
func (p *LLMParser) IsEnabled() bool {
	_, err := exec.LookPath(p.bin)
	return err == nil
}

// TrySnippet always returns false — CLI parsing requires the full body.
func (p *LLMParser) TrySnippet(_, _ string) (*ParsedRide, bool) { return nil, false }

// Parse satisfies the Parser interface using context.Background internally.
func (p *LLMParser) Parse(subject, body string) (*ParsedRide, error) {
	return p.ParseWithContext(context.Background(), subject, body)
}

// ParseWithContext is like Parse but respects the provided context for
// cancellation and timeout during semaphore acquisition and CLI execution.
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

// invoke runs the claude CLI once and parses its JSON output.
func (p *LLMParser) invoke(ctx context.Context, subject, body string) (*ParsedRide, error) {
	prompt := fmt.Sprintf("%s\n\nEmail to parse:\nSubject: %s\n\n%s",
		llmSystemPrompt, subject, llmTruncate(body, 8000))

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.bin, "-p", prompt, "--output-format", "text")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude cli: %w (stderr: %s)", err, llmTruncate(stderr.String(), 200))
	}

	text := llmStripFence(strings.TrimSpace(stdout.String()))

	var data llmRideJSON
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("parse cli json: %w (output: %s)", err, llmTruncate(text, 200))
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
