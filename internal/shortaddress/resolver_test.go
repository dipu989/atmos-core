package shortaddress

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeCache struct {
	mu       sync.Mutex
	store    map[string]string
	getCalls int
	putCalls int
	getErr   error
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: map[string]string{}}
}

func cacheKey(lat, lng float64) string {
	return fmt.Sprintf("%v,%v", lat, lng)
}

func (f *fakeCache) Get(_ context.Context, lat, lng float64) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.store[cacheKey(lat, lng)]
	return v, ok, nil
}

func (f *fakeCache) Put(_ context.Context, lat, lng float64, _, display string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putCalls++
	f.store[cacheKey(lat, lng)] = display
	return nil
}

func newTestResolver(t *testing.T, cache Cache, reverseURL, detailsURL string) *googleResolver {
	t.Helper()
	return &googleResolver{
		apiKey:            "test-key",
		cache:             cache,
		client:            http.DefaultClient,
		reverseGeocodeURL: reverseURL,
		placeDetailsURL:   detailsURL,
	}
}

func TestResolve_CacheHit_NoHTTPCall(t *testing.T) {
	cache := newFakeCache()
	cache.store[cacheKey(roundCoord(12.9716), roundCoord(77.5946))] = "Cached Address"

	// Point both URLs at an address nothing is listening on — if the cache
	// hit didn't short-circuit, the HTTP call would fail and Resolve would
	// return an error instead of the cached value.
	r := newTestResolver(t, cache, "http://127.0.0.1:0", "http://127.0.0.1:0")

	got, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "Cached Address" {
		t.Errorf("Resolve() = %q, want %q", got, "Cached Address")
	}
	if cache.putCalls != 0 {
		t.Errorf("cache.putCalls = %d, want 0 (cache hit should not write back)", cache.putCalls)
	}
}

func TestResolve_CacheMiss_ResolvesAndCaches(t *testing.T) {
	reverseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","results":[{"place_id":"abc123"}]}`))
	}))
	defer reverseSrv.Close()

	detailsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/abc123" {
			t.Errorf("place details request path = %q, want /abc123", req.URL.Path)
		}
		if req.Header.Get("X-Goog-FieldMask") != "shortFormattedAddress" {
			t.Errorf("X-Goog-FieldMask header = %q, want shortFormattedAddress", req.Header.Get("X-Goog-FieldMask"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shortFormattedAddress":"Kaggadasapura, Bengaluru"}`))
	}))
	defer detailsSrv.Close()

	cache := newFakeCache()
	r := newTestResolver(t, cache, reverseSrv.URL, detailsSrv.URL+"/")

	got, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "Kaggadasapura, Bengaluru" {
		t.Errorf("Resolve() = %q, want %q", got, "Kaggadasapura, Bengaluru")
	}
	if cache.putCalls != 1 {
		t.Errorf("cache.putCalls = %d, want 1", cache.putCalls)
	}
}

func TestNew_NoAPIKey_ReturnsUnavailable(t *testing.T) {
	r := New("", newFakeCache())
	_, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("Resolve() error = %v, want ErrUnavailable", err)
	}
}

func TestResolve_ReverseGeocodeNoResults_ReturnsUnavailable(t *testing.T) {
	reverseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ZERO_RESULTS","results":[]}`))
	}))
	defer reverseSrv.Close()

	cache := newFakeCache()
	r := newTestResolver(t, cache, reverseSrv.URL, "http://127.0.0.1:0")

	_, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("Resolve() error = %v, want ErrUnavailable", err)
	}
	if cache.putCalls != 0 {
		t.Errorf("cache.putCalls = %d, want 0", cache.putCalls)
	}
}

func TestResolve_CacheGetError_FallsBackToAPI(t *testing.T) {
	reverseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","results":[{"place_id":"abc123"}]}`))
	}))
	defer reverseSrv.Close()

	detailsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shortFormattedAddress":"Kaggadasapura, Bengaluru"}`))
	}))
	defer detailsSrv.Close()

	cache := newFakeCache()
	cache.getErr = errors.New("connection pool exhausted")
	r := newTestResolver(t, cache, reverseSrv.URL, detailsSrv.URL+"/")

	// A broken cache (real DB error, not just a miss) must degrade to the
	// full API resolve rather than failing the whole Resolve call.
	got, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "Kaggadasapura, Bengaluru" {
		t.Errorf("Resolve() = %q, want %q", got, "Kaggadasapura, Bengaluru")
	}
}

func TestResolve_PlaceDetailsHTTPError_ReturnsError(t *testing.T) {
	reverseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","results":[{"place_id":"abc123"}]}`))
	}))
	defer reverseSrv.Close()

	detailsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer detailsSrv.Close()

	cache := newFakeCache()
	r := newTestResolver(t, cache, reverseSrv.URL, detailsSrv.URL+"/")

	_, err := r.Resolve(context.Background(), 12.9716, 77.5946)
	if err == nil {
		t.Fatal("Resolve() error = nil, want non-nil")
	}
	if cache.putCalls != 0 {
		t.Errorf("cache.putCalls = %d, want 0", cache.putCalls)
	}
}
