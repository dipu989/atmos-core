package parser

import (
	"encoding/json"
	"testing"
)

// TestFlexFloat64 verifies that flexFloat64 accepts both JSON number and
// quoted-string forms, which Groq returns interchangeably.
func TestFlexFloat64(t *testing.T) {
	cases := []struct {
		input string
		want  *float64
	}{
		{`21.89`, ptr64(21.89)},
		{`"21.89"`, ptr64(21.89)},
		{`"13.63"`, ptr64(13.63)},
		{`null`, nil},
	}
	for _, c := range cases {
		var f flexFloat64
		if err := json.Unmarshal([]byte(c.input), &f); err != nil {
			t.Fatalf("input %q: unexpected error: %v", c.input, err)
		}
		if c.want == nil {
			if f.v != nil {
				t.Errorf("input %q: want nil, got %v", c.input, *f.v)
			}
		} else {
			if f.v == nil {
				t.Errorf("input %q: want %v, got nil", c.input, *c.want)
			} else if *f.v != *c.want {
				t.Errorf("input %q: want %v, got %v", c.input, *c.want, *f.v)
			}
		}
	}
}

// TestFlexInt verifies that flexInt accepts both JSON number and quoted-string.
func TestFlexInt(t *testing.T) {
	cases := []struct {
		input string
		want  *int
	}{
		{`37`, ptrInt(37)},
		{`"37"`, ptrInt(37)},
		{`null`, nil},
	}
	for _, c := range cases {
		var f flexInt
		if err := json.Unmarshal([]byte(c.input), &f); err != nil {
			t.Fatalf("input %q: unexpected error: %v", c.input, err)
		}
		if c.want == nil {
			if f.v != nil {
				t.Errorf("input %q: want nil, got %v", c.input, *f.v)
			}
		} else {
			if f.v == nil {
				t.Errorf("input %q: want %v, got nil", c.input, *c.want)
			} else if *f.v != *c.want {
				t.Errorf("input %q: want %v, got %v", c.input, *c.want, *f.v)
			}
		}
	}
}

// TestLLMParseGroqResponse verifies full parse of the exact JSON Groq returned
// for the June 6 Uber Auto ride — string-typed numerics and all.
func TestLLMParseGroqResponse(t *testing.T) {
	raw := `{
  "pickup_address": "XMR9+6VF, 16C Cross Rd, Pai Layout, Dooravani Nagar, Bengaluru, Karnataka 560016, India",
  "drop_address": "Sy no 90/3, K, 572/90, Outer Ring Rd, beside Manhpo Convention Center, DadaMastan Layout, Manayata Tech Park, Nagavara, Bengaluru, Karnataka 560045, India",
  "distance_km": "13.63",
  "duration_minutes": "37",
  "fare_amount": "235.54",
  "currency": "INR",
  "vehicle_type": "Auto",
  "provider": "Uber",
  "transport_mode": "cab",
  "started_at": "2026-06-06T20:54:00Z"
}`

	var data llmRideJSON
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if data.PickupAddress == nil || *data.PickupAddress == "" {
		t.Error("pickup_address should be populated")
	}
	if data.DropAddress == nil || *data.DropAddress == "" {
		t.Error("drop_address should be populated")
	}
	if data.DistanceKM.v == nil || *data.DistanceKM.v != 13.63 {
		t.Errorf("distance_km: want 13.63, got %v", data.DistanceKM.v)
	}
	if data.DurationMinutes.v == nil || *data.DurationMinutes.v != 37 {
		t.Errorf("duration_minutes: want 37, got %v", data.DurationMinutes.v)
	}
	if data.FareAmount.v == nil || *data.FareAmount.v != 235.54 {
		t.Errorf("fare_amount: want 235.54, got %v", data.FareAmount.v)
	}
}

func ptr64(v float64) *float64 { return &v }
func ptrInt(v int) *int        { return &v }
