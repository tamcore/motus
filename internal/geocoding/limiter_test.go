package geocoding

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_SharesRateAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	a, b := NewRedisLimiter(client, 10), NewRedisLimiter(client, 10)

	start := time.Now()
	for _, l := range []*RedisLimiter{a, b, a, b} {
		if err := l.Wait(t.Context()); err != nil {
			t.Fatalf("Wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Errorf("4 waits at a shared 10/s took %v, want >= 250ms", elapsed)
	}
}

func TestRedisLimiter_FallsBackWhenRedisDown(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	l := NewRedisLimiter(client, 10)
	mr.Close()

	if err := l.Wait(t.Context()); err != nil {
		t.Fatalf("Wait with Redis down: %v", err)
	}
}

func TestRedisLimiter_SharesFairlyBetweenContenders(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	counts := make([]atomic.Int32, 2)
	for i := range counts {
		l := NewRedisLimiter(client, 20)
		wg.Go(func() {
			for l.Wait(ctx) == nil {
				counts[i].Add(1)
			}
		})
	}
	wg.Wait()

	a, b := counts[0].Load(), counts[1].Load()
	if total := a + b; total < 20 || min(a, b) < total/4 {
		t.Errorf("slots = %d / %d, want both >= 25%% of %d", a, b, total)
	}
}
