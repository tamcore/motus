package websocket

import (
	"sync"
	"testing"
	"time"
)

func TestDeviceAccessCache_GetSet(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	t.Run("miss on empty cache", func(t *testing.T) {
		ids, ok := c.get(1)
		if ok {
			t.Error("expected cache miss on empty cache")
		}
		if ids != nil {
			t.Errorf("expected nil, got %v", ids)
		}
	})

	t.Run("hit after set", func(t *testing.T) {
		c.set(10, []int64{1, 2, 3})

		ids, ok := c.get(10)
		if !ok {
			t.Fatal("expected cache hit")
		}
		if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Errorf("expected [1 2 3], got %v", ids)
		}
	})

	t.Run("miss for different device", func(t *testing.T) {
		_, ok := c.get(99)
		if ok {
			t.Error("expected cache miss for uncached device")
		}
	})

	t.Run("set empty slice", func(t *testing.T) {
		c.set(20, []int64{})

		ids, ok := c.get(20)
		if !ok {
			t.Fatal("expected cache hit for empty slice")
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})

	t.Run("set nil slice", func(t *testing.T) {
		c.set(30, nil)

		ids, ok := c.get(30)
		if !ok {
			t.Fatal("expected cache hit for nil slice")
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})

	t.Run("overwrite existing entry", func(t *testing.T) {
		c.set(10, []int64{1, 2, 3})
		c.set(10, []int64{4, 5})

		ids, ok := c.get(10)
		if !ok {
			t.Fatal("expected cache hit")
		}
		if len(ids) != 2 || ids[0] != 4 || ids[1] != 5 {
			t.Errorf("expected [4 5], got %v", ids)
		}
	})
}

func TestDeviceAccessCache_ReturnsCopy(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)
	c.set(10, []int64{1, 2, 3})

	// Get and mutate the returned slice.
	ids, _ := c.get(10)
	ids[0] = 999

	// The cached data should be unaffected.
	ids2, _ := c.get(10)
	if ids2[0] != 1 {
		t.Errorf("cache was mutated through returned slice: expected 1, got %d", ids2[0])
	}
}

func TestDeviceAccessCache_SetCopiesInput(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)
	input := []int64{1, 2, 3}
	c.set(10, input)

	// Mutate the input slice after set.
	input[0] = 999

	// The cached data should be unaffected.
	ids, _ := c.get(10)
	if ids[0] != 1 {
		t.Errorf("cache was mutated through input slice: expected 1, got %d", ids[0])
	}
}

func TestDeviceAccessCache_TTLExpiration(t *testing.T) {
	c := newDeviceAccessCache(5 * time.Second)

	// Use a controllable clock.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	c.set(10, []int64{1, 2})

	// Still valid within TTL.
	now = now.Add(4 * time.Second)
	ids, ok := c.get(10)
	if !ok {
		t.Fatal("expected cache hit within TTL")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}

	// Expired after TTL.
	now = now.Add(2 * time.Second) // total: 6s > 5s TTL
	_, ok = c.get(10)
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}

	// Entry should have been lazily evicted.
	if c.len() != 0 {
		t.Errorf("expected 0 entries after eviction, got %d", c.len())
	}
}

func TestDeviceAccessCache_TTLBoundary(t *testing.T) {
	c := newDeviceAccessCache(10 * time.Second)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	c.set(10, []int64{1})

	// At exactly TTL: now == expiresAt, so time.After returns false.
	// The entry is still considered valid at the exact boundary.
	now = now.Add(10 * time.Second)
	_, ok := c.get(10)
	if !ok {
		t.Error("expected cache hit at exact TTL boundary (time.After is strict >)")
	}

	// One nanosecond later it is expired.
	now = now.Add(1 * time.Nanosecond)
	_, ok = c.get(10)
	if ok {
		t.Error("expected cache miss one nanosecond after TTL boundary")
	}
}

func TestDeviceAccessCache_Invalidate(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	c.set(10, []int64{1, 2})
	c.set(20, []int64{3})

	// Invalidate device 10 only.
	c.invalidate(10)

	_, ok := c.get(10)
	if ok {
		t.Error("expected cache miss after invalidation")
	}

	// Device 20 should still be cached.
	ids, ok := c.get(20)
	if !ok {
		t.Fatal("expected cache hit for non-invalidated device")
	}
	if len(ids) != 1 || ids[0] != 3 {
		t.Errorf("expected [3], got %v", ids)
	}
}

func TestDeviceAccessCache_InvalidateNonexistent(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	// Should not panic or error.
	c.invalidate(999)
}

func TestDeviceAccessCache_InvalidateAll(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	c.set(10, []int64{1})
	c.set(20, []int64{2})
	c.set(30, []int64{3})

	c.invalidateAll()

	if c.len() != 0 {
		t.Errorf("expected 0 entries after invalidateAll, got %d", c.len())
	}

	for _, deviceID := range []int64{10, 20, 30} {
		_, ok := c.get(deviceID)
		if ok {
			t.Errorf("expected cache miss for device %d after invalidateAll", deviceID)
		}
	}
}

func TestDeviceAccessCache_DefaultTTL(t *testing.T) {
	c := newDeviceAccessCache(0) // should use defaultCacheTTL
	if c.ttl != defaultCacheTTL {
		t.Errorf("expected default TTL %v, got %v", defaultCacheTTL, c.ttl)
	}
}

func TestDeviceAccessCache_ConcurrentAccess(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	const numGoroutines = 100
	const numOps = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			deviceID := int64(id % 10)

			for j := 0; j < numOps; j++ {
				switch j % 4 {
				case 0:
					c.set(deviceID, []int64{int64(id), int64(j)})
				case 1:
					c.get(deviceID)
				case 2:
					c.invalidate(deviceID)
				case 3:
					c.len()
				}
			}
		}(i)
	}

	wg.Wait()
	// If we get here without data races (run with -race), the test passes.
}

func TestDeviceAccessCache_ConcurrentSetAndGet(t *testing.T) {
	c := newDeviceAccessCache(30 * time.Second)

	// One goroutine writes, another reads. Should not race.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			c.set(1, []int64{int64(i)})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			ids, ok := c.get(1)
			if ok && len(ids) != 1 {
				t.Errorf("unexpected IDs length: %d", len(ids))
			}
		}
	}()

	wg.Wait()
}

func TestDeviceAccessCache_LazyEvictionRace(t *testing.T) {
	// Test that concurrent expired reads + a refresh set do not cause issues.
	c := newDeviceAccessCache(1 * time.Second)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	c.set(10, []int64{1, 2})

	// Advance past TTL.
	now = now.Add(2 * time.Second)

	var wg sync.WaitGroup
	wg.Add(3)

	// Two goroutines try to read (triggering lazy eviction).
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			c.get(10)
		}()
	}

	// One goroutine refreshes the entry.
	go func() {
		defer wg.Done()
		c.set(10, []int64{3, 4})
	}()

	wg.Wait()
}
