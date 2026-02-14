package geocoding

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// mockGeocoder implements Geocoder for testing.
type mockGeocoder struct {
	calls    atomic.Int32
	response string
	err      error
}

func (m *mockGeocoder) ReverseGeocode(_ context.Context, lat, lon float64) (string, error) {
	m.calls.Add(1)
	if m.err != nil {
		return coordinateFallback(lat, lon), m.err
	}
	return m.response, nil
}

func TestCachedGeocoder_Lookup_CacheHit(t *testing.T) {
	mock := &mockGeocoder{response: "Berlin, Germany"}
	cg := NewCachedGeocoder(mock, 1*time.Minute)

	// First call: cache miss, calls geocoder.
	addr := cg.Lookup(context.Background(), 52.5200, 13.4050)
	if addr != "Berlin, Germany" {
		t.Errorf("expected Berlin, got %q", addr)
	}
	if mock.calls.Load() != 1 {
		t.Errorf("expected 1 geocoder call, got %d", mock.calls.Load())
	}

	// Second call: cache hit, does NOT call geocoder.
	addr = cg.Lookup(context.Background(), 52.5200, 13.4050)
	if addr != "Berlin, Germany" {
		t.Errorf("expected Berlin, got %q", addr)
	}
	if mock.calls.Load() != 1 {
		t.Errorf("expected still 1 geocoder call, got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_Lookup_CacheMiss_DifferentLocation(t *testing.T) {
	mock := &mockGeocoder{response: "Address"}
	cg := NewCachedGeocoder(mock, 1*time.Minute)

	// Two different locations should each call the geocoder.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	cg.Lookup(context.Background(), 48.8566, 2.3522)

	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 geocoder calls, got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_Lookup_GeocodingError_NotCached(t *testing.T) {
	mock := &mockGeocoder{err: fmt.Errorf("service unavailable")}
	cg := NewCachedGeocoder(mock, 1*time.Minute)

	// First call fails: returns fallback.
	addr := cg.Lookup(context.Background(), 52.5200, 13.4050)
	if addr != "52.52000, 13.40500" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}

	// Second call: should retry (not cached).
	_ = cg.Lookup(context.Background(), 52.5200, 13.4050)
	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 geocoder calls (error should not be cached), got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_Lookup_CacheExpiry(t *testing.T) {
	mock := &mockGeocoder{response: "Berlin, Germany"}
	cg := NewCachedGeocoder(mock, 100*time.Millisecond)

	// Inject test clock.
	now := time.Now()
	cg.cache.now = func() time.Time { return now }

	// Populate cache.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	if mock.calls.Load() != 1 {
		t.Fatal("expected 1 call")
	}

	// Cache hit.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	if mock.calls.Load() != 1 {
		t.Fatal("expected cache hit")
	}

	// Advance time past TTL.
	now = now.Add(200 * time.Millisecond)

	// Should call geocoder again.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 geocoder calls after expiry, got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_NearbyPointsShareCache(t *testing.T) {
	mock := &mockGeocoder{response: "Neighborhood"}
	cg := NewCachedGeocoder(mock, 1*time.Minute)

	// These two points are within ~5m of each other, so they should
	// round to the same cache key (4 decimal places).
	cg.Lookup(context.Background(), 52.52001, 13.40502)
	cg.Lookup(context.Background(), 52.52004, 13.40504)

	// Only 1 geocoder call expected (second is cache hit).
	if mock.calls.Load() != 1 {
		t.Errorf("expected 1 geocoder call for nearby points, got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_StartCleanup(t *testing.T) {
	mock := &mockGeocoder{response: "Test"}
	cg := NewCachedGeocoder(mock, 50*time.Millisecond)

	// Populate cache.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	if cg.Cache().Size() != 1 {
		t.Fatal("expected 1 cache entry")
	}

	// Start cleanup with a short interval.
	ctx, cancel := context.WithCancel(context.Background())

	go cg.StartCleanup(ctx, 25*time.Millisecond)

	// Wait for the TTL to expire and cleanup to run.
	time.Sleep(200 * time.Millisecond)

	if cg.Cache().Size() != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", cg.Cache().Size())
	}

	cancel()
}

func TestCachedGeocoder_SetLogger(t *testing.T) {
	g := &mockGeocoder{response: "Berlin, Germany"}
	cg := NewCachedGeocoder(g, time.Hour)

	initial := cg.logger
	cg.SetLogger(nil) // nil should not change logger
	if cg.logger != initial {
		t.Error("SetLogger(nil) should not change logger")
	}
	custom := slog.Default()
	cg.SetLogger(custom)
	if cg.logger != custom {
		t.Error("SetLogger(custom) should replace logger")
	}
}
