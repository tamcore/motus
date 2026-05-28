package api

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/version"
	"github.com/tamcore/motus/internal/websocket"
	"github.com/tamcore/motus/web"
)

// maxRequestBodySize is the maximum allowed request body size (16 MB).
// Set to 16 MB to accommodate GPX file imports; all JSON endpoints use far less.
const maxRequestBodySize = 16 << 20

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		next.ServeHTTP(w, r)
	})
}

// RouterConfig holds optional middleware for the router.
type RouterConfig struct {
	// LoginRateLimit is applied only to POST /api/session.
	LoginRateLimit func(http.Handler) http.Handler
	// APIRateLimit is applied to all routes handled by the ogen server.
	APIRateLimit func(http.Handler) http.Handler
	// CSRFProtect enforces CSRF protection. POST /api/session is automatically exempt.
	CSRFProtect func(http.Handler) http.Handler
	// SecurityHeaders sets security-related response headers globally.
	SecurityHeaders func(http.Handler) http.Handler
	// Auth populates user/API-key context before WriteAccess runs.
	// Required when WriteAccess is set; ogen's SecurityHandler runs after WriteAccess.
	Auth func(http.Handler) http.Handler
	// WriteAccess enforces read-only restrictions for API keys with readonly permissions.
	WriteAccess func(http.Handler) http.Handler
	// Logger logs HTTP requests.
	Logger func(http.Handler) http.Handler
}

// injectResponseWriter stores w in the request context so ogen handlers can
// set cookies via ResponseWriterFromContext.
func injectResponseWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ContextWithResponseWriter(r.Context(), w))
		next.ServeHTTP(w, r)
	})
}

// exemptLoginFromCSRF marks POST /api/session as CSRF-exempt so the login
// endpoint can be reached before the client holds a CSRF token.
func exemptLoginFromCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/session" {
			r = csrf.UnsafeSkipCheck(r)
		}
		next.ServeHTTP(w, r)
	})
}

// csrfTokenMiddleware injects the X-CSRF-Token response header on any /api/session
// request so clients can obtain the token both on login and from an existing session.
func csrfTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session" {
			w.Header().Set("X-CSRF-Token", csrf.Token(r))
		}
		next.ServeHTTP(w, r)
	})
}

// serveDocs serves a single file from an embedded FS.
func serveDocs(f fs.FS, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(f, path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(path, ".yaml"):
			w.Header().Set("Content-Type", "application/yaml")
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
		_, _ = w.Write(b)
	}
}

// NewRouter creates the HTTP router with the ogen server mounted for all
// /api/ routes. Authentication is handled by sec (SecurityHandler) inside the
// ogen layer — the old authMiddleware and adminMiddleware chi parameters are
// replaced by the SecurityHandler.
func NewRouter(h oas.Handler, sec oas.SecurityHandler, hub *websocket.Hub, opts ...RouterConfig) http.Handler {
	var cfg RouterConfig
	if len(opts) > 0 {
		cfg = opts[0]
	}

	oasServer, err := oas.NewServer(h, sec)
	if err != nil {
		panic("failed to create ogen server: " + err.Error())
	}

	r := chi.NewRouter()

	if cfg.Logger != nil {
		r.Use(cfg.Logger)
	}
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(limitRequestBody)
	if cfg.SecurityHeaders != nil {
		r.Use(cfg.SecurityHeaders)
	}
	// Skip metrics wrapping for WebSocket — it wraps ResponseWriter, breaking http.Hijacker.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/socket" {
				next.ServeHTTP(w, r)
			} else {
				metrics.HTTPMetrics(next).ServeHTTP(w, r)
			}
		})
	})

	// Docs endpoints (public, no auth, no CSRF).
	r.Get("/api/docs/", http.RedirectHandler("/api/docs", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/api/docs", serveDocs(DocsFS, "docs/scalar.html"))
	r.Get("/api/docs/openapi.yaml", serveDocs(DocsFS, "docs/openapi.yaml"))
	r.Get("/api/docs/scalar.js", serveDocs(DocsFS, "docs/scalar.js"))

	// WebSocket (auth handled internally by hub).
	r.Get("/api/socket", hub.HandleConnect)

	// Build the ogen API handler with middleware applied. Execution order is
	// outermost-first: injectResponseWriter runs first, oasServer runs last.
	var apiHandler http.Handler = oasServer

	// Write access enforcement (innermost wrapper around ogen).
	if cfg.WriteAccess != nil {
		apiHandler = cfg.WriteAccess(apiHandler)
	}
	// Auth populates context with user/API key before WriteAccess can read it.
	if cfg.Auth != nil {
		apiHandler = cfg.Auth(apiHandler)
	}
	// General API rate limit.
	if cfg.APIRateLimit != nil {
		apiHandler = cfg.APIRateLimit(apiHandler)
	}
	// Inject X-CSRF-Token header into login responses.
	apiHandler = csrfTokenMiddleware(apiHandler)
	// CSRF protection (login already marked exempt by exemptLoginFromCSRF below).
	if cfg.CSRFProtect != nil {
		apiHandler = cfg.CSRFProtect(apiHandler)
	}
	// Mark POST /api/session CSRF-exempt before the CSRF check runs.
	apiHandler = exemptLoginFromCSRF(apiHandler)
	// Login rate limit applied only to POST /api/session.
	if cfg.LoginRateLimit != nil {
		loginRL := cfg.LoginRateLimit
		apiHandler = func(inner http.Handler) http.Handler {
			limited := loginRL(inner)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/session" {
					limited.ServeHTTP(w, r)
					return
				}
				inner.ServeHTTP(w, r)
			})
		}(apiHandler)
	}
	// Inject ResponseWriter into context (outermost layer, runs first).
	apiHandler = injectResponseWriter(apiHandler)

	// Mount ogen server for all /api/ routes. Use Handle (not Mount) so chi
	// does not strip the /api prefix — ogen's generated router expects the
	// full path including /api/.
	r.Handle("/api/*", apiHandler)

	// Serve embedded frontend static files if a build is present.
	webFS, err2 := fs.Sub(web.BuildFS, "build")
	if err2 == nil {
		if entries, _ := fs.ReadDir(webFS, "."); len(entries) > 1 || (len(entries) == 1 && entries[0].Name() != ".gitkeep") {
			indexHTML, _ := fs.ReadFile(webFS, "index.html")
			fileServer := http.FileServerFS(webFS)
			r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api") {
					http.NotFound(w, r)
					return
				}
				cleanPath := strings.TrimPrefix(r.URL.Path, "/")
				if cleanPath == "sw.js" {
					if body, ok := versionedSW(webFS); ok {
						w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
						_, _ = w.Write(body)
						return
					}
				}
				if cleanPath != "" {
					if _, err := fs.Stat(webFS, cleanPath); err == nil {
						fileServer.ServeHTTP(w, r)
						return
					}
				}
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
				if indexHTML != nil {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = w.Write(indexHTML)
					return
				}
				http.NotFound(w, r)
			})
		}
	}

	return r
}

var (
	swOnce  sync.Once
	swBytes []byte
)

func versionedSW(webFS fs.FS) ([]byte, bool) {
	swOnce.Do(func() {
		raw, err := fs.ReadFile(webFS, "sw.js")
		if err != nil {
			return
		}
		swBytes = bytes.ReplaceAll(raw, []byte("__CACHE_VERSION__"), []byte(version.Version))
	})
	return swBytes, len(swBytes) > 0
}
