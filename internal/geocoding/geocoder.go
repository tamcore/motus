// Package geocoding provides reverse geocoding capabilities for converting
// GPS coordinates into human-readable addresses. It includes a thread-safe
// cache and rate-limited Nominatim client implementation.
package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Geocoder converts latitude/longitude coordinates into a human-readable address.
type Geocoder interface {
	// ReverseGeocode returns an address string for the given coordinates.
	// On failure, implementations should return a coordinate-based fallback
	// string rather than an error, to avoid blocking position processing.
	ReverseGeocode(ctx context.Context, lat, lon float64) (string, error)
}

// NominatimConfig holds configuration for the Nominatim geocoder.
type NominatimConfig struct {
	// URL is the base URL for the Nominatim reverse geocoding endpoint.
	// Default: "https://nominatim.openstreetmap.org/reverse"
	URL string

	// RateLimit is the maximum number of requests per second.
	// OSM Nominatim policy requires at most 1 req/sec.
	// Default: 1
	RateLimit float64

	// Timeout is the HTTP request timeout.
	// Default: 5s
	Timeout time.Duration

	// UserAgent is sent as the User-Agent header (required by OSM).
	// Default: "Motus GPS Tracker (https://github.com/tamcore/motus)"
	UserAgent string
}

// nominatimResponse is the JSON structure returned by Nominatim /reverse.
type nominatimResponse struct {
	DisplayName string `json:"display_name"`
	Error       string `json:"error"`
}

// NominatimGeocoder implements Geocoder using the Nominatim API.
type NominatimGeocoder struct {
	url       string
	client    *http.Client
	limiter   *rate.Limiter
	userAgent string
	logger    *slog.Logger
}

// NewNominatimGeocoder creates a new Nominatim-based reverse geocoder.
func NewNominatimGeocoder(cfg NominatimConfig) *NominatimGeocoder {
	if cfg.URL == "" {
		cfg.URL = "https://nominatim.openstreetmap.org/reverse"
	}
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = 1.0
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Motus GPS Tracker (https://github.com/tamcore/motus)"
	}

	return &NominatimGeocoder{
		url: cfg.URL,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		limiter:   rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
		userAgent: cfg.UserAgent,
		logger:    slog.Default(),
	}
}

// SetLogger configures the structured logger for the geocoder.
func (g *NominatimGeocoder) SetLogger(l *slog.Logger) {
	if l != nil {
		g.logger = l
	}
}

// ReverseGeocode queries the Nominatim API for the address at the given coordinates.
// It respects the configured rate limit and timeout. On any error, it returns a
// coordinate-based fallback string rather than propagating the error, so callers
// always get a usable address string.
func (g *NominatimGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	fallback := fmt.Sprintf("%.5f, %.5f", lat, lon)

	// Wait for rate limiter. If context is cancelled, return fallback.
	if err := g.limiter.Wait(ctx); err != nil {
		g.logger.Debug("geocoding rate limit wait cancelled",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		return fallback, fmt.Errorf("rate limit wait: %w", err)
	}

	reqURL := fmt.Sprintf("%s?lat=%.6f&lon=%.6f&format=json&zoom=18&addressdetails=0", g.url, lat, lon)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fallback, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		g.logger.Warn("geocoding request failed",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		return fallback, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("geocoding returned non-200 status",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Int("status", resp.StatusCode),
		)
		return fallback, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 64KB max
	if err != nil {
		return fallback, fmt.Errorf("read response: %w", err)
	}

	var result nominatimResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fallback, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != "" {
		g.logger.Debug("geocoding API error",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.String("error", result.Error),
		)
		return fallback, fmt.Errorf("nominatim error: %s", result.Error)
	}

	if result.DisplayName == "" {
		return fallback, nil
	}

	return result.DisplayName, nil
}

// coordinateFallback returns a human-readable coordinate string for use when
// geocoding is disabled or fails.
func coordinateFallback(lat, lon float64) string {
	return fmt.Sprintf("%.5f, %.5f", lat, lon)
}
