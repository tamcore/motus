package geocoding

import (
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
