package ttlcache

import (
	"sync"
	"testing"
	"time"
)

func TestCache_GetSet(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	t.Run("miss on empty cache", func(t *testing.T) {
		ids, ok := c.Get(1)
		if ok {
			t.Error("expected cache miss on empty cache")
		}
		if ids != nil {
			t.Errorf("expected nil, got %v", ids)
		}
	})

	t.Run("hit after set", func(t *testing.T) {
		c.Set(10, []int64{1, 2, 3})

		ids, ok := c.Get(10)
		if !ok {
			t.Fatal("expected cache hit")
		}
		if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Errorf("expected [1 2 3], got %v", ids)
		}
	})

	t.Run("miss for different device", func(t *testing.T) {
		_, ok := c.Get(99)
		if ok {
			t.Error("expected cache miss for uncached device")
		}
	})

	t.Run("set empty slice", func(t *testing.T) {
		c.Set(20, []int64{})

		ids, ok := c.Get(20)
		if !ok {
			t.Fatal("expected cache hit for empty slice")
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})

	t.Run("set nil slice", func(t *testing.T) {
		c.Set(30, nil)

		ids, ok := c.Get(30)
		if !ok {
			t.Fatal("expected cache hit for nil slice")
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})

	t.Run("overwrite existing entry", func(t *testing.T) {
		c.Set(10, []int64{1, 2, 3})
		c.Set(10, []int64{4, 5})

		ids, ok := c.Get(10)
		if !ok {
			t.Fatal("expected cache hit")
		}
		if len(ids) != 2 || ids[0] != 4 || ids[1] != 5 {
			t.Errorf("expected [4 5], got %v", ids)
		}
	})
}

func TestCache_TTLExpiration(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	// Use a controllable clock.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }

	c.Set(10, []int64{1, 2})

	// Still valid within TTL.
	now = now.Add(defaultCacheTTL - time.Second)
	ids, ok := c.Get(10)
	if !ok {
		t.Fatal("expected cache hit within TTL")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}

	// Expired after TTL.
	now = now.Add(2 * time.Second)
	_, ok = c.Get(10)
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}

}

func TestCache_TTLBoundary(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }

	c.Set(10, []int64{1})

	// At exactly TTL: now == expiresAt, so time.After returns false.
	// The entry is still considered valid at the exact boundary.
	now = now.Add(defaultCacheTTL)
	_, ok := c.Get(10)
	if !ok {
		t.Error("expected cache hit at exact TTL boundary (time.After is strict >)")
	}

	// One nanosecond later it is expired.
	now = now.Add(1 * time.Nanosecond)
	_, ok = c.Get(10)
	if ok {
		t.Error("expected cache miss one nanosecond after TTL boundary")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	c.Set(10, []int64{1, 2})
	c.Set(20, []int64{3})

	// Invalidate device 10 only.
	c.Delete(10)

	_, ok := c.Get(10)
	if ok {
		t.Error("expected cache miss after invalidation")
	}

	// Device 20 should still be cached.
	ids, ok := c.Get(20)
	if !ok {
		t.Fatal("expected cache hit for non-invalidated device")
	}
	if len(ids) != 1 || ids[0] != 3 {
		t.Errorf("expected [3], got %v", ids)
	}
}

func TestCache_InvalidateNonexistent(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	// Should not panic or error.
	c.Delete(999)
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	const numGoroutines = 100
	const numOps = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			deviceID := int64(id % 10)

			for j := range numOps {
				switch j % 3 {
				case 0:
					c.Set(deviceID, []int64{int64(id), int64(j)})
				case 1:
					c.Get(deviceID)
				case 2:
					c.Delete(deviceID)
				}
			}
		}(i)
	}

	wg.Wait()
	// If we get here without data races (run with -race), the test passes.
}

func TestCache_ConcurrentSetAndGet(t *testing.T) {
	c := New[int64, []int64](defaultCacheTTL)

	// One goroutine writes, another reads. Should not race.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 10000 {
			c.Set(1, []int64{int64(i)})
		}
	}()

	go func() {
		defer wg.Done()
		for range 10000 {
			ids, ok := c.Get(1)
			if ok && len(ids) != 1 {
				t.Errorf("unexpected IDs length: %d", len(ids))
			}
		}
	}()

	wg.Wait()
}

const defaultCacheTTL = 30 * time.Second
