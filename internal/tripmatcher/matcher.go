// Package tripmatcher scores the likelihood that two trip records describe the
// same physical journey. It is intentionally a pure-function package with no
// database, HTTP, or I/O dependencies so it can be unit-tested exhaustively.
//
// Typical callers:
//   - Receipt ingest pipeline (Task 4): find a GPS activity that matches the
//     incoming receipt before creating a duplicate.
//   - GPS confirm endpoint (Task 5): find a receipt activity that matches the
//     just-confirmed GPS trip before creating a duplicate.
package tripmatcher

import (
	"math"
	"time"
)

// ------------------- public types -------------------

// TripCandidate is the minimal set of fields the scorer needs. Callers project
// their domain.Activity (or an in-progress GPS session) into this struct.
type TripCandidate struct {
	StartedAt       time.Time
	EndedAt         *time.Time // nil means the trip is still in progress
	OriginLat       *float64
	OriginLng       *float64
	DestLat         *float64
	DestLng         *float64
	TransportMode   string // empty string means unknown — mode check is skipped
	DurationMinutes *int   // nil means unknown — duration signal is skipped
}

// SignalScores holds the per-signal contributions before weighting, useful for
// debugging and for the "review" notification body.
type SignalScores struct {
	TimeOverlap    float64 // [0, 1]
	DestDistance   float64 // [0, 1]
	OriginDistance float64 // [0, 1]
	DurationDelta  float64 // [0, 1]
}

// MatchResult is the output of Score(). Confidence is the weighted sum of the
// four signal scores. HasCoords is false when at least one candidate lacked
// destination coordinates — callers should apply a stricter merge threshold
// (0.75 instead of 0.65) in that case.
type MatchResult struct {
	Confidence float64
	Signals    SignalScores
	HasCoords  bool
}

// Confidence thresholds — exported so callers can apply consistent rules.
const (
	ThresholdAutoMerge       = 0.65 // ≥ this → auto-merge into one activity
	ThresholdAutoMergeNoCoord = 0.75 // ≥ this → auto-merge when coords unavailable
	ThresholdReview          = 0.45 // ≥ this but < AutoMerge → flag for user review
)

// ------------------- scorer -------------------

// Score computes the match confidence between two trip candidates.
// Returns MatchResult with Confidence in [0, 1].
// A Confidence of 0 means the trips are definitively different (hard-fail gate).
func Score(a, b TripCandidate) MatchResult {
	// Hard gate 1: mode incompatibility. If both modes are known and
	// incompatible, these cannot be the same trip.
	if a.TransportMode != "" && b.TransportMode != "" {
		if !modesCompatible(a.TransportMode, b.TransportMode) {
			return MatchResult{}
		}
	}

	// Hard gate 2: zero time overlap. Trips on different time windows cannot
	// be duplicates regardless of other signals.
	aEnd := effectiveEnd(a)
	bEnd := effectiveEnd(b)
	overlapScore, overlapRatio := timeOverlapScore(a.StartedAt, aEnd, b.StartedAt, bEnd)
	if overlapRatio == 0 {
		return MatchResult{}
	}

	destScore, hasDestCoords := destinationScore(a, b)
	originScore := originScore(a, b)
	durScore := durationScore(a, b)

	signals := SignalScores{
		TimeOverlap:    overlapScore,
		DestDistance:   destScore,
		OriginDistance: originScore,
		DurationDelta:  durScore,
	}

	confidence := overlapScore*0.40 + destScore*0.30 + originScore*0.20 + durScore*0.10

	return MatchResult{
		Confidence: math.Round(confidence*1000) / 1000,
		Signals:    signals,
		HasCoords:  hasDestCoords,
	}
}

// ------------------- signal scorers -------------------

// effectiveEnd returns the EndedAt time, or StartedAt + DurationMinutes if
// EndedAt is nil. If neither is available, falls back to StartedAt + 60 min
// as a conservative estimate.
func effectiveEnd(c TripCandidate) time.Time {
	if c.EndedAt != nil {
		return *c.EndedAt
	}
	if c.DurationMinutes != nil {
		return c.StartedAt.Add(time.Duration(*c.DurationMinutes) * time.Minute)
	}
	return c.StartedAt.Add(60 * time.Minute)
}

// timeOverlapScore returns a score in [0, 1] and the raw overlap ratio.
// Overlap is measured relative to the shorter trip's duration so that a
// complete short trip inside a longer trip scores 1.0 on this signal.
func timeOverlapScore(aStart, aEnd, bStart, bEnd time.Time) (score, ratio float64) {
	overlapStart := aStart
	if bStart.After(overlapStart) {
		overlapStart = bStart
	}
	overlapEnd := aEnd
	if bEnd.Before(overlapEnd) {
		overlapEnd = bEnd
	}

	if !overlapEnd.After(overlapStart) {
		return 0, 0
	}

	overlapSecs := overlapEnd.Sub(overlapStart).Seconds()
	aDur := aEnd.Sub(aStart).Seconds()
	bDur := bEnd.Sub(bStart).Seconds()
	shorter := aDur
	if bDur < shorter {
		shorter = bDur
	}
	if shorter <= 0 {
		return 0, 0
	}
	ratio = overlapSecs / shorter
	if ratio > 1 {
		ratio = 1
	}
	return ratio, ratio
}

// destinationScore returns a score in [0, 1] and whether both candidates had
// destination coordinates. Neutral score 0.5 is used when coords are absent
// (neither helps nor hurts the overall confidence).
func destinationScore(a, b TripCandidate) (score float64, hasCoords bool) {
	if a.DestLat == nil || a.DestLng == nil || b.DestLat == nil || b.DestLng == nil {
		return 0.5, false
	}
	distKm := haversineKm(*a.DestLat, *a.DestLng, *b.DestLat, *b.DestLng)
	// 0 km → 1.0; 2 km → 0.0 (linear)
	score = math.Max(0, 1-distKm/2.0)
	return score, true
}

// originScore returns a score in [0, 1]. Threshold is 3 km (vs 2 km for
// destination) because GPS can wake up 100-200 m late relative to the receipt's
// exact pickup point.
func originScore(a, b TripCandidate) float64 {
	if a.OriginLat == nil || a.OriginLng == nil || b.OriginLat == nil || b.OriginLng == nil {
		return 0.5
	}
	distKm := haversineKm(*a.OriginLat, *a.OriginLng, *b.OriginLat, *b.OriginLng)
	// 0 km → 1.0; 3 km → 0.0 (linear)
	return math.Max(0, 1-distKm/3.0)
}

// durationScore returns a score in [0, 1]. 30+ minute delta → 0.
func durationScore(a, b TripCandidate) float64 {
	if a.DurationMinutes == nil || b.DurationMinutes == nil {
		return 0.5
	}
	delta := math.Abs(float64(*a.DurationMinutes - *b.DurationMinutes))
	return math.Max(0, 1-delta/30.0)
}

// ------------------- mode compatibility -------------------

// modeGroup maps a transport mode to its compatibility class. Modes in the
// same group are considered equivalent for dedup purposes (e.g. "cab" and
// "car" are both 4-wheel motorised; "two_wheeler" covers Rapido bike taxi).
var modeGroup = map[string]int{
	"car":          1,
	"cab":          1,
	"two_wheeler":  2,
	"auto_rickshaw": 3,
	"bus":          4,
	"metro":        5,
	"train":        6,
	"walking":      7,
	"walk":         7,
	"cycling":      8,
	"bicycle":      8,
	"flight":       9,
}

func modesCompatible(a, b string) bool {
	ga, aOk := modeGroup[a]
	gb, bOk := modeGroup[b]
	if !aOk || !bOk {
		// Unknown mode — treat as compatible (don't hard-fail on missing data).
		return true
	}
	return ga == gb
}

// ------------------- Haversine -------------------

const earthRadiusKm = 6371.0

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
