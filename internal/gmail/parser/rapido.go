package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RapidoParser handles Rapido ride receipt emails.
//
// Confirmed sender (from inbox screenshot): shoutout@rapido.bike
// Subject: always "Rapido Invoice"
//
// Two email formats exist:
//  1. Ride invoice  — "Ride Charge. ₹64.68. Booking Fees…"  (distance in body)
//  2. Payment summary — "Payment Summary. Ride ID. RD172…"  (distance in body)
//
// Cancellation emails also come from the same sender+subject — detected by
// "Cancellation Fee" in the snippet/body and returned as ErrCancellation.
//
// Snippet analysis: Rapido snippets show fare breakdown, NOT distance.
// TrySnippet always returns (nil, false) — full body is always needed.
//
// Vehicle type is in the body ("Bike", "Auto", "Cab") and determines which
// provider_email_types.code is assigned: rapido_bike / rapido_auto / rapido_cab.
type RapidoParser struct {
	reDistance *regexp.Regexp
	reDuration *regexp.Regexp
	reFare     *regexp.Regexp
	reVehicle  *regexp.Regexp
	reRideID   *regexp.Regexp
	rePickup   *regexp.Regexp // pickup address line
	reDrop     *regexp.Regexp // drop address line
}

func NewRapidoParser() *RapidoParser {
	return &RapidoParser{
		// "6.05 kms" | "6.05 km" | "6 km"
		reDistance: regexp.MustCompile(`(?i)([\d]+\.?[\d]*)\s*(?:km|kms|kilometre|kilometres)`),
		// "17.17 mins" | "17 min" | "1 hr 5 mins"
		reDuration: regexp.MustCompile(`(?i)(?:(\d+)\s*hr[s]?\s*)?(\d+(?:\.\d+)?)\s*min(?:s|utes)?`),
		// "₹68" | "INR 68" | "Rs. 68"
		reFare: regexp.MustCompile(`(?i)(?:₹|INR|Rs\.?)\s*([\d,]+\.?\d*)`),
		// "Bike" | "Auto" | "Cab" — word boundary to avoid partial matches
		reVehicle: regexp.MustCompile(`(?i)\b(Bike|Auto|Cab)\b`),
		// "RD17245133312223185"
		reRideID: regexp.MustCompile(`(?i)(RD\d{10,})`),
		// Rapido address formats:
		//   "Pickup Location\nKoramangala, Bengaluru"
		//   "Pickup: Koramangala, Bengaluru"
		rePickup: regexp.MustCompile(`(?im)^(?:pickup\s*(?:location)?|pick.?up)\s*:?\s*\n?([^\n:][^\n]+)`),
		reDrop:   regexp.MustCompile(`(?im)^(?:drop\s*(?:location)?|drop.?off|destination)\s*:?\s*\n?([^\n:][^\n]+)`),
	}
}

// TrySnippet always returns (nil, false) for Rapido.
// Rapido snippets show fare breakdown ("Ride Charge. ₹64.68…"), not distance.
// The full body is always required.
func (p *RapidoParser) TrySnippet(_, _ string) (*ParsedRide, bool) {
	return nil, false
}

// Parse extracts a ride from the full plain-text Rapido invoice email.
func (p *RapidoParser) Parse(subject, body string) (*ParsedRide, error) {
	// ── Cancellation guard ────────────────────────────────────────────────────
	if IsCancellation(subject, body) {
		return nil, ErrCancellation
	}

	// ── Distance (required) ───────────────────────────────────────────────────
	distMatch := p.reDistance.FindStringSubmatch(body)
	if distMatch == nil {
		return nil, fmt.Errorf("%w: rapido: distance not found", ErrUnrecognisedFormat)
	}
	dist, err := strconv.ParseFloat(strings.ReplaceAll(distMatch[1], ",", ""), 64)
	if err != nil || dist <= 0 {
		return nil, fmt.Errorf("%w: rapido: invalid distance %q", ErrUnrecognisedFormat, distMatch[1])
	}

	// ── Duration (optional) ───────────────────────────────────────────────────
	var durationMins *int
	if durMatch := p.reDuration.FindStringSubmatch(body); durMatch != nil {
		total := 0
		if durMatch[1] != "" {
			if h, err := strconv.Atoi(durMatch[1]); err == nil {
				total += h * 60
			}
		}
		// Rapido sometimes uses "17.17 mins" — treat as integer minutes
		if f, err := strconv.ParseFloat(durMatch[2], 64); err == nil {
			total += int(f)
		}
		if total > 0 {
			durationMins = &total
		}
	}

	// ── Vehicle type → resolves the provider_email_type code ─────────────────
	vehicleStr := p.reVehicle.FindString(body)
	vehicle, mode, code := p.resolveVehicle(vehicleStr)

	// ── Fare (optional) ───────────────────────────────────────────────────────
	var fareAmount *float64
	if fareMatch := p.reFare.FindStringSubmatch(body); fareMatch != nil {
		fareStr := strings.ReplaceAll(fareMatch[1], ",", "")
		if f, err := strconv.ParseFloat(fareStr, 64); err == nil {
			fareAmount = &f
		}
	}

	// ── Ride ID (optional, aids idempotency debugging) ────────────────────────
	rideID := ""
	if m := p.reRideID.FindString(body); m != "" {
		rideID = m
	}

	// ── Date ──────────────────────────────────────────────────────────────────
	startedAt := extractRapidoDate(body)

	// ── Metadata ─────────────────────────────────────────────────────────────
	meta := map[string]any{
		"provider": "rapido",
		"subject":  subject,
	}
	if vehicle != "" {
		meta["vehicle_type"] = vehicle
	}
	if fareAmount != nil {
		meta["fare_amount"] = *fareAmount
		meta["currency"] = "INR"
	}
	if rideID != "" {
		meta["ride_id"] = rideID
	}

	pickup := extractFirstLine(p.rePickup, body)
	drop := extractFirstLine(p.reDrop, body)

	return &ParsedRide{
		ProviderEmailTypeCode: code,
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

// resolveVehicle maps the detected vehicle string to:
//   - display name  ("Bike", "Auto", "Cab")
//   - transport_mode ("two_wheeler", "auto_rickshaw", "cab")
//   - provider_email_type code ("rapido_bike", "rapido_auto", "rapido_cab")
func (p *RapidoParser) resolveVehicle(raw string) (display, mode, code string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto":
		return "Auto", "auto_rickshaw", "rapido_auto"
	case "cab":
		return "Cab", "cab", "rapido_cab"
	default:
		// Default to bike — Rapido's primary product
		return "Bike", "two_wheeler", "rapido_bike"
	}
}

var rapidoDateLayouts = []string{
	"2 Jan 2006",
	"2 January 2006",
	"Jan 2, 2006",
	"January 2, 2006",
}

// extractRapidoDate finds dates in "21 Oct 2023" style.
func extractRapidoDate(text string) time.Time {
	re := regexp.MustCompile(`(?i)(\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9}\s+\d{4})`)
	m := re.FindString(text)
	if m == "" {
		return time.Time{}
	}
	// Strip ordinal suffixes ("21st" → "21")
	m = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)`).ReplaceAllString(m, "$1")
	for _, layout := range rapidoDateLayouts {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(m), time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}
