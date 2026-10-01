package geocoding

import (
	"context"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

const redisLimiterKey = "motus:ratelimit:geocoding"

// Limiter blocks until the caller may send the next geocoding request.
type Limiter interface {
	Wait(ctx context.Context) error
}

// RedisLimiter enforces one geocoding rate across all pods sharing a Redis.
// If Redis fails, it falls back to a per-process limiter of the same rate.
type RedisLimiter struct {
	limiter  *redis_rate.Limiter
	limit    redis_rate.Limit
	fallback *rate.Limiter
}

// NewRedisLimiter returns a limiter allowing perSecond requests per second in total.
func NewRedisLimiter(client *redis.Client, perSecond float64) *RedisLimiter {
	return &RedisLimiter{
		limiter:  redis_rate.NewLimiter(client),
		limit:    redis_rate.Limit{Rate: 1, Burst: 1, Period: time.Duration(float64(time.Second) / perSecond)},
		fallback: rate.NewLimiter(rate.Limit(perSecond), 1),
	}
}

// Wait blocks until the shared limit allows one request or ctx is done.
func (l *RedisLimiter) Wait(ctx context.Context) error {
	for {
		res, err := l.limiter.Allow(ctx, redisLimiterKey, l.limit)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return l.fallback.Wait(ctx)
		}
		if res.Allowed > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(res.RetryAfter):
		}
	}
}
