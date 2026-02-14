package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
)

// RateLimitConfig holds rate limiting parameters.
type RateLimitConfig struct {
	// Max is the maximum number of requests allowed per Period.
	Max float64
	// Period is the time window for rate limiting.
	Period time.Duration
}

// DefaultLoginRateLimit returns the default rate limit for login endpoints
// (5 requests per minute). Override with MOTUS_LOGIN_RATE_LIMIT env var.
func DefaultLoginRateLimit() RateLimitConfig {
	max := 5.0
	if v := os.Getenv("MOTUS_LOGIN_RATE_LIMIT"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			max = n
		}
	}
	return RateLimitConfig{Max: max, Period: time.Minute}
}

// DefaultAPIRateLimit returns the default rate limit for general API endpoints
// (100 requests per minute). Override with MOTUS_API_RATE_LIMIT env var.
func DefaultAPIRateLimit() RateLimitConfig {
	max := 100.0
	if v := os.Getenv("MOTUS_API_RATE_LIMIT"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			max = n
		}
	}
	return RateLimitConfig{Max: max, Period: time.Minute}
}

// newLimiter creates a tollbooth limiter from a RateLimitConfig.
// It uses a token-bucket approach where the refill rate is Max/Period
// (requests per second) and the burst size equals Max, allowing up to
// Max requests in a single burst before throttling begins.
func newLimiter(cfg RateLimitConfig) *limiter.Limiter {
	// tollbooth's max is requests per second; convert from requests per period.
	rps := cfg.Max / cfg.Period.Seconds()
	lmt := tollbooth.NewLimiter(rps, &limiter.ExpirableOptions{
		DefaultExpirationTTL: cfg.Period,
	})
	// Allow a burst equal to the full period quota so that clients can make
	// up to Max requests immediately before tokens need to refill.
	lmt.SetBurst(int(math.Max(1, cfg.Max)))
	// chi's RealIP middleware already rewrites RemoteAddr, but also check
	// the original headers as a fallback for direct access without proxies.
	lmt.SetIPLookups([]string{"RemoteAddr", "X-Real-Ip", "X-Forwarded-For"})
	return lmt
}

// rateLimitResponse writes a JSON 429 response matching the project's error format.
func rateLimitResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
}

// RateLimit returns middleware that applies the given rate limit configuration.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	lmt := newLimiter(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpError := tollbooth.LimitByRequest(lmt, w, r)
			if httpError != nil {
				rateLimitResponse(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoginRateLimit returns middleware with the default login rate limit (5 req/min).
func LoginRateLimit() func(http.Handler) http.Handler {
	return RateLimit(DefaultLoginRateLimit())
}

// APIRateLimit returns middleware with the default API rate limit (100 req/min).
func APIRateLimit() func(http.Handler) http.Handler {
	return RateLimit(DefaultAPIRateLimit())
}
