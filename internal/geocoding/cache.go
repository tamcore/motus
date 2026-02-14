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
// Expiration is lazy: entries are checked on read and cleaned up periodically.
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
	// Round to 4 decimal places.
	rlat := math.Round(lat*10000) / 10000
	rlon := math.Round(lon*10000) / 10000
	return fmt.Sprintf("%.4f,%.4f", rlat, rlon)
}

// Get retrieves a cached address for the given coordinates.
// Returns the address and true if found and not expired, or empty string
// and false on cache miss or expiration.
func (c *Cache) Get(lat, lon float64) (string, bool) {
	key := cacheKey(lat, lon)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return "", false
	}

	// Lazy expiration: check if the entry has expired.
	if c.now().After(entry.expiresAt) {
		// Remove expired entry under write lock.
		c.mu.Lock()
		// Double-check: another goroutine may have already removed or renewed it.
		if e, ok := c.entries[key]; ok && c.now().After(e.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
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
