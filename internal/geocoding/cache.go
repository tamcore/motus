package geocoding

import (
	"fmt"
	"math"
	"time"

	"github.com/tamcore/motus/internal/ttlcache"
)

// Cache is a thread-safe, TTL-based cache for geocoded addresses.
// Keys are lat/lon pairs rounded to 4 decimal places (~11m precision).
type Cache struct {
	*ttlcache.Cache[string, string]
}

// NewCache creates a new address cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttlcache.New[string, string](ttl)}
}

// cacheKey creates a cache key from lat/lon rounded to 4 decimal places.
// At the equator, 0.0001 degrees is about 11 meters, providing a reasonable
// balance between precision and cache reuse.
func cacheKey(lat, lon float64) string {
	return fmt.Sprintf("%.4f,%.4f", math.Round(lat*10000)/10000, math.Round(lon*10000)/10000)
}

// Get returns the cached address for the given coordinates if not expired.
func (c *Cache) Get(lat, lon float64) (string, bool) {
	return c.Cache.Get(cacheKey(lat, lon))
}

// Set stores an address in the cache for the given coordinates.
func (c *Cache) Set(lat, lon float64, address string) {
	c.Cache.Set(cacheKey(lat, lon), address)
}
