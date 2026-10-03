package geocoding

import (
	"cmp"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/ticker"
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

	queue   chan int64
	mu      sync.Mutex
	pending map[int64]request
	results map[int64]Resolved
}

type request struct {
	lat, lon float64
	at       time.Time
}

// Resolved is the latest background lookup result for a Prefetch key.
type Resolved struct {
	Lat, Lon    float64
	Address     string
	RequestedAt time.Time
}

// prefetchQueueSize bounds the number of keys waiting for a background lookup.
const prefetchQueueSize = 256

// NewCachedGeocoder creates a CachedGeocoder wrapping the given geocoder with
// the specified cache TTL. A nil logger means slog.Default().
func NewCachedGeocoder(geocoder Geocoder, cacheTTL time.Duration, logger *slog.Logger) *CachedGeocoder {
	return &CachedGeocoder{
		geocoder: geocoder,
		cache:    NewCache(cacheTTL),
		logger:   cmp.Or(logger, slog.Default()),
		queue:    make(chan int64, prefetchQueueSize),
		pending:  make(map[int64]request),
		results:  make(map[int64]Resolved),
	}
}

// Peek returns the cached address for the coordinates without geocoding.
func (cg *CachedGeocoder) Peek(lat, lon float64) (string, bool) {
	return cg.cache.Get(lat, lon)
}

// Prefetch requests a background lookup for key (e.g. a device ID) without
// blocking. Requests coalesce per key: only the latest coordinates are looked
// up, so a slow geocoder never builds a backlog of outdated points.
func (cg *CachedGeocoder) Prefetch(key int64, lat, lon float64) {
	cg.mu.Lock()
	_, queued := cg.pending[key]
	cg.pending[key] = request{lat, lon, time.Now()}
	cg.mu.Unlock()
	if queued {
		return
	}
	select {
	case cg.queue <- key:
	default:
		cg.mu.Lock()
		delete(cg.pending, key)
		cg.mu.Unlock()
		metrics.GeocodingPrefetchDropped.Inc()
	}
}

// Resolved returns the latest successful background lookup for key.
func (cg *CachedGeocoder) Resolved(key int64) (Resolved, bool) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	r, ok := cg.results[key]
	return r, ok
}

// StartPrefetch serves Prefetch requests until ctx is cancelled.
func (cg *CachedGeocoder) StartPrefetch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-cg.queue:
			cg.mu.Lock()
			req := cg.pending[key]
			delete(cg.pending, key)
			cg.mu.Unlock()
			if addr, ok := cg.lookup(ctx, req.lat, req.lon); ok {
				cg.mu.Lock()
				cg.results[key] = Resolved{req.lat, req.lon, addr, req.at}
				cg.mu.Unlock()
			}
		}
	}
}

// Lookup returns the address for the given coordinates, using the cache when
// possible. On a cache miss, it calls the underlying geocoder, caches the
// result, and returns it. If geocoding fails, the fallback coordinate string
// is returned but NOT cached (so subsequent requests will retry).
func (cg *CachedGeocoder) Lookup(ctx context.Context, lat, lon float64) string {
	addr, _ := cg.lookup(ctx, lat, lon)
	return addr
}

func (cg *CachedGeocoder) lookup(ctx context.Context, lat, lon float64) (string, bool) {
	if addr, ok := cg.cache.Get(lat, lon); ok {
		return addr, true
	}

	// Cache miss: call the geocoder.
	addr, err := cg.geocoder.ReverseGeocode(ctx, lat, lon)
	if err != nil {
		cg.logger.Debug("geocoding failed, using coordinate fallback",
			slog.Float64("lat", lat),
			slog.Float64("lon", lon),
			slog.Any("error", err),
		)
		// Do NOT cache the fallback so subsequent requests will retry.
		return coordinateFallback(lat, lon), false
	}

	cg.cache.Set(lat, lon, addr)
	return addr, true
}

// StartCleanup starts a background goroutine that periodically removes expired
// cache entries. It stops when the context is cancelled.
func (cg *CachedGeocoder) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker.Every(ctx, interval, func() {
		if removed := cg.cache.Cleanup(); removed > 0 {
			cg.logger.Debug("geocoding cache cleanup",
				slog.Int("removed", removed),
				slog.Int("remaining", cg.cache.Size()),
			)
		}
	})
}
