package middleware

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
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
	limitHeader := fmt.Sprintf("%.2f", rps)
	roundedLimit := strconv.Itoa(int(math.Round(rps)))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Add("X-Rate-Limit-Limit", limitHeader)
			h.Add("X-Rate-Limit-Duration", "1")
			if xff := r.Header.Get("X-Forwarded-For"); strings.TrimSpace(xff) != "" {
				h.Add("X-Rate-Limit-Request-Forwarded-For", xff)
			}
			h.Add("X-Rate-Limit-Request-Remote-Addr", r.RemoteAddr)

			ip := clientIP(r.RemoteAddr)
			if ip == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed, remaining := store.allow(ip + "|" + r.URL.Path)
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

// LoginRateLimit returns middleware with the default login rate limit (5 req/min).
func LoginRateLimit() func(http.Handler) http.Handler {
	return RateLimit(DefaultLoginRateLimit())
}

// APIRateLimit returns middleware with the default API rate limit (100 req/min).
func APIRateLimit() func(http.Handler) http.Handler {
	return RateLimit(DefaultAPIRateLimit())
}
