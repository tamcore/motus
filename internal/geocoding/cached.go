package geocoding

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/metrics"
)

// CachedGeocoder wraps a Geocoder with a TTL-based address cache.
// It provides two modes of operation:
//
//   - Lookup: Returns a cached address or blocks to geocode and cache it.
//     Used by the idle service for stopped positions.
//
//   - Peek + Prefetch: Non-blocking cache read, with misses geocoded in the
//     background by StartPrefetch. Used on the GPS ingest path.
type CachedGeocoder struct {
	geocoder Geocoder
	cache    *Cache
	logger   *slog.Logger
	queue    chan point
}

type point struct{ lat, lon float64 }

// prefetchQueueSize bounds pending background lookups; Prefetch drops beyond it.
const prefetchQueueSize = 256

// NewCachedGeocoder creates a CachedGeocoder wrapping the given geocoder with
// the specified cache TTL. A nil logger means slog.Default().
func NewCachedGeocoder(geocoder Geocoder, cacheTTL time.Duration, logger *slog.Logger) *CachedGeocoder {
	return &CachedGeocoder{
		geocoder: geocoder,
		cache:    NewCache(cacheTTL),
		logger:   cmp.Or(logger, slog.Default()),
		queue:    make(chan point, prefetchQueueSize),
	}
}

// Peek returns the cached address for the coordinates without geocoding.
func (cg *CachedGeocoder) Peek(lat, lon float64) (string, bool) {
	return cg.cache.Get(lat, lon)
}

// Prefetch queues a background lookup that fills the cache. It never blocks;
// when the queue is full the request is dropped.
func (cg *CachedGeocoder) Prefetch(lat, lon float64) {
	select {
	case cg.queue <- point{lat, lon}:
	default:
		metrics.GeocodingPrefetchDropped.Inc()
	}
}

// StartPrefetch serves Prefetch requests until ctx is cancelled.
func (cg *CachedGeocoder) StartPrefetch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-cg.queue:
			if _, ok := cg.cache.Get(p.lat, p.lon); !ok {
				cg.Lookup(ctx, p.lat, p.lon)
			}
		}
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
