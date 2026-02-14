package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// HTTPMetrics returns middleware that records HTTP request metrics.
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		endpoint := normalizeEndpoint(r.URL.Path)

		HTTPRequestsTotal.WithLabelValues(r.Method, endpoint, fmt.Sprintf("%d", rec.statusCode)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, endpoint).Observe(duration)
	})
}

// normalizeEndpoint replaces numeric path segments with {id} to reduce
// metric cardinality. For example, /api/devices/42 becomes /api/devices/{id}.
func normalizeEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if isNumeric(part) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
