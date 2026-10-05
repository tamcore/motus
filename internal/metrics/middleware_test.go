package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetrics(t *testing.T) {
	handler := HTTPMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/devices", "/api/devices"},
		{"/api/devices/42", "/api/devices/{id}"},
		{"/api/users/1/devices/99", "/api/users/{id}/devices/{id}"},
		{"/api/health", "/api/health"},
		{"/api/share/abc123def", "/api/share/abc123def"},
		{"/api/x//007", "/api/x//{id}"},
	}

	for _, tt := range tests {
		got := normalizeEndpoint(tt.path)
		if got != tt.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestHTTPMetrics_RecordsStatus(t *testing.T) {
	tests := []struct {
		path    string
		handler http.HandlerFunc
		want    string
	}{
		{"/api/metrics-test/1", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, "404"},
		{"/api/metrics-test/2", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }, "200"},
		{"/api/metrics-test/3", func(http.ResponseWriter, *http.Request) {}, "200"},
	}
	for _, tt := range tests {
		counter := HTTPRequestsTotal.WithLabelValues(http.MethodGet, "/api/metrics-test/{id}", tt.want)
		before := testutil.ToFloat64(counter)
		HTTPMetrics(tt.handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))
		if got := testutil.ToFloat64(counter) - before; got != 1 {
			t.Errorf("%s: status %s counter delta = %v, want 1", tt.path, tt.want, got)
		}
	}
}
