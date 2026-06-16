package service

import (
	"testing"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
)

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }

// ── isReceiptSource ──────────────────────────────────────────────────────────

func TestIsReceiptSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  actdomain.ActivitySource
		want bool
	}{
		{actdomain.SourceGmail, true},
		{actdomain.SourceUber, true},
		{actdomain.SourceOla, true},
		{actdomain.SourceRapido, true},
		{actdomain.SourceNammaYatri, true},
		{actdomain.SourceGPS, false},
		{actdomain.SourceGPSReceipt, false},
		{actdomain.SourceManual, false},
		{actdomain.SourceHealth, false},
	}
	for _, tc := range cases {
		if got := isReceiptSource(tc.src); got != tc.want {
			t.Errorf("isReceiptSource(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// ── buildGPSEnrichInput ──────────────────────────────────────────────────────

func TestBuildGPSEnrichInput_WithCoords(t *testing.T) {
	t.Parallel()
	input := IngestInput{
		StartedAt:  time.Now(),
		OriginLat:  f64(12.9352),
		OriginLng:  f64(77.6245),
		DestLat:    f64(12.9698),
		DestLng:    f64(77.7499),
		DistanceKM: f64(15.2),
		FareAmount: f64(200),
	}
	e := buildGPSEnrichInput(input, 0.82)

	if e.MatchConfidence != 0.82 {
		t.Errorf("MatchConfidence = %v, want 0.82", e.MatchConfidence)
	}
	if e.OriginLat == nil || *e.OriginLat != 12.9352 {
		t.Errorf("OriginLat = %v, want 12.9352", e.OriginLat)
	}
	if e.DestLat == nil || *e.DestLat != 12.9698 {
		t.Errorf("DestLat = %v, want 12.9698", e.DestLat)
	}
	// GPS enrichment must NOT overwrite fare or distance — receipt wins those.
	if e.FareAmount != nil {
		t.Errorf("FareAmount should be nil (receipt wins), got %v", *e.FareAmount)
	}
	if e.DistanceKM != nil {
		t.Errorf("DistanceKM should be nil (receipt wins), got %v", *e.DistanceKM)
	}
	if e.ReceiptID != "" {
		t.Errorf("ReceiptID should be empty (receipt keeps its own), got %q", e.ReceiptID)
	}
	if e.Provider != "" {
		t.Errorf("Provider should be empty (receipt wins), got %q", e.Provider)
	}
}

func TestBuildGPSEnrichInput_NoCoords(t *testing.T) {
	t.Parallel()
	input := IngestInput{
		StartedAt:       time.Now(),
		DurationMinutes: i(25),
	}
	e := buildGPSEnrichInput(input, 0.50)

	if e.OriginLat != nil {
		t.Errorf("OriginLat should be nil when GPS has no coords")
	}
	if e.DestLat != nil {
		t.Errorf("DestLat should be nil when GPS has no coords")
	}
}
