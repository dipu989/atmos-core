package tripmatcher

import (
	"testing"
	"time"
)

// ptr helpers

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }
func ts(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
func tsEnd(start time.Time, minutes int) *time.Time {
	end := start.Add(time.Duration(minutes) * time.Minute)
	return &end
}

// Bangalore coords used throughout.
// Home (Koramangala):  12.9352, 77.6245
// Work (Whitefield):   12.9698, 77.7499
// Offset ~200 m south: 12.9334, 77.6245
// Another destination: 12.9800, 77.7300

const (
	homeLat     = 12.9352
	homeLng     = 77.6245
	workLat     = 12.9698
	workLng     = 77.7499
	nearHomeLat = 12.9334 // ~200 m south of home
	nearHomeLng = 77.6245
)

// ---- E1: Perfect match -------------------------------------------------------

func TestPerfectMatch(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	a := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 45),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(45),
	}
	b := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 45),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(45),
	}
	r := Score(a, b)
	if r.Confidence < ThresholdAutoMerge {
		t.Errorf("perfect match: want confidence ≥ %.2f, got %.3f", ThresholdAutoMerge, r.Confidence)
	}
	if !r.HasCoords {
		t.Error("perfect match: expected HasCoords=true")
	}
}

// ---- E2: GPS wakes up 200 m late (your real scenario) -----------------------
// Origin offset is ~200 m. Destination is identical. Time overlap is 100%.
// Should still auto-merge despite the origin penalty.

func TestGPSLateStart200m(t *testing.T) {
	t.Parallel()
	receiptStart := ts("2024-06-15T09:00:00Z")
	gpsStart := ts("2024-06-15T09:02:00Z") // 2 minutes late

	receipt := TripCandidate{
		StartedAt:       receiptStart,
		EndedAt:         tsEnd(receiptStart, 45),
		OriginLat:       f64(homeLat), // exact pickup
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(45),
	}
	gps := TripCandidate{
		StartedAt:       gpsStart,
		EndedAt:         tsEnd(gpsStart, 43), // GPS missed the first 2 min
		OriginLat:       f64(nearHomeLat),    // ~200 m from actual pickup
		OriginLng:       f64(nearHomeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(43),
	}
	r := Score(receipt, gps)
	if r.Confidence < ThresholdAutoMerge {
		t.Errorf("GPS late start: want confidence ≥ %.2f, got %.3f (signals: %+v)",
			ThresholdAutoMerge, r.Confidence, r.Signals)
	}
}

// ---- E3: Two Uber trips in the same hour ------------------------------------
// Back-to-back trips: first ends at 09:45, second starts at 10:00.
// Zero time overlap → hard-fail.

func TestTwoTripsNoOverlap(t *testing.T) {
	t.Parallel()
	trip1Start := ts("2024-06-15T09:00:00Z")
	trip2Start := ts("2024-06-15T10:00:00Z") // 15 min gap after trip1 ends

	a := TripCandidate{
		StartedAt:       trip1Start,
		EndedAt:         tsEnd(trip1Start, 45),
		TransportMode:   "cab",
		DurationMinutes: i(45),
	}
	b := TripCandidate{
		StartedAt:       trip2Start,
		EndedAt:         tsEnd(trip2Start, 30),
		TransportMode:   "cab",
		DurationMinutes: i(30),
	}
	r := Score(a, b)
	if r.Confidence != 0 {
		t.Errorf("no time overlap: want confidence=0, got %.3f", r.Confidence)
	}
}

// ---- E4: Mode mismatch (Rapido two_wheeler vs GPS cab) ----------------------

func TestModeMismatch(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	cab := TripCandidate{
		StartedAt:     start,
		EndedAt:       tsEnd(start, 40),
		TransportMode: "cab",
	}
	bike := TripCandidate{
		StartedAt:     start,
		EndedAt:       tsEnd(start, 40),
		TransportMode: "two_wheeler",
	}
	r := Score(cab, bike)
	if r.Confidence != 0 {
		t.Errorf("mode mismatch cab vs two_wheeler: want confidence=0, got %.3f", r.Confidence)
	}
}

// ---- E4b: Compatible modes (car vs cab) -------------------------------------

func TestCompatibleModes(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	a := TripCandidate{StartedAt: start, EndedAt: tsEnd(start, 40), TransportMode: "car"}
	b := TripCandidate{StartedAt: start, EndedAt: tsEnd(start, 40), TransportMode: "cab"}
	r := Score(a, b)
	if r.Confidence == 0 {
		t.Error("car vs cab should be compatible, got confidence=0")
	}
}

// ---- E4c: walking vs cycling mismatch ---------------------------------------

func TestWalkingVsCyclingMismatch(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	a := TripCandidate{StartedAt: start, EndedAt: tsEnd(start, 30), TransportMode: "walking"}
	b := TripCandidate{StartedAt: start, EndedAt: tsEnd(start, 30), TransportMode: "cycling"}
	r := Score(a, b)
	if r.Confidence != 0 {
		t.Errorf("walking vs cycling: want confidence=0, got %.3f", r.Confidence)
	}
}

// ---- E5: GPS records only 60% of the trip (tunnel / signal loss) ------------
// Receipt has full 90 min record; GPS only has 54 min overlap.
// Should still score above review threshold.

func TestGPSPartialCoverage(t *testing.T) {
	t.Parallel()
	receiptStart := ts("2024-06-15T08:00:00Z")
	gpsStart := ts("2024-06-15T08:36:00Z") // GPS starts 36 min in (tunnel entry)

	receipt := TripCandidate{
		StartedAt:       receiptStart,
		EndedAt:         tsEnd(receiptStart, 90),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "train",
		DurationMinutes: i(90),
	}
	gps := TripCandidate{
		StartedAt:       gpsStart,
		EndedAt:         tsEnd(receiptStart, 90), // same end, just late start
		OriginLat:       nil,                     // no coords at GPS start (tunnel)
		OriginLng:       nil,
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "train",
		DurationMinutes: i(54),
	}
	r := Score(receipt, gps)
	if r.Confidence < ThresholdReview {
		t.Errorf("60%% GPS coverage: want confidence ≥ %.2f, got %.3f (signals: %+v)",
			ThresholdReview, r.Confidence, r.Signals)
	}
}

// ---- E6: No coordinates on receipt (geocoding failed) -----------------------
// Should still compute a score from time + duration only.
// HasCoords must be false.

func TestNoCoordinatesOnReceipt(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	receipt := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 45),
		TransportMode:   "cab",
		DurationMinutes: i(45),
		// no coords
	}
	gps := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 45),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(45),
	}
	r := Score(receipt, gps)
	if r.HasCoords {
		t.Error("missing receipt coords: expected HasCoords=false")
	}
	// Time + duration are perfect matches → should be above review threshold
	// even without coords, but caller must use stricter ThresholdAutoMergeNoCoord.
	if r.Confidence < ThresholdReview {
		t.Errorf("no coords: want confidence ≥ %.2f, got %.3f", ThresholdReview, r.Confidence)
	}
}

// ---- E7: Minimal time overlap (< 10%) → effectively a separate trip ---------

func TestMinimalTimeOverlap(t *testing.T) {
	t.Parallel()
	// Trip A: 09:00–09:45. Trip B: 09:40–10:30.
	// Only 5 min overlap on a 45- and 50-min trip → very low time score.
	aStart := ts("2024-06-15T09:00:00Z")
	bStart := ts("2024-06-15T09:40:00Z")
	a := TripCandidate{
		StartedAt:       aStart,
		EndedAt:         tsEnd(aStart, 45),
		DurationMinutes: i(45),
	}
	b := TripCandidate{
		StartedAt:       bStart,
		EndedAt:         tsEnd(bStart, 50),
		DurationMinutes: i(50),
	}
	r := Score(a, b)
	if r.Confidence >= ThresholdAutoMerge {
		t.Errorf("minimal overlap: want confidence < %.2f, got %.3f", ThresholdAutoMerge, r.Confidence)
	}
}

// ---- E8: Unknown mode on GPS side (before user confirms) --------------------
// Mode check should be skipped → scored normally on time + coords.

func TestUnknownModeSkipsCheck(t *testing.T) {
	t.Parallel()
	start := ts("2024-06-15T09:00:00Z")
	receipt := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 40),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "cab",
		DurationMinutes: i(40),
	}
	gps := TripCandidate{
		StartedAt:       start,
		EndedAt:         tsEnd(start, 40),
		OriginLat:       f64(homeLat),
		OriginLng:       f64(homeLng),
		DestLat:         f64(workLat),
		DestLng:         f64(workLng),
		TransportMode:   "", // not yet confirmed by user
		DurationMinutes: i(40),
	}
	r := Score(receipt, gps)
	if r.Confidence < ThresholdAutoMerge {
		t.Errorf("unknown mode: want confidence ≥ %.2f, got %.3f", ThresholdAutoMerge, r.Confidence)
	}
}

// ---- Haversine unit check ---------------------------------------------------

func TestHaversine(t *testing.T) {
	t.Parallel()
	// Koramangala → Whitefield is roughly 15 km by road, ~13 km straight line.
	dist := haversineKm(homeLat, homeLng, workLat, workLng)
	if dist < 10 || dist > 20 {
		t.Errorf("haversine KOM→WF: expected ~13 km, got %.2f km", dist)
	}
	// Same point → 0.
	zero := haversineKm(homeLat, homeLng, homeLat, homeLng)
	if zero > 0.001 {
		t.Errorf("haversine same point: expected ~0, got %.6f", zero)
	}
	// 200 m offset → < 0.3 km.
	small := haversineKm(homeLat, homeLng, nearHomeLat, nearHomeLng)
	if small > 0.3 {
		t.Errorf("haversine 200m offset: expected < 0.3 km, got %.4f", small)
	}
}

// ---- Mode group coverage ---------------------------------------------------

func TestAllModesHaveGroups(t *testing.T) {
	t.Parallel()
	known := []string{"car", "cab", "two_wheeler", "auto_rickshaw", "bus",
		"metro", "train", "walking", "walk", "cycling", "bicycle", "flight"}
	for _, m := range known {
		if _, ok := modeGroup[m]; !ok {
			t.Errorf("mode %q missing from modeGroup", m)
		}
	}
}
