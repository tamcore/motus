package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/websocket"
)

// allNopHandlers returns a Handlers struct with all fields set to nop handlers.
func allNopHandlers() Handlers {
	return Handlers{
		GetServer:                 nopHandler(),
		Login:                     nopHandler(),
		GetCurrentSession:         nopHandler(),
		GenerateToken:             nopHandler(),
		Logout:                    nopHandler(),
		ListDevices:               nopHandler(),
		GetDevice:                 nopHandler(),
		CreateDevice:              nopHandler(),
		UpdateDevice:              nopHandler(),
		DeleteDevice:              nopHandler(),
		GetPositions:              nopHandler(),
		CreateCommand:             nopHandler(),
		SendCommand:               nopHandler(),
		GetCmdTypes:               nopHandler(),
		ListCommands:              nopHandler(),
		ListGeofences:             nopHandler(),
		GetGeofence:               nopHandler(),
		CreateGeofence:            nopHandler(),
		UpdateGeofence:            nopHandler(),
		DeleteGeofence:            nopHandler(),
		ListEvents:                nopHandler(),
		ReportEvents:              nopHandler(),
		ListNotifications:         nopHandler(),
		CreateNotification:        nopHandler(),
		UpdateNotification:        nopHandler(),
		DeleteNotification:        nopHandler(),
		TestNotification:          nopHandler(),
		NotificationLogs:          nopHandler(),
		ImportGPX:                 nopHandler(),
		ListCalendars:             nopHandler(),
		CreateCalendar:            nopHandler(),
		UpdateCalendar:            nopHandler(),
		DeleteCalendar:            nopHandler(),
		CheckCalendar:             nopHandler(),
		CreateApiKey:              nopHandler(),
		ListApiKeys:               nopHandler(),
		DeleteApiKey:              nopHandler(),
		AdminListUserKeys:         nopHandler(),
		ListSessions:              nopHandler(),
		DeleteSession:             nopHandler(),
		AdminDeleteSession:        nopHandler(),
		ListUsers:                 nopHandler(),
		CreateUser:                nopHandler(),
		UpdateUser:                nopHandler(),
		DeleteUser:                nopHandler(),
		ListUserDevs:              nopHandler(),
		AdminListAllDevices:       nopHandler(),
		AssignDevice:              nopHandler(),
		UnassignDevice:            nopHandler(),
		AdminListAllGeofences:     nopHandler(),
		AdminListAllCalendars:     nopHandler(),
		AdminListAllNotifications: nopHandler(),
		AdminGetAllPositions:      nopHandler(),
		StartSudo:                 nopHandler(),
		EndSudo:                   nopHandler(),
		GetSudoStatus:             nopHandler(),
		CreateShare:               nopHandler(),
		ListShares:                nopHandler(),
		DeleteShare:               nopHandler(),
		GetSharedDevice:           nopHandler(),
		GetPlatformStats:          nopHandler(),
		GetUserStats:              nopHandler(),
		GetAuditLog:               nopHandler(),
	}
}

// passthroughMiddleware lets all requests through without modification.
func passthroughMiddleware(next http.Handler) http.Handler {
	return next
}

func TestNewRouter_HealthCheck(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub)

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

func TestNewRouter_LoginRouteIsPublic(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	loginCalled := false
	h := allNopHandlers()
	h.Login = func(w http.ResponseWriter, r *http.Request) {
		loginCalled = true
		w.WriteHeader(http.StatusOK)
	}

	// Auth middleware that blocks everything.
	blockAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusUnauthorized, "blocked")
		})
	}

	router := NewRouter(h, blockAuth, passthroughMiddleware, hub)

	// POST /api/session should bypass auth.
	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !loginCalled {
		t.Error("expected login handler to be called (public route)")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestNewRouter_ProtectedRouteRequiresAuth(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	blockAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusUnauthorized, "authentication required")
		})
	}

	router := NewRouter(h, blockAuth, passthroughMiddleware, hub)

	// GET /api/devices should be blocked by auth middleware.
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestNewRouter_AdminRouteRequiresAdmin(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	// Auth passes, admin blocks.
	blockAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusForbidden, "admin access required")
		})
	}

	router := NewRouter(h, passthroughMiddleware, blockAdmin, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestNewRouter_SudoStatusAccessibleWithoutAdmin(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	sudoStatusCalled := false
	h := allNopHandlers()
	h.GetSudoStatus = func(w http.ResponseWriter, r *http.Request) {
		sudoStatusCalled = true
		w.WriteHeader(http.StatusOK)
	}

	// Auth passes, admin blocks -- simulates a non-admin impersonated user.
	blockAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusForbidden, "admin access required")
		})
	}

	router := NewRouter(h, passthroughMiddleware, blockAdmin, hub)

	// GET /api/admin/sudo should be accessible without admin middleware.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !sudoStatusCalled {
		t.Error("expected GetSudoStatus handler to be called (not blocked by admin middleware)")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestNewRouter_EndSudoAccessibleWithoutAdmin(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	endSudoCalled := false
	h := allNopHandlers()
	h.EndSudo = func(w http.ResponseWriter, r *http.Request) {
		endSudoCalled = true
		w.WriteHeader(http.StatusOK)
	}

	// Auth passes, admin blocks -- simulates a non-admin impersonated user.
	blockAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusForbidden, "admin access required")
		})
	}

	router := NewRouter(h, passthroughMiddleware, blockAdmin, hub)

	// DELETE /api/admin/sudo should be accessible without admin middleware.
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !endSudoCalled {
		t.Error("expected EndSudo handler to be called (not blocked by admin middleware)")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestNewRouter_StartSudoRequiresAdmin(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	// Auth passes, admin blocks.
	blockAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusForbidden, "admin access required")
		})
	}

	router := NewRouter(h, passthroughMiddleware, blockAdmin, hub)

	// POST /api/admin/sudo/123 should still require admin.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/123", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403 (admin required), got %d", rr.Code)
	}
}

func TestNewRouter_SudoStatusRequiresAuth(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	// Auth blocks.
	blockAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			RespondError(w, http.StatusUnauthorized, "authentication required")
		})
	}

	router := NewRouter(h, blockAuth, passthroughMiddleware, hub)

	// GET /api/admin/sudo should still require authentication.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func nopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
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

func TestNewRouter_WithLoggerMiddleware(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	loggerCalled := false
	cfg := RouterConfig{
		Logger: flagMiddleware(&loggerCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !loggerCalled {
		t.Error("expected logger middleware to be called")
	}
}

func TestNewRouter_WithSecurityHeadersMiddleware(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	secHeadersCalled := false
	cfg := RouterConfig{
		SecurityHeaders: flagMiddleware(&secHeadersCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !secHeadersCalled {
		t.Error("expected security headers middleware to be called")
	}
}

func TestNewRouter_WithLoginRateLimit_GetSession(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	rateLimitCalled := false
	cfg := RouterConfig{
		LoginRateLimit: flagMiddleware(&rateLimitCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rateLimitCalled {
		t.Error("GET /api/session (session check) must not use login rate limit")
	}
}

func TestNewRouter_WithLoginRateLimit_PostSession(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	rateLimitCalled := false
	cfg := RouterConfig{
		LoginRateLimit: flagMiddleware(&rateLimitCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !rateLimitCalled {
		t.Error("expected login rate limit middleware to be called for POST /api/session")
	}
}

func TestNewRouter_WithAPIRateLimit(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	apiRateCalled := false
	cfg := RouterConfig{
		APIRateLimit: flagMiddleware(&apiRateCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !apiRateCalled {
		t.Error("expected API rate limit middleware to be called for authenticated route")
	}
}

func TestNewRouter_WithCSRFProtect(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	csrfCalled := false
	cfg := RouterConfig{
		CSRFProtect: flagMiddleware(&csrfCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !csrfCalled {
		t.Error("expected CSRF middleware to be called for authenticated route")
	}
}

func TestNewRouter_WithWriteAccess(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	writeAccessCalled := false
	cfg := RouterConfig{
		WriteAccess: flagMiddleware(&writeAccessCalled),
	}
	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !writeAccessCalled {
		t.Error("expected write access middleware to be called for authenticated route")
	}
}

func TestNewRouter_OIDCRoutes(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	configCalled, loginCalled, callbackCalled := false, false, false
	h.OIDCConfig = func(w http.ResponseWriter, r *http.Request) {
		configCalled = true
		w.WriteHeader(http.StatusOK)
	}
	h.OIDCLogin = func(w http.ResponseWriter, r *http.Request) {
		loginCalled = true
		w.WriteHeader(http.StatusOK)
	}
	h.OIDCCallback = func(w http.ResponseWriter, r *http.Request) {
		callbackCalled = true
		w.WriteHeader(http.StatusOK)
	}

	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub)

	for _, tc := range []struct {
		path   string
		called *bool
	}{
		{"/api/auth/oidc/config", &configCalled},
		{"/api/auth/oidc/login", &loginCalled},
		{"/api/auth/oidc/callback", &callbackCalled},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if !*tc.called {
			t.Errorf("expected OIDC handler for %s to be called", tc.path)
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", tc.path, rr.Code)
		}
	}
}

func TestNewRouter_MetricsSkippedForWebSocket(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	h := allNopHandlers()

	router := NewRouter(h, passthroughMiddleware, passthroughMiddleware, hub)

	// /api/socket requests skip metrics wrapping. The WebSocket upgrade will
	// fail (no Upgrade header) but the path through the metrics-bypass should
	// be executed without panic.
	req := httptest.NewRequest(http.MethodGet, "/api/socket", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// Any non-panic response is acceptable — just verify the path was reached.
}
