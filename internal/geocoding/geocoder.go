// Package geocoding provides reverse geocoding capabilities for converting
// GPS coordinates into human-readable addresses. It includes a thread-safe
// cache and rate-limited Nominatim client implementation.
package geocoding

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/tamcore/motus/internal/metrics"
)

// Geocoder converts latitude/longitude coordinates into a human-readable address.
type Geocoder interface {
	// ReverseGeocode returns an address string for the given coordinates.
	// On failure it returns "" and an error; callers pick a fallback.
	ReverseGeocode(ctx context.Context, lat, lon float64) (string, error)
}

// ForwardGeocoder converts a free-text address query into coordinates.
type ForwardGeocoder interface {
	ForwardGeocode(ctx context.Context, query string) (lat, lon float64, displayName string, err error)
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

	// Logger receives geocoder logs. Default: slog.Default()
	Logger *slog.Logger

	// Limiter overrides the per-process RateLimit limiter, e.g. with a
	// RedisLimiter shared by all pods.
	Limiter Limiter
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
	limiter   Limiter
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
		limiter:   cmp.Or[Limiter](cfg.Limiter, rate.NewLimiter(rate.Limit(cfg.RateLimit), 1)),
		userAgent: cfg.UserAgent,
		logger:    cmp.Or(cfg.Logger, slog.Default()),
	}
}

// ReverseGeocode queries the Nominatim API for the address at the given coordinates.
// It respects the configured rate limit and timeout. An empty display name
// yields the coordinate fallback string.
func (g *NominatimGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		g.logger.Debug("geocoding rate limit wait cancelled",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		return "", fmt.Errorf("rate limit wait: %w", err)
	}

	addr, err := g.reverseGeocode(ctx, lat, lon)
	result := "ok"
	if err != nil {
		result = "error"
	}
	metrics.GeocodingRequests.WithLabelValues(result).Inc()
	return addr, err
}

func (g *NominatimGeocoder) reverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	reqURL := fmt.Sprintf("%s?lat=%.6f&lon=%.6f&format=json&zoom=18&addressdetails=0", g.url, lat, lon)
	var result nominatimResponse
	if err := g.getJSON(ctx, reqURL, &result); err != nil {
		g.logger.Warn("geocoding request failed",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		return "", err
	}

	if result.Error != "" {
		g.logger.Debug("geocoding API error",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.String("error", result.Error),
		)
		return "", fmt.Errorf("nominatim error: %s", result.Error)
	}

	if result.DisplayName == "" {
		return coordinateFallback(lat, lon), nil
	}
	return result.DisplayName, nil
}

// ForwardGeocode queries Nominatim /search for the given address string and
// returns the top-ranked result's coordinates and display name.
func (g *NominatimGeocoder) ForwardGeocode(ctx context.Context, query string) (lat, lon float64, displayName string, err error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return 0, 0, "", fmt.Errorf("rate limit wait: %w", err)
	}

	// Derive the search base: replace trailing "/reverse" with "/search".
	searchBase := strings.TrimSuffix(g.url, "/reverse")
	reqURL := fmt.Sprintf("%s/search?q=%s&format=jsonv2&limit=1",
		searchBase, url.QueryEscape(query))

	var results []struct {
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		DisplayName string `json:"display_name"`
	}
	if err := g.getJSON(ctx, reqURL, &results); err != nil {
		return 0, 0, "", err
	}
	if len(results) == 0 {
		return 0, 0, "", fmt.Errorf("no results for %q", query)
	}

	latF, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse lat: %w", err)
	}
	lonF, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse lon: %w", err)
	}
	return latF, lonF, results[0].DisplayName, nil
}

// getJSON GETs reqURL and decodes the JSON body (at most 64 KiB) into v.
func (g *NominatimGeocoder) getJSON(ctx context.Context, reqURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

// coordinateFallback returns a human-readable coordinate string for use when
// geocoding is disabled or fails.
func coordinateFallback(lat, lon float64) string {
	return fmt.Sprintf("%.5f, %.5f", lat, lon)
}
