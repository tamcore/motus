package middleware

import (
	"maps"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tamcore/motus/internal/api"
	"golang.org/x/time/rate"
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
	return perMinuteFromEnv("MOTUS_LOGIN_RATE_LIMIT", 5)
}

// DefaultAPIRateLimit returns the default rate limit for general API endpoints
// (100 requests per minute). Override with MOTUS_API_RATE_LIMIT env var.
func DefaultAPIRateLimit() RateLimitConfig {
	return perMinuteFromEnv("MOTUS_API_RATE_LIMIT", 100)
}

// perMinuteFromEnv ignores unset, unparsable or non-positive values.
func perMinuteFromEnv(key string, def float64) RateLimitConfig {
	if n, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && n > 0 {
		def = n
	}
	return RateLimitConfig{Max: def, Period: time.Minute}
}

type bucket struct {
	limiter *rate.Limiter
	expires time.Time
}

type bucketStore struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	nextSweep time.Time
	limit     rate.Limit
	burst     int
	ttl       time.Duration
}

// allow reports whether a request for key may proceed and how many tokens
// remain. A bucket lives for ttl after creation, then starts full again.
func (s *bucketStore) allow(key string) (bool, int) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.After(s.nextSweep) {
		maps.DeleteFunc(s.buckets, func(_ string, b *bucket) bool { return now.After(b.expires) })
		s.nextSweep = now.Add(s.ttl)
	}
	b, ok := s.buckets[key]
	if !ok || now.After(b.expires) {
		b = &bucket{limiter: rate.NewLimiter(s.limit, s.burst), expires: now.Add(s.ttl)}
		s.buckets[key] = b
	}
	if !b.limiter.AllowN(now, 1) {
		return false, 0
	}
	return true, int(b.limiter.TokensAt(now))
}

// clientIP returns the RemoteAddr host (chi's RealIP has already rewritten
// it to the client IP), reduced to its /64 prefix for IPv6.
func clientIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	if i := strings.IndexAny(ip, ".:"); i < 0 || ip[i] == '.' {
		return ip
	}
	v6 := net.ParseIP(ip)
	if v6 == nil {
		return ip
	}
	clear(v6[8:])
	return v6.String()
}

// rateLimitResponse writes a JSON 429 response matching the project's error format.
func rateLimitResponse(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	api.RespondError(w, http.StatusTooManyRequests, "rate limit exceeded")
}

// RateLimit returns middleware that applies a token bucket per client IP and
// path, refilling at Max/Period with a burst of Max.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	rps := cfg.Max / cfg.Period.Seconds()
	store := &bucketStore{
		buckets: map[string]*bucket{},
		limit:   rate.Limit(rps),
		burst:   int(max(1, cfg.Max)),
		ttl:     cfg.Period,
	}
	roundedLimit := strconv.Itoa(int(math.Round(rps)))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r.RemoteAddr)
			if ip == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed, remaining := store.allow(ip + "|" + r.URL.Path)
			h := w.Header()
			h.Add("RateLimit-Limit", roundedLimit)
			h.Add("RateLimit-Reset", "1")
			h.Add("RateLimit-Remaining", strconv.Itoa(remaining))
			if !allowed {
				rateLimitResponse(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
