package middleware

import (
	"net/http"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// NewRedisLoginRateLimit returns middleware that enforces the given rate limit
// using Redis as the shared store, making it effective across multiple pods.
// The client IP comes from RemoteAddr, which RealIP has already resolved.
func NewRedisLoginRateLimit(client *redis.Client, cfg RateLimitConfig) func(http.Handler) http.Handler {
	limiter := redis_rate.NewLimiter(client)
	limit := redis_rate.Limit{
		Rate:   int(cfg.Max),
		Period: cfg.Period,
		Burst:  int(cfg.Max),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r.RemoteAddr)
			key := "motus:ratelimit:login:" + ip

			res, err := limiter.Allow(r.Context(), key, limit)
			if err != nil || res.Allowed == 0 {
				rateLimitResponse(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
