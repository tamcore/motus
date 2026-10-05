package middleware

import (
	"maps"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"golang.org/x/time/rate"
)

// RateLimitConfig holds rate limiting parameters.
type RateLimitConfig struct {
	// Max is the maximum number of requests allowed per Period.
	Max float64
	// Period is the time window for rate limiting.
	Period time.Duration
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
	ip, ok := audit.ParseRemoteAddr(remoteAddr)
	if !ok {
		return remoteAddr
	}
	if ip.Is6() {
		ip = netip.PrefixFrom(ip, 64).Masked().Addr()
	}
	return ip.String()
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
