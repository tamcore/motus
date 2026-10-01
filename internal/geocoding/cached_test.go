package geocoding

import (
	"context"
	"errors"
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
	cg := NewCachedGeocoder(mock, 1*time.Minute, nil)

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
	cg := NewCachedGeocoder(mock, 1*time.Minute, nil)

	// Two different locations should each call the geocoder.
	cg.Lookup(context.Background(), 52.5200, 13.4050)
	cg.Lookup(context.Background(), 48.8566, 2.3522)

	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 geocoder calls, got %d", mock.calls.Load())
	}
}

func TestCachedGeocoder_Lookup_GeocodingError_NotCached(t *testing.T) {
	mock := &mockGeocoder{err: fmt.Errorf("service unavailable")}
	cg := NewCachedGeocoder(mock, 1*time.Minute, nil)

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
	cg := NewCachedGeocoder(mock, 100*time.Millisecond, nil)

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
	cg := NewCachedGeocoder(mock, 1*time.Minute, nil)

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
	cg := NewCachedGeocoder(mock, 50*time.Millisecond, nil)

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

func TestCachedGeocoder_Logger(t *testing.T) {
	g := &mockGeocoder{response: "Berlin, Germany"}
	if cg := NewCachedGeocoder(g, time.Hour, nil); cg.logger != slog.Default() {
		t.Error("nil logger should default to slog.Default()")
	}
	custom := slog.New(slog.Default().Handler())
	if cg := NewCachedGeocoder(g, time.Hour, custom); cg.logger != custom {
		t.Error("custom logger should be kept")
	}
}

func waitResolved(t *testing.T, cg *CachedGeocoder, key int64) Resolved {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if r, ok := cg.Resolved(key); ok {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatal("prefetch did not resolve")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCachedGeocoder_PrefetchResolvesAndFillsCache(t *testing.T) {
	mock := &mockGeocoder{response: "Berlin, Germany"}
	cg := NewCachedGeocoder(mock, time.Minute, nil)
	go cg.StartPrefetch(t.Context())

	cg.Prefetch(7, 52.52, 13.405)
	r := waitResolved(t, cg, 7)
	if r.Address != "Berlin, Germany" || r.Lat != 52.52 || r.Lon != 13.405 {
		t.Errorf("Resolved = %+v, want Berlin at 52.52,13.405", r)
	}
	if addr, ok := cg.Peek(52.52, 13.405); !ok || addr != "Berlin, Germany" {
		t.Errorf("Peek = %q,%v, want cached Berlin", addr, ok)
	}
}

func TestCachedGeocoder_PrefetchCoalescesPerKey(t *testing.T) {
	mock := &mockGeocoder{response: "somewhere"}
	cg := NewCachedGeocoder(mock, time.Minute, nil)
	for i := range 10 {
		cg.Prefetch(1, float64(i), 0)
	}
	go cg.StartPrefetch(t.Context())

	if r := waitResolved(t, cg, 1); r.Lat != 9 {
		t.Errorf("resolved lat = %v, want latest 9", r.Lat)
	}
	time.Sleep(20 * time.Millisecond)
	if n := mock.calls.Load(); n != 1 {
		t.Errorf("geocoder calls = %d, want 1 for 10 coalesced requests", n)
	}
}

func TestCachedGeocoder_PrefetchSkipsFailedLookups(t *testing.T) {
	cg := NewCachedGeocoder(&mockGeocoder{err: errors.New("down")}, time.Minute, nil)
	go cg.StartPrefetch(t.Context())
	cg.Prefetch(1, 1, 1)
	time.Sleep(50 * time.Millisecond)
	if _, ok := cg.Resolved(1); ok {
		t.Error("failed lookup reported as resolved")
	}
}

func TestCachedGeocoder_PrefetchNeverBlocks(t *testing.T) {
	cg := NewCachedGeocoder(&mockGeocoder{}, time.Minute, nil)
	done := make(chan struct{})
	go func() {
		for i := range prefetchQueueSize * 2 {
			cg.Prefetch(int64(i), float64(i), 0)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Prefetch blocked with a full queue and no worker")
	}
}
