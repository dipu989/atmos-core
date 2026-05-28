// Package parser provides email receipt parsers for ride-sharing services.
// Each parser implements the Parser interface and is registered in a Registry
// by the provider_email_types.code it handles.
//
// Design patterns:
//   - Strategy   : Parser interface = strategy; each file = concrete strategy
//   - Registry   : maps provider_email_types.code → Parser implementation
//   - Table-Driven: routing (which sender → which codes) lives in DB, not here
package parser

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// ErrUnrecognisedFormat is returned when a parser cannot find required fields.
var ErrUnrecognisedFormat = errors.New("email format not recognised")

// ErrCancellation is returned when the email is a cancellation — no activity
// should be created, but it is NOT a parse failure.
var ErrCancellation = errors.New("cancellation email — skip")

// ParsedRide is the normalised output every parser produces.
type ParsedRide struct {
	// ProviderEmailTypeCode is the resolved code, e.g. "rapido_bike" or "rapido_auto".
	// The parser sets this after it determines the vehicle type from the body.
	ProviderEmailTypeCode string

	TransportMode   string  // matches activities.transport_mode enum
	VehicleType     *string // "Uber Go", "Bike", "Auto" — stored in metadata
	DistanceKM      float64
	DurationMinutes *int
	StartedAt       time.Time // zero → caller falls back to email Date: header
	PickupAddress   string
	DropAddress     string
	FareAmount      *float64
	Currency        string
	Metadata        map[string]any // stored as raw_metadata on the activity
}

// Parser is implemented by each provider-specific file.
// Routing (which sender maps to which code) is the DB's job — parsers only parse.
type Parser interface {
	// TrySnippet attempts a quick parse from the ~100-char Gmail snippet.
	// Returns (result, true) if the snippet contained enough data to create
	// a complete ParsedRide without fetching the full body.
	// Returns (nil, false) if the full body is needed.
	TrySnippet(subject, snippet string) (*ParsedRide, bool)

	// Parse extracts a ride from the full plain-text email body.
	// Returns ErrCancellation if the email is a cancellation.
	// Returns ErrUnrecognisedFormat if required fields are missing.
	Parse(subject, body string) (*ParsedRide, error)
}

// Registry maps provider_email_types.code → Parser implementation.
// The DB controls which codes are active and how they are routed;
// the Registry controls which Go implementation handles each code.
type Registry struct {
	parsers map[string]Parser
}

func NewRegistry() *Registry {
	r := &Registry{parsers: make(map[string]Parser)}
	// Register all built-in parsers.
	// Adding a new platform = new file + one Register call here.
	uberParser := NewUberParser()
	r.Register("uber_ride", uberParser)

	rapidoParser := NewRapidoParser()
	r.Register("rapido_bike", rapidoParser)
	r.Register("rapido_auto", rapidoParser)
	r.Register("rapido_cab", rapidoParser)

	return r
}

// Register associates a provider_email_types.code with a Parser.
func (r *Registry) Register(code string, p Parser) {
	r.parsers[code] = p
}

// Get returns the Parser for a given code, or (nil, false) if none registered.
func (r *Registry) Get(code string) (Parser, bool) {
	p, ok := r.parsers[code]
	return p, ok
}

// ── shared helpers ────────────────────────────────────────────────────────────

var reMultiSpace = regexp.MustCompile(`\s+`)

// normalise collapses whitespace and lowercases.
func normalise(s string) string {
	return strings.TrimSpace(reMultiSpace.ReplaceAllString(strings.ToLower(s), " "))
}

// isCancellation returns true if the subject or snippet indicates a ride
// cancellation — common across all ride providers.
func IsCancellation(subject, snippet string) bool {
	lower := strings.ToLower(subject + " " + snippet)
	return strings.Contains(lower, "cancellation") ||
		strings.Contains(lower, "cancelled") ||
		strings.Contains(lower, "cancellation fee")
}
