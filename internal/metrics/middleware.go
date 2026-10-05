package metrics

import (
	"cmp"
	"net/http"
	"strconv"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// HTTPMetrics returns middleware that records HTTP request metrics.
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		endpoint := normalizeEndpoint(r.URL.Path)

		HTTPRequestsTotal.WithLabelValues(r.Method, endpoint, strconv.Itoa(cmp.Or(ww.Status(), http.StatusOK))).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, endpoint).Observe(duration)
	})
}

// normalizeEndpoint replaces numeric path segments with {id} to reduce
// metric cardinality. For example, /api/devices/42 becomes /api/devices/{id}.
func normalizeEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part != "" && strings.Trim(part, "0123456789") == "" {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}
