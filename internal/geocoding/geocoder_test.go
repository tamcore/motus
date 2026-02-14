package geocoding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNominatimGeocoder_ReverseGeocode_Success(t *testing.T) {
	// Set up a mock Nominatim server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify User-Agent header is sent.
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("expected User-Agent header")
		}

		// Verify query parameters.
		q := r.URL.Query()
		if q.Get("format") != "json" {
			t.Errorf("expected format=json, got %q", q.Get("format"))
		}
		if q.Get("lat") == "" || q.Get("lon") == "" {
			t.Error("expected lat and lon parameters")
		}

		resp := nominatimResponse{
			DisplayName: "123 Main Street, Springfield, IL, USA",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100, // high rate for testing
		Timeout:   5 * time.Second,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 39.7817, -89.6501)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "123 Main Street, Springfield, IL, USA" {
		t.Errorf("unexpected address: %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_EmptyDisplayName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := nominatimResponse{DisplayName: ""}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 0.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return coordinate fallback when display_name is empty.
	if addr != "0.00000, 0.00000" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := nominatimResponse{Error: "Unable to geocode"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 52.5200, 13.4050)
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	// Should return coordinate fallback.
	if addr != "52.52000, 13.40500" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 48.8566, 2.3522)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	// Should return coordinate fallback.
	if addr != "48.85660, 2.35220" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 51.5074, -0.1278)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if addr != "51.50740, -0.12780" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond) // exceed client timeout
		resp := nominatimResponse{DisplayName: "should not reach"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
		Timeout:   50 * time.Millisecond,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 40.7128, -74.0060)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if addr != "40.71280, -74.00600" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Second) // slow server
		resp := nominatimResponse{DisplayName: "should not reach"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	addr, err := geocoder.ReverseGeocode(ctx, 35.6762, 139.6503)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if addr != "35.67620, 139.65030" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_RateLimiting(t *testing.T) {
	var reqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqCount.Add(1)
		resp := nominatimResponse{DisplayName: "Test Address"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Rate limit: 5 req/sec.
	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       server.URL,
		RateLimit: 5,
	})

	// Fire 3 requests quickly (should succeed within ~600ms at 5 req/sec).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		_, err := geocoder.ReverseGeocode(ctx, float64(i), float64(i))
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	if reqCount.Load() != 3 {
		t.Errorf("expected 3 requests, got %d", reqCount.Load())
	}
}

func TestNominatimGeocoder_DefaultConfig(t *testing.T) {
	geocoder := NewNominatimGeocoder(NominatimConfig{})

	if geocoder.url != "https://nominatim.openstreetmap.org/reverse" {
		t.Errorf("unexpected default URL: %q", geocoder.url)
	}
	if geocoder.userAgent != "Motus GPS Tracker (https://github.com/tamcore/motus)" {
		t.Errorf("unexpected default User-Agent: %q", geocoder.userAgent)
	}
}

func TestNominatimGeocoder_ServerDown(t *testing.T) {
	// Use a URL that will fail to connect.
	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       "http://127.0.0.1:1", // nothing listens on port 1
		RateLimit: 100,
		Timeout:   100 * time.Millisecond,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 51.5074, -0.1278)
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if addr != "51.50740, -0.12780" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestCoordinateFallback(t *testing.T) {
	tests := []struct {
		lat, lon float64
		expected string
	}{
		{52.52000, 13.40500, "52.52000, 13.40500"},
		{-33.86880, 151.20930, "-33.86880, 151.20930"},
		{0.0, 0.0, "0.00000, 0.00000"},
		{90.0, 180.0, "90.00000, 180.00000"},
		{-90.0, -180.0, "-90.00000, -180.00000"},
	}

	for _, tt := range tests {
		result := coordinateFallback(tt.lat, tt.lon)
		if result != tt.expected {
			t.Errorf("coordinateFallback(%f, %f) = %q, want %q", tt.lat, tt.lon, result, tt.expected)
		}
	}
}

// errorBodyTransport returns a response with a body that immediately errors on Read.
type errorBodyTransport struct{}

func (e *errorBodyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	r, w := io.Pipe()
	_ = w.CloseWithError(errors.New("simulated read error"))
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       r,
	}, nil
}

func TestNominatimGeocoder_ReverseGeocode_InvalidURL(t *testing.T) {
	// A URL with a newline causes http.NewRequestWithContext to fail.
	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       "http://localhost\n", // newline in URL triggers NewRequestWithContext error
		RateLimit: 100,
	})

	addr, err := geocoder.ReverseGeocode(context.Background(), 51.0, 0.0)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if addr != "51.00000, 0.00000" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_ReverseGeocode_ReadBodyError(t *testing.T) {
	geocoder := NewNominatimGeocoder(NominatimConfig{
		URL:       "http://localhost",
		RateLimit: 100,
	})
	geocoder.client = &http.Client{Transport: &errorBodyTransport{}}

	addr, err := geocoder.ReverseGeocode(context.Background(), 51.0, 0.0)
	if err == nil {
		t.Fatal("expected error when reading response body fails")
	}
	if addr != "51.00000, 0.00000" {
		t.Errorf("expected coordinate fallback, got %q", addr)
	}
}

func TestNominatimGeocoder_SetLogger(t *testing.T) {
	g := NewNominatimGeocoder(NominatimConfig{URL: "http://localhost"})

	initial := g.logger
	g.SetLogger(nil) // nil should not change logger
	if g.logger != initial {
		t.Error("SetLogger(nil) should not change logger")
	}
	custom := slog.Default()
	g.SetLogger(custom)
	if g.logger != custom {
		t.Error("SetLogger(custom) should replace logger")
	}
}
