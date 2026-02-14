package geocoding

import (
	"context"
	"log/slog"
	"time"
)

// CachedGeocoder wraps a Geocoder with a TTL-based address cache.
// It provides two modes of operation:
//
//   - Lookup: Returns a cached address or performs geocoding and caches the result.
//     Used for live tracking where the address is set on the API response but
//     not persisted to the database.
//
//   - LookupAndStore: Same as Lookup, but intended for idle/stopped positions
//     where the address should be persisted in the database.
type CachedGeocoder struct {
	geocoder Geocoder
	cache    *Cache
	logger   *slog.Logger
}

// NewCachedGeocoder creates a CachedGeocoder wrapping the given geocoder with
// the specified cache TTL.
func NewCachedGeocoder(geocoder Geocoder, cacheTTL time.Duration) *CachedGeocoder {
	return &CachedGeocoder{
		geocoder: geocoder,
		cache:    NewCache(cacheTTL),
		logger:   slog.Default(),
	}
}

// SetLogger configures the structured logger.
func (cg *CachedGeocoder) SetLogger(l *slog.Logger) {
	if l != nil {
		cg.logger = l
	}
}

// Lookup returns the address for the given coordinates, using the cache when
// possible. On a cache miss, it calls the underlying geocoder, caches the
// result, and returns it. If geocoding fails, the fallback coordinate string
// is returned but NOT cached (so subsequent requests will retry).
func (cg *CachedGeocoder) Lookup(ctx context.Context, lat, lon float64) string {
	// Check cache first.
	if addr, ok := cg.cache.Get(lat, lon); ok {
		return addr
	}

	// Cache miss: call the geocoder.
	addr, err := cg.geocoder.ReverseGeocode(ctx, lat, lon)
	if err != nil {
		cg.logger.Debug("geocoding failed, using coordinate fallback",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		// Return the fallback (which ReverseGeocode already provides) but
		// do NOT cache it so subsequent requests will retry.
		return coordinateFallback(lat, lon)
	}

	// Cache the result.
	cg.cache.Set(lat, lon, addr)
	return addr
}

// Cache returns the underlying cache for inspection or cleanup.
func (cg *CachedGeocoder) Cache() *Cache {
	return cg.cache
}

// StartCleanup starts a background goroutine that periodically removes expired
// cache entries. It stops when the context is cancelled.
func (cg *CachedGeocoder) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := cg.cache.Cleanup()
			if removed > 0 {
				cg.logger.Debug("geocoding cache cleanup",
					slog.Int("removed", removed),
					slog.Int("remaining", cg.cache.Size()),
				)
			}
		}
	}
}
