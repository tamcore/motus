package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/version"
	"github.com/tamcore/motus/internal/websocket"
	"github.com/tamcore/motus/web"
)

// maxRequestBodySize is the maximum allowed request body size (16 MB).
// Set to 16 MB to accommodate GPX file imports; all JSON endpoints use far less.
const maxRequestBodySize = 16 << 20

// limitRequestBody restricts the size of incoming request bodies.
func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		next.ServeHTTP(w, r)
	})
}

// Handlers groups all HTTP handler components for route registration.
type Handlers struct {
	GetServer http.HandlerFunc

	Login             http.HandlerFunc
	GetCurrentSession http.HandlerFunc
	GenerateToken     http.HandlerFunc
	Logout            http.HandlerFunc

	ListDevices  http.HandlerFunc
	GetDevice    http.HandlerFunc
	CreateDevice http.HandlerFunc
	UpdateDevice http.HandlerFunc
	DeleteDevice http.HandlerFunc

	GetPositions http.HandlerFunc

	CreateCommand http.HandlerFunc
	SendCommand   http.HandlerFunc
	GetCmdTypes   http.HandlerFunc
	ListCommands  http.HandlerFunc

	ListGeofences  http.HandlerFunc
	GetGeofence    http.HandlerFunc
	CreateGeofence http.HandlerFunc
	UpdateGeofence http.HandlerFunc
	DeleteGeofence http.HandlerFunc

	ListEvents   http.HandlerFunc
	ReportEvents http.HandlerFunc

	ListNotifications  http.HandlerFunc
	CreateNotification http.HandlerFunc
	UpdateNotification http.HandlerFunc
	DeleteNotification http.HandlerFunc
	TestNotification   http.HandlerFunc
	NotificationLogs   http.HandlerFunc

	// GPX import.
	ImportGPX http.HandlerFunc

	// Device sharing.
	CreateShare     http.HandlerFunc
	ListShares      http.HandlerFunc
	DeleteShare     http.HandlerFunc
	GetSharedDevice http.HandlerFunc

	// Admin: User management.
	ListUsers           http.HandlerFunc
	CreateUser          http.HandlerFunc
	UpdateUser          http.HandlerFunc
	DeleteUser          http.HandlerFunc
	ListUserDevs        http.HandlerFunc
	AdminListAllDevices http.HandlerFunc
	AssignDevice        http.HandlerFunc
	UnassignDevice      http.HandlerFunc

	// Admin: All geofences, calendars, notifications, positions.
	AdminListAllGeofences     http.HandlerFunc
	AdminListAllCalendars     http.HandlerFunc
	AdminListAllNotifications http.HandlerFunc
	AdminGetAllPositions      http.HandlerFunc

	// Admin: Sudo/Impersonation.
	StartSudo     http.HandlerFunc
	EndSudo       http.HandlerFunc
	GetSudoStatus http.HandlerFunc

	// Admin: Statistics.
	GetPlatformStats http.HandlerFunc
	GetUserStats     http.HandlerFunc

	// Admin: Audit log.
	GetAuditLog http.HandlerFunc

	// Calendars.
	ListCalendars  http.HandlerFunc
	CreateCalendar http.HandlerFunc
	UpdateCalendar http.HandlerFunc
	DeleteCalendar http.HandlerFunc
	CheckCalendar  http.HandlerFunc

	// API key management.
	CreateApiKey      http.HandlerFunc
	ListApiKeys       http.HandlerFunc
	DeleteApiKey      http.HandlerFunc
	AdminListUserKeys http.HandlerFunc

	// Session management (list / revoke).
	ListSessions       http.HandlerFunc
	DeleteSession      http.HandlerFunc
	AdminDeleteSession http.HandlerFunc

	// OIDC authentication.
	OIDCConfig   http.HandlerFunc
	OIDCLogin    http.HandlerFunc
	OIDCCallback http.HandlerFunc
}

// RouterConfig holds optional middleware for the router.
type RouterConfig struct {
	// LoginRateLimit is applied to the login endpoint (POST /api/session).
	// If nil, no rate limiting is applied to login.
	LoginRateLimit func(http.Handler) http.Handler
	// APIRateLimit is applied to all authenticated API endpoints.
	// If nil, no rate limiting is applied to API routes.
	APIRateLimit func(http.Handler) http.Handler
	// CSRFProtect is applied globally to enforce CSRF protection on
	// state-changing requests. Bearer token requests are expected to be
	// exempt inside the middleware itself. If nil, no CSRF protection.
	CSRFProtect func(http.Handler) http.Handler
	// SecurityHeaders is applied globally to set security-related HTTP
	// response headers. If nil, no security headers are added.
	SecurityHeaders func(http.Handler) http.Handler
	// WriteAccess is applied to authenticated routes to enforce read-only
	// restrictions for API keys with readonly permissions.
	// If nil, no write access enforcement is applied.
	WriteAccess func(http.Handler) http.Handler
	// Logger is applied globally to log HTTP requests with structured fields.
	// If nil, no request logging is applied.
	Logger func(http.Handler) http.Handler
}

// NewRouter creates the HTTP router with all API routes.
func NewRouter(h Handlers, authMiddleware, adminMiddleware func(http.Handler) http.Handler, hub *websocket.Hub, opts ...RouterConfig) http.Handler {
	var cfg RouterConfig
	if len(opts) > 0 {
		cfg = opts[0]
	}

	r := chi.NewRouter()

	if cfg.Logger != nil {
		r.Use(cfg.Logger)
	}
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(limitRequestBody)
	// Security response headers (X-Content-Type-Options, X-Frame-Options, CSP, etc.).
	if cfg.SecurityHeaders != nil {
		r.Use(cfg.SecurityHeaders)
	}
	// Note: CSRF protection is applied only to authenticated routes below,
	// not globally, to allow public endpoints (login, share links) to work.
	// Skip metrics middleware for WebSocket (it wraps ResponseWriter, breaking http.Hijacker)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/socket" {
				next.ServeHTTP(w, r) // Skip metrics for WebSocket
			} else {
				metrics.HTTPMetrics(next).ServeHTTP(w, r)
			}
		})
	})

	// Health check (unauthenticated).
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Version endpoint (unauthenticated).
	r.Get("/api/version", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, version.Info())
	})

	// Public routes.
	r.Get("/api/server", h.GetServer)

	// Session check endpoint (public, NOT rate-limited).
	// This is called on every page load to validate the session cookie.
	// Rate-limiting this would lock users out after a few refreshes.
	r.Get("/api/session", h.GetCurrentSession)

	// Login endpoint with optional rate limiting and CSRF exemption.
	// CSRF is exempt because users cannot obtain a token before authenticating.
	// However, we still provide a CSRF token in the response for subsequent requests.
	exemptCSRF := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = csrf.UnsafeSkipCheck(r)
			// Inject CSRF token into response header for client to use
			w.Header().Set("X-CSRF-Token", csrf.Token(r))
			h.ServeHTTP(w, r)
		})
	}
	if cfg.LoginRateLimit != nil {
		r.Group(func(r chi.Router) {
			r.Use(cfg.LoginRateLimit)
			r.Method("POST", "/api/session", exemptCSRF(h.Login))
		})
	} else {
		r.Method("POST", "/api/session", exemptCSRF(h.Login))
	}
	r.Get("/api/share/{token}", h.GetSharedDevice)
	// WebSocket (auth handled internally)
	r.Get("/api/socket", hub.HandleConnect)

	// OIDC endpoints (public, CSRF-exempt — these are browser redirects).
	if h.OIDCConfig != nil {
		r.Get("/api/auth/oidc/config", h.OIDCConfig)
	}
	if h.OIDCLogin != nil {
		r.Get("/api/auth/oidc/login", h.OIDCLogin)
	}
	if h.OIDCCallback != nil {
		r.Get("/api/auth/oidc/callback", h.OIDCCallback)
	}

	// Protected routes.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		// CSRF protection for authenticated routes only.
		if cfg.CSRFProtect != nil {
			r.Use(cfg.CSRFProtect)
		}
		if cfg.APIRateLimit != nil {
			r.Use(cfg.APIRateLimit)
		}
		// Enforce read-only restrictions for API keys with readonly permissions.
		// Session routes are exempt (checked inside middleware).
		if cfg.WriteAccess != nil {
			r.Use(cfg.WriteAccess)
		}

		// Session management (authenticated).
		// Note: GET /api/session is public (above) to support ?token= auth
		r.Post("/api/session/token", h.GenerateToken)
		r.Delete("/api/session", h.Logout)

		// Devices.
		r.Get("/api/devices", h.ListDevices)
		r.Post("/api/devices", h.CreateDevice)
		r.Get("/api/devices/{id}", h.GetDevice)
		r.Put("/api/devices/{id}", h.UpdateDevice)
		r.Delete("/api/devices/{id}", h.DeleteDevice)

		// Positions.
		r.Get("/api/positions", h.GetPositions)

		// Commands.
		r.Get("/api/commands", h.ListCommands)
		r.Post("/api/commands", h.CreateCommand)
		r.Post("/api/commands/send", h.SendCommand)
		r.Get("/api/commands/types", h.GetCmdTypes)

		// Geofences.
		r.Get("/api/geofences", h.ListGeofences)
		r.Post("/api/geofences", h.CreateGeofence)
		r.Get("/api/geofences/{id}", h.GetGeofence)
		r.Put("/api/geofences/{id}", h.UpdateGeofence)
		r.Delete("/api/geofences/{id}", h.DeleteGeofence)

		// Calendars.
		r.Get("/api/calendars", h.ListCalendars)
		r.Post("/api/calendars", h.CreateCalendar)
		r.Put("/api/calendars/{id}", h.UpdateCalendar)
		r.Delete("/api/calendars/{id}", h.DeleteCalendar)
		r.Get("/api/calendars/{id}/check", h.CheckCalendar)

		// Events.
		r.Get("/api/events", h.ListEvents)

		// Reports.
		r.Get("/api/reports/events", h.ReportEvents)

		// Notifications.
		r.Get("/api/notifications", h.ListNotifications)
		r.Post("/api/notifications", h.CreateNotification)
		r.Put("/api/notifications/{id}", h.UpdateNotification)
		r.Delete("/api/notifications/{id}", h.DeleteNotification)
		r.Post("/api/notifications/{id}/test", h.TestNotification)
		r.Get("/api/notifications/{id}/logs", h.NotificationLogs)

		// GPX import.
		r.Post("/api/devices/{id}/gpx", h.ImportGPX)

		// Device sharing.
		r.Post("/api/devices/{id}/share", h.CreateShare)
		r.Get("/api/devices/{id}/shares", h.ListShares)
		r.Delete("/api/shares/{id}", h.DeleteShare)

		// API keys.
		r.Post("/api/keys", h.CreateApiKey)
		r.Get("/api/keys", h.ListApiKeys)
		r.Delete("/api/keys/{id}", h.DeleteApiKey)

		// Session management.
		r.Get("/api/sessions", h.ListSessions)
		r.Delete("/api/sessions/{id}", h.DeleteSession)

		// Sudo: status and end must be accessible by the impersonated user
		// (who may not be an admin), so they live outside the admin group.
		r.Get("/api/admin/sudo", h.GetSudoStatus)
		r.Delete("/api/admin/sudo", h.EndSudo)

		// WebSocket.
	})

	// Admin-only routes (auth + admin middleware).
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(adminMiddleware)

		r.Get("/api/users", h.ListUsers)
		r.Post("/api/users", h.CreateUser)
		r.Put("/api/users/{id}", h.UpdateUser)
		r.Delete("/api/users/{id}", h.DeleteUser)
		r.Get("/api/users/{id}/devices", h.ListUserDevs)
		r.Post("/api/users/{id}/devices/{deviceId}", h.AssignDevice)
		r.Delete("/api/users/{id}/devices/{deviceId}", h.UnassignDevice)

		// Admin: All devices.
		r.Get("/api/admin/devices", h.AdminListAllDevices)

		// Admin: All geofences, calendars, notifications, positions.
		r.Get("/api/admin/geofences", h.AdminListAllGeofences)
		r.Get("/api/admin/calendars", h.AdminListAllCalendars)
		r.Get("/api/admin/notifications", h.AdminListAllNotifications)
		r.Get("/api/admin/positions", h.AdminGetAllPositions)

		// Admin: API key management for other users.
		r.Get("/api/users/{id}/keys", h.AdminListUserKeys)

		// Admin: Session management for other users.
		r.Delete("/api/users/{id}/sessions/{sessionId}", h.AdminDeleteSession)

		// Admin: Sudo/Impersonation (start only; status/end are in the auth group above).
		r.Post("/api/admin/sudo/{id}", h.StartSudo)

		// Admin: Statistics.
		r.Get("/api/admin/statistics", h.GetPlatformStats)
		r.Get("/api/admin/statistics/users/{id}", h.GetUserStats)

		// Admin: Audit log.
		r.Get("/api/admin/audit", h.GetAuditLog)
	})

	// Serve embedded frontend static files if a build is present.
	webFS, err := fs.Sub(web.BuildFS, "build")
	if err == nil {
		if entries, _ := fs.ReadDir(webFS, "."); len(entries) > 1 || (len(entries) == 1 && entries[0].Name() != ".gitkeep") {
			// Read index.html once for SPA fallback (avoids http.FileServer redirect loops).
			indexHTML, _ := fs.ReadFile(webFS, "index.html")

			fileServer := http.FileServerFS(webFS)
			r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api") {
					http.NotFound(w, r)
					return
				}

				cleanPath := strings.TrimPrefix(r.URL.Path, "/")

				// Try exact path first (static assets like .js, .css, .png).
				if cleanPath != "" {
					if _, err := fs.Stat(webFS, cleanPath); err == nil {
						fileServer.ServeHTTP(w, r)
						return
					}
				}

				// SvelteKit adapter-static emits route.html files (e.g. login.html).
				if cleanPath != "" {
					if _, err := fs.Stat(webFS, cleanPath+".html"); err == nil {
						r.URL.Path = "/" + cleanPath + ".html"
						fileServer.ServeHTTP(w, r)
						return
					}
					if _, err := fs.Stat(webFS, cleanPath+"/index.html"); err == nil {
						r.URL.Path = "/" + cleanPath + "/index.html"
						fileServer.ServeHTTP(w, r)
						return
					}
				}

				// SPA fallback: write index.html directly to avoid
				// http.FileServer redirecting /index.html -> / in a loop.
				if indexHTML != nil {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write(indexHTML) //nolint:errcheck
					return
				}

				http.NotFound(w, r)
			})
		}
	}

	return r
}
