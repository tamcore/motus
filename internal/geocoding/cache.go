package geocoding

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// cacheEntry holds a cached address with its expiration time.
type cacheEntry struct {
	address   string
	expiresAt time.Time
}

// Cache is a thread-safe, TTL-based cache for geocoded addresses.
// Keys are lat/lon pairs rounded to 4 decimal places (~11m precision).
// Expired entries are ignored on read and removed by Cleanup.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	now     func() time.Time // injectable clock for testing
}

// NewCache creates a new address cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// cacheKey creates a cache key from lat/lon rounded to 4 decimal places.
// At the equator, 0.0001 degrees is about 11 meters, providing a reasonable
// balance between precision and cache reuse.
func cacheKey(lat, lon float64) string {
	return fmt.Sprintf("%.4f,%.4f", math.Round(lat*10000)/10000, math.Round(lon*10000)/10000)
}

// Get retrieves a cached address for the given coordinates.
// Returns the address and true if found and not expired, or empty string
// and false on cache miss or expiration.
func (c *Cache) Get(lat, lon float64) (string, bool) {
	key := cacheKey(lat, lon)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || c.now().After(entry.expiresAt) {
		return "", false
	}
	return entry.address, true
}

// Set stores an address in the cache for the given coordinates.
func (c *Cache) Set(lat, lon float64, address string) {
	key := cacheKey(lat, lon)

	c.mu.Lock()
	c.entries[key] = cacheEntry{
		address:   address,
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Size returns the current number of entries in the cache (including expired ones).
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Cleanup removes all expired entries from the cache. This can be called
// periodically to reclaim memory for long-running processes.
func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	removed := 0
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			removed++
		}
	}
	return removed
}
