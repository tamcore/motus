package geocoding

import (
	"testing"
	"time"
)

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		expected string
	}{
		{"exact 4 decimals", 52.5200, 13.4050, "52.5200,13.4050"},
		{"rounds up", 52.52005, 13.40505, "52.5201,13.4051"},
		{"rounds down", 52.52004, 13.40504, "52.5200,13.4050"},
		{"negative coords", -33.8688, 151.2093, "-33.8688,151.2093"},
		{"zero", 0.0, 0.0, "0.0000,0.0000"},
		// Two nearby points within ~11m should map to the same key.
		{"nearby point A", 52.52001, 13.40502, "52.5200,13.4050"},
		{"nearby point B", 52.52004, 13.40504, "52.5200,13.4050"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := cacheKey(tt.lat, tt.lon)
			if key != tt.expected {
				t.Errorf("cacheKey(%f, %f) = %q, want %q", tt.lat, tt.lon, key, tt.expected)
			}
		})
	}
}

func TestCacheKey_NearbyPointsSameKey(t *testing.T) {
	// Points within ~5m of each other should map to the same cache key.
	keyA := cacheKey(52.52001, 13.40502)
	keyB := cacheKey(52.52004, 13.40504)
	if keyA != keyB {
		t.Errorf("nearby points should have same cache key: %q != %q", keyA, keyB)
	}

	// Points ~22m apart (different at 4th decimal) should have different keys.
	keyC := cacheKey(52.5200, 13.4050)
	keyD := cacheKey(52.5202, 13.4050)
	if keyC == keyD {
		t.Error("distant points should have different cache keys")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	cache := NewCache(1 * time.Minute)

	// Initially empty.
	if _, ok := cache.Get(52.5200, 13.4050); ok {
		t.Error("expected cache miss on empty cache")
	}

	// Set and retrieve.
	cache.Set(52.5200, 13.4050, "Berlin, Germany")
	addr, ok := cache.Get(52.5200, 13.4050)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if addr != "Berlin, Germany" {
		t.Errorf("unexpected address: %q", addr)
	}
}

func TestCache_Expiration(t *testing.T) {
	cache := NewCache(100 * time.Millisecond)

	// Use injectable clock for deterministic testing.
	now := time.Now()
	cache.now = func() time.Time { return now }

	cache.Set(52.5200, 13.4050, "Berlin, Germany")

	// Should be available before TTL.
	addr, ok := cache.Get(52.5200, 13.4050)
	if !ok || addr != "Berlin, Germany" {
		t.Fatal("expected cache hit before TTL")
	}

	// Advance time past TTL.
	now = now.Add(200 * time.Millisecond)

	// Should be expired.
	_, ok = cache.Get(52.5200, 13.4050)
	if ok {
		t.Error("expected cache miss after TTL")
	}

	// Entry should have been removed (lazy expiration).
	if cache.Size() != 0 {
		t.Errorf("expected 0 entries after expiration, got %d", cache.Size())
	}
}

func TestCache_OverwriteEntry(t *testing.T) {
	cache := NewCache(1 * time.Minute)

	cache.Set(52.5200, 13.4050, "Old Address")
	cache.Set(52.5200, 13.4050, "New Address")

	addr, ok := cache.Get(52.5200, 13.4050)
	if !ok || addr != "New Address" {
		t.Errorf("expected updated address, got %q", addr)
	}
}

func TestCache_MultipleEntries(t *testing.T) {
	cache := NewCache(1 * time.Minute)

	cache.Set(52.5200, 13.4050, "Berlin")
	cache.Set(48.8566, 2.3522, "Paris")
	cache.Set(40.7128, -74.0060, "New York")

	if cache.Size() != 3 {
		t.Errorf("expected 3 entries, got %d", cache.Size())
	}

	tests := []struct {
		lat, lon float64
		expected string
	}{
		{52.5200, 13.4050, "Berlin"},
		{48.8566, 2.3522, "Paris"},
		{40.7128, -74.0060, "New York"},
	}

	for _, tt := range tests {
		addr, ok := cache.Get(tt.lat, tt.lon)
		if !ok || addr != tt.expected {
			t.Errorf("Get(%f, %f) = %q, want %q", tt.lat, tt.lon, addr, tt.expected)
		}
	}
}

func TestCache_Cleanup(t *testing.T) {
	cache := NewCache(100 * time.Millisecond)

	now := time.Now()
	cache.now = func() time.Time { return now }

	cache.Set(52.5200, 13.4050, "Berlin")
	cache.Set(48.8566, 2.3522, "Paris")

	if cache.Size() != 2 {
		t.Fatalf("expected 2 entries, got %d", cache.Size())
	}

	// Advance time past TTL.
	now = now.Add(200 * time.Millisecond)

	removed := cache.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if cache.Size() != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", cache.Size())
	}
}

func TestCache_CleanupPartialExpiry(t *testing.T) {
	cache := NewCache(1 * time.Minute)

	now := time.Now()
	cache.now = func() time.Time { return now }

	cache.Set(52.5200, 13.4050, "Berlin")

	// Advance 30 seconds.
	now = now.Add(30 * time.Second)

	// Add another entry.
	cache.Set(48.8566, 2.3522, "Paris")

	// Advance another 40 seconds (total 70s - Berlin expired, Paris still valid).
	now = now.Add(40 * time.Second)

	removed := cache.Cleanup()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if cache.Size() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", cache.Size())
	}

	// Paris should still be retrievable.
	addr, ok := cache.Get(48.8566, 2.3522)
	if !ok || addr != "Paris" {
		t.Errorf("expected Paris to still be cached, got %q (ok=%v)", addr, ok)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(1 * time.Second)

	// Run concurrent reads and writes to test thread safety.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			lat := float64(n)
			lon := float64(n + 100)
			for j := 0; j < 100; j++ {
				cache.Set(lat, lon, "addr")
				cache.Get(lat, lon)
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no panics occurred and size is reasonable.
	if cache.Size() > 10 {
		t.Errorf("unexpected cache size: %d", cache.Size())
	}
}

func TestCache_LazyExpirationRaceCondition(t *testing.T) {
	// Test that concurrent reads of an expired entry don't cause issues.
	cache := NewCache(50 * time.Millisecond)

	now := time.Now()
	cache.now = func() time.Time { return now }

	cache.Set(52.5200, 13.4050, "Berlin")

	// Expire it.
	now = now.Add(100 * time.Millisecond)

	// Concurrent reads should all get cache miss without panicking.
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = cache.Get(52.5200, 13.4050)
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}
