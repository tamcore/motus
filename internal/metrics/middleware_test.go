package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
	}

	for _, tt := range tests {
		got := normalizeEndpoint(tt.path)
		if got != tt.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestStatusRecorderDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, statusCode: http.StatusOK}

	// Write without calling WriteHeader should keep default.
	_, _ = sr.Write([]byte("test"))
	if sr.statusCode != http.StatusOK {
		t.Errorf("expected default status 200, got %d", sr.statusCode)
	}
}

func TestStatusRecorderExplicit(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, statusCode: http.StatusOK}

	sr.WriteHeader(http.StatusNotFound)
	if sr.statusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", sr.statusCode)
	}
}
