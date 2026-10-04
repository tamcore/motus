package middleware

import (
	"cmp"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// skippedPaths lists URL path prefixes that are excluded from request
// logging because they generate high-frequency, low-value log entries.
var skippedPaths = []string{"/api/health", "/metrics", "/api/socket"}

// shouldSkipLog returns true if the request path matches any skipped prefix.
func shouldSkipLog(path string) bool {
	return slices.ContainsFunc(skippedPaths, func(prefix string) bool { return strings.HasPrefix(path, prefix) })
}

// Logger returns middleware that logs every HTTP request with structured
// fields using log/slog. Each log entry includes a "type"="http" field
// for easy filtering in log aggregation systems. Paths in skippedPaths are
// not logged.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipLog(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		slog.Info("http", //nolint:gosec // G706: user-supplied values are in structured slog fields, not interpolated into the message string
			slog.String("type", "http"),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", cmp.Or(ww.Status(), http.StatusOK)),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", r.RemoteAddr),
		)
	})
}
