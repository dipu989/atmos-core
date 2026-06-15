package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UberParser handles Uber ride receipt emails.
//
// Confirmed sender (from inbox screenshot): noreply@uber.com
// Subject pattern: "[Personal] Your {weekday} {time-of-day} trip with Uber"
//
// Snippet format (observed): "Uber Go. 4.47 kilometers | 9 min. 11:59 PM. 847, Indira ..."
// The snippet alone contains vehicle type, distance, and duration — enough
// to build a ParsedRide without fetching the full body.
type UberParser struct {
	// Snippet patterns
	reSnippetDistance *regexp.Regexp // "4.47 kilometers"
	reSnippetDuration *regexp.Regexp // "9 min"
	reSnippetVehicle  *regexp.Regexp // "Uber Go" at start of snippet

	// Full-body patterns (fallback)
	reDistance *regexp.Regexp
	reDuration *regexp.Regexp
	reVehicle  *regexp.Regexp
	reFare     *regexp.Regexp
	rePickup   *regexp.Regexp // address line after "Pickup" / "Pick-up"
	reDrop     *regexp.Regexp // address line after "Drop-off" / "Destination"
}

func NewUberParser() *UberParser {
	return &UberParser{
		// Snippet-specific: tight patterns matching the observed format
		reSnippetDistance: regexp.MustCompile(`([\d]+\.?[\d]*)\s*(?:km|kms|kilometre|kilometres|kilometer|kilometers)`),
		reSnippetDuration: regexp.MustCompile(`(\d+)\s*min`),
		reSnippetVehicle:  regexp.MustCompile(`^(Uber\s*(?:Go|XL|Auto|Moto|Premier|Pool|Comfort|Black|SUV|Intercity)?)`),

		// Full-body fallbacks
		reDistance: regexp.MustCompile(`(?i)([\d]+\.?[\d]*)\s*(?:km|kms|kilometre|kilometres|kilometer|kilometers)`),
		reDuration: regexp.MustCompile(`(?i)(?:(\d+)\s*hr[s]?\s*)?(\d+)\s*min(?:s|utes)?`),
		reVehicle:  regexp.MustCompile(`(?i)(Uber\s*(?:Go|XL|Auto|Moto|Premier|Pool|Comfort|Black|SUV|Intercity)?)`),
		reFare:     regexp.MustCompile(`(?i)(?:₹|INR|Rs\.?)\s*([\d,]+\.?\d*)`),
		// Address extraction: capture the first non-empty line after the label.
		// Uber plain-text format:
		//   Pickup\n847, Indira Nagar, Bengaluru\n\nDrop-off\nWhitefield, Bengaluru
		rePickup: regexp.MustCompile(`(?im)^(?:pickup|pick-up)\s*:?\s*\n([^\n]+)`),
		reDrop:   regexp.MustCompile(`(?im)^(?:drop.?off|destination|drop)\s*:?\s*\n([^\n]+)`),
	}
}

// TrySnippet parses the Uber receipt from the Gmail snippet alone.
// The observed snippet format is:
//
//	"Uber Go. 4.47 kilometers | 9 min. 11:59 PM. 847, Indira ..."
//
// Returns (result, true) when distance was found — that is the only required
// field. Date falls back to the email Date: header in the caller.
func (p *UberParser) TrySnippet(subject, snippet string) (*ParsedRide, bool) {
	if snippet == "" {
		return nil, false
	}

	distMatch := p.reSnippetDistance.FindStringSubmatch(snippet)
	if distMatch == nil {
		return nil, false // snippet doesn't have distance — need full body
	}

	dist, err := strconv.ParseFloat(strings.ReplaceAll(distMatch[1], ",", ""), 64)
	if err != nil || dist <= 0 {
		return nil, false
	}

	// Duration
	var durationMins *int
	if durMatch := p.reSnippetDuration.FindStringSubmatch(snippet); durMatch != nil {
		if m, err := strconv.Atoi(durMatch[1]); err == nil && m > 0 {
			durationMins = &m
		}
	}

	// Vehicle type — Uber snippets start with the vehicle name
	vehicle, mode := p.resolveVehicle(p.reSnippetVehicle.FindString(snippet))

	meta := p.buildMeta(subject, vehicle, nil)

	return &ParsedRide{
		ProviderEmailTypeCode: "uber_ride",
		TransportMode:         mode,
		VehicleType:           vehiclePtr(vehicle),
		DistanceKM:            dist,
		DurationMinutes:       durationMins,
		Metadata:              meta,
	}, true
}

// Parse extracts a ride from the full plain-text email body.
func (p *UberParser) Parse(subject, body string) (*ParsedRide, error) {
	if IsCancellation(subject, body) {
		return nil, ErrCancellation
	}

	distMatch := p.reDistance.FindStringSubmatch(body)
	if distMatch == nil {
		return nil, fmt.Errorf("%w: uber: distance not found", ErrUnrecognisedFormat)
	}
	dist, err := strconv.ParseFloat(strings.ReplaceAll(distMatch[1], ",", ""), 64)
	if err != nil || dist <= 0 {
		return nil, fmt.Errorf("%w: uber: invalid distance %q", ErrUnrecognisedFormat, distMatch[1])
	}

	var durationMins *int
	if durMatch := p.reDuration.FindStringSubmatch(body); durMatch != nil {
		mins := 0
		if durMatch[1] != "" {
			if h, err := strconv.Atoi(durMatch[1]); err == nil {
				mins += h * 60
			}
		}
		if m, err := strconv.Atoi(durMatch[2]); err == nil {
			mins += m
		}
		if mins > 0 {
			durationMins = &mins
		}
	}

	vehicleStr := p.reVehicle.FindString(body)
	vehicle, mode := p.resolveVehicle(vehicleStr)

	var fareAmount *float64
	if fareMatch := p.reFare.FindStringSubmatch(body); fareMatch != nil {
		fareStr := strings.ReplaceAll(fareMatch[1], ",", "")
		if f, err := strconv.ParseFloat(fareStr, 64); err == nil {
			fareAmount = &f
		}
	}

	startedAt := extractDateFromText(body)
	meta := p.buildMeta(subject, vehicle, fareAmount)

	pickup := extractFirstLine(p.rePickup, body)
	drop := extractFirstLine(p.reDrop, body)

	return &ParsedRide{
		ProviderEmailTypeCode: "uber_ride",
		TransportMode:         mode,
		VehicleType:           vehiclePtr(vehicle),
		DistanceKM:            dist,
		DurationMinutes:       durationMins,
		StartedAt:             startedAt,
		PickupAddress:         pickup,
		DropAddress:           drop,
		FareAmount:            fareAmount,
		Currency:              "INR",
		Metadata:              meta,
	}, nil
}


func (p *UberParser) resolveVehicle(raw string) (display, mode string) {
	display = strings.TrimSpace(raw)
	lower := strings.ToLower(display)
	switch {
	case strings.Contains(lower, "auto"):
		mode = "auto_rickshaw"
	case strings.Contains(lower, "moto"):
		mode = "two_wheeler"
	default:
		mode = "cab"
	}
	return
}

func (p *UberParser) buildMeta(subject, vehicle string, fare *float64) map[string]any {
	meta := map[string]any{
		"provider": "uber",
		"subject":  subject,
	}
	if vehicle != "" {
		meta["vehicle_type"] = vehicle
	}
	if fare != nil {
		meta["fare_amount"] = *fare
		meta["currency"] = "INR"
	}
	return meta
}

// ── date extraction helpers ───────────────────────────────────────────────────

var uberDateLayouts = []string{
	"Monday, January 2, 2006",
	"Monday, Jan 2, 2006",
	"January 2, 2006",
	"Jan 2, 2006",
}

var reDateCandidate = regexp.MustCompile(`(?i)\d{1,2}\s+[A-Za-z]{3,9}\s+\d{4}|[A-Za-z]{3,9}\s+\d{1,2},?\s+\d{4}`)

func extractDateFromText(text string) time.Time {
	m := reDateCandidate.FindString(text)
	if m == "" {
		return time.Time{}
	}
	for _, layout := range uberDateLayouts {
		if t, err := time.ParseInLocation(layout, m, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}

func vehiclePtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
