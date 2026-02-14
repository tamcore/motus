package middleware

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the HTTP status code
// written by downstream handlers. It delegates all other ResponseWriter
// methods to the underlying writer, preserving Flush/Hijack support.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the status code and delegates to the underlying writer.
func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.written {
		sr.statusCode = code
		sr.written = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

// Write ensures the status code is recorded (defaults to 200) before writing.
func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.statusCode = http.StatusOK
		sr.written = true
	}
	return sr.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, enabling chi middleware
// (e.g., Recoverer) and net/http helpers to access the original writer
// for interface assertions like http.Flusher and http.Hijacker.
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

// Hijack implements http.Hijacker by delegating to the underlying writer.
// Required for WebSocket upgrades, which take over the raw TCP connection
// via a direct type assertion (w.(http.Hijacker)) that bypasses Unwrap().
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := sr.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

// skippedPaths lists URL path prefixes that are excluded from request
// logging because they generate high-frequency, low-value log entries.
var skippedPaths = []string{"/api/health", "/metrics", "/api/socket"}

// shouldSkipLog returns true if the request path matches any skipped prefix.
func shouldSkipLog(path string) bool {
	for _, prefix := range skippedPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// Logger returns middleware that logs every HTTP request with structured
// fields using log/slog. Each log entry includes a "type"="http" field
// for easy filtering in log aggregation systems.
//
// Requests to health check (/api/health) and metrics (/metrics) endpoints
// are silently skipped to reduce log noise.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipLog(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Info("http",
			slog.String("type", "http"),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.statusCode),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", r.RemoteAddr),
		)
	})
}
