package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/websocket"
)

// nopOASHandler satisfies oas.Handler for router-level tests.
// GetHealth returns a real 200 response; all other methods return ErrNotImplemented.
type nopOASHandler struct{ oas.UnimplementedHandler }

func (nopOASHandler) GetHealth(ctx context.Context) (*oas.GetHealthOK, error) {
	return &oas.GetHealthOK{Status: "ok"}, nil
}

// passthroughSecHandler satisfies oas.SecurityHandler, always granting access.
type passthroughSecHandler struct{}

func (passthroughSecHandler) HandleBearerAuth(ctx context.Context, _ oas.OperationName, _ oas.BearerAuth) (context.Context, error) {
	return ctx, nil
}
func (passthroughSecHandler) HandleCookieAuth(ctx context.Context, _ oas.OperationName, _ oas.CookieAuth) (context.Context, error) {
	return ctx, nil
}
func (passthroughSecHandler) HandleXAuthToken(ctx context.Context, _ oas.OperationName, _ oas.XAuthToken) (context.Context, error) {
	return ctx, nil
}


func newTestHub() *websocket.Hub {
	return websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
}

// flagMiddleware returns a middleware that sets a flag when invoked.
func flagMiddleware(called *bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*called = true
			next.ServeHTTP(w, r)
		})
	}
}

func TestNewRouter_HealthCheck(t *testing.T) {
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub())

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", body["status"])
	}
}

func TestNewRouter_DocsRoutes(t *testing.T) {
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub())

	cases := []struct {
		path        string
		contentType string
	}{
		{"/api/docs", "text/html; charset=utf-8"},
		{"/api/docs/openapi.yaml", "application/yaml"},
		{"/api/docs/scalar.js", "application/javascript; charset=utf-8"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", tc.path, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); ct != tc.contentType {
			t.Errorf("GET %s: expected Content-Type %q, got %q", tc.path, tc.contentType, ct)
		}
	}
}

func TestNewRouter_WithLoggerMiddleware(t *testing.T) {
	loggerCalled := false
	cfg := RouterConfig{Logger: flagMiddleware(&loggerCalled)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if !loggerCalled {
		t.Error("expected logger middleware to be called")
	}
}

func TestNewRouter_WithSecurityHeadersMiddleware(t *testing.T) {
	called := false
	cfg := RouterConfig{SecurityHeaders: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if !called {
		t.Error("expected security headers middleware to be called")
	}
}

func TestNewRouter_WithLoginRateLimit_GetSession(t *testing.T) {
	called := false
	cfg := RouterConfig{LoginRateLimit: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/session", nil))

	if called {
		t.Error("GET /api/session must not trigger the login rate limit")
	}
}

func TestNewRouter_WithLoginRateLimit_PostSession(t *testing.T) {
	called := false
	cfg := RouterConfig{LoginRateLimit: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/session", nil))

	if !called {
		t.Error("POST /api/session must trigger the login rate limit")
	}
}

func TestNewRouter_WithAPIRateLimit(t *testing.T) {
	called := false
	cfg := RouterConfig{APIRateLimit: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/devices", nil))

	if !called {
		t.Error("expected API rate limit middleware to be called")
	}
}

func TestNewRouter_WithCSRFProtect(t *testing.T) {
	called := false
	cfg := RouterConfig{CSRFProtect: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/devices", nil))

	if !called {
		t.Error("expected CSRF middleware to be called")
	}
}

func TestNewRouter_WithWriteAccess(t *testing.T) {
	called := false
	cfg := RouterConfig{WriteAccess: flagMiddleware(&called)}
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub(), cfg)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/devices", nil))

	if !called {
		t.Error("expected write access middleware to be called")
	}
}

func TestNewRouter_MetricsSkippedForWebSocket(t *testing.T) {
	router := NewRouter(nopOASHandler{}, passthroughSecHandler{}, newTestHub())

	req := httptest.NewRequest(http.MethodGet, "/api/socket", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// Any non-panic response is acceptable — the WebSocket upgrade will fail
	// (no Upgrade header) but the metrics-bypass path must execute without panic.
}
