package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/api/middleware"
)

func setupRedisForRateLimit(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func redisOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRedisLoginRateLimit_BlocksAfterLimit(t *testing.T) {
	client := setupRedisForRateLimit(t)

	cfg := middleware.RateLimitConfig{Max: 3, Period: time.Minute}
	mw := middleware.NewRedisLoginRateLimit(client, cfg)
	handler := mw(redisOKHandler())

	ip := "10.0.0.1"
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
		req.RemoteAddr = ip + ":9999"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// 4th request must be rate-limited.
	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	req.RemoteAddr = ip + ":9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestRedisLoginRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	client := setupRedisForRateLimit(t)

	cfg := middleware.RateLimitConfig{Max: 2, Period: time.Minute}
	mw := middleware.NewRedisLoginRateLimit(client, cfg)
	handler := mw(redisOKHandler())

	// Exhaust ip1.
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
		req.RemoteAddr = "10.0.1.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// ip2 must still be allowed.
	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	req.RemoteAddr = "10.0.1.2:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ip2: expected 200, got %d", rr.Code)
	}
}

func TestRedisLoginRateLimit_SpoofedXForwardedForSharesBucket(t *testing.T) {
	client := setupRedisForRateLimit(t)
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	limiter := middleware.NewRedisLoginRateLimit(client, middleware.RateLimitConfig{Max: 1, Period: time.Minute})
	handler := middleware.RealIP(trusted)(limiter(redisOKHandler()))

	send := func(remoteAddr, xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
		req.RemoteAddr = remoteAddr
		req.Header.Set("X-Forwarded-For", xff)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := send("198.51.100.7:1234", "203.0.113.1"); code != http.StatusOK {
		t.Fatalf("first direct request: got %d, want 200", code)
	}
	if code := send("198.51.100.7:1234", "203.0.113.2"); code != http.StatusTooManyRequests {
		t.Errorf("spoofed X-Forwarded-For from untrusted peer: got %d, want 429", code)
	}
	if code := send("10.0.0.5:1234", "203.0.113.5"); code != http.StatusOK {
		t.Errorf("client behind trusted proxy: got %d, want 200", code)
	}
	if code := send("10.0.0.5:1234", "203.0.113.5"); code != http.StatusTooManyRequests {
		t.Errorf("same client behind trusted proxy: got %d, want 429", code)
	}
}
