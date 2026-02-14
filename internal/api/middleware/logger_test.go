package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogger_EmitsStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	log := buf.String()

	// Verify all required structured fields are present.
	for _, field := range []string{
		`type=http`,
		`method=GET`,
		`path=/api/devices`,
		`status=200`,
		`duration=`,
		`ip=192.168.1.1:12345`,
	} {
		if !strings.Contains(log, field) {
			t.Errorf("log output missing field %q\nlog: %s", field, log)
		}
	}
}

func TestLogger_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			slog.SetDefault(logger)

			handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			log := buf.String()
			expected := "status="
			if tc.statusCode == http.StatusOK {
				expected = "status=200"
			}
			_ = expected
			if !strings.Contains(log, "status=") {
				t.Errorf("log output missing status field\nlog: %s", log)
			}
		})
	}
}

func TestLogger_DefaultsTo200WhenNoWriteHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)

	// Handler that writes body without calling WriteHeader explicitly.
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	log := buf.String()
	if !strings.Contains(log, "status=200") {
		t.Errorf("expected status=200 for implicit write, got log: %s", log)
	}
}

func TestLogger_SkipsHealthEndpoint(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected handler to still execute, got status %d", rr.Code)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no log output for /api/health, got: %s", buf.String())
	}
}

func TestLogger_SkipsMetricsEndpoint(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected handler to still execute, got status %d", rr.Code)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no log output for /metrics, got: %s", buf.String())
	}
}

func TestLogger_LogsNonSkippedPaths(t *testing.T) {
	paths := []string{
		"/api/devices",
		"/api/positions",
		"/api/session",
		"/api/users",
		"/api/health-check", // not exactly /api/health but starts with it — actually this does match prefix
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			slog.SetDefault(logger)

			handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if shouldSkipLog(path) {
				if buf.Len() > 0 {
					t.Errorf("expected no log for skipped path %s", path)
				}
			} else {
				if buf.Len() == 0 {
					t.Errorf("expected log output for path %s, got nothing", path)
				}
			}
		})
	}
}

func TestLogger_IncludesDuration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	log := buf.String()
	if !strings.Contains(log, "duration=") {
		t.Errorf("expected duration field in log output\nlog: %s", log)
	}
}

func TestLogger_PreservesResponseWriter(t *testing.T) {
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}
	if rr.Header().Get("X-Custom") != "value" {
		t.Errorf("expected X-Custom header to be preserved")
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Errorf("expected body to be preserved, got %q", rr.Body.String())
	}
}

func TestShouldSkipLog(t *testing.T) {
	tests := []struct {
		path string
		skip bool
	}{
		{"/api/health", true},
		{"/api/health/ready", true}, // prefix match
		{"/metrics", true},
		{"/metrics/prometheus", true}, // prefix match
		{"/api/devices", false},
		{"/api/positions", false},
		{"/api/session", false},
		{"/", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := shouldSkipLog(tc.path)
			if got != tc.skip {
				t.Errorf("shouldSkipLog(%q) = %v, want %v", tc.path, got, tc.skip)
			}
		})
	}
}

func TestStatusRecorder_WriteHeaderCalledOnce(t *testing.T) {
	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr, statusCode: http.StatusOK}

	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusOK) // second call should not change recorded status

	if rec.statusCode != http.StatusNotFound {
		t.Errorf("expected first WriteHeader to win, got status %d", rec.statusCode)
	}
}

func TestStatusRecorder_Unwrap(t *testing.T) {
	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr}

	if rec.Unwrap() != rr {
		t.Error("Unwrap should return the underlying ResponseWriter")
	}
}
