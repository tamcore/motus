package websocket

import (
	"sync"
	"time"
)

// defaultCacheTTL is how long cached user-device access entries remain valid.
// After this duration, the next access check will query the database again.
const defaultCacheTTL = 30 * time.Second

// cacheEntry holds a list of user IDs and the time at which the entry expires.
type cacheEntry struct {
	userIDs   []int64
	expiresAt time.Time
}

// deviceAccessCache is a thread-safe, TTL-based in-memory cache for
// device-to-user-ID mappings. It reduces database load by caching the result
// of DeviceAccessChecker.GetUserIDs, which is called on every WebSocket
// broadcast. Each pod maintains its own cache instance (no cross-pod sharing).
type deviceAccessCache struct {
	mu      sync.RWMutex
	entries map[int64]cacheEntry // deviceID -> cacheEntry
	ttl     time.Duration
	now     func() time.Time // injectable clock for testing
}

// newDeviceAccessCache creates a cache with the given TTL.
// If ttl is zero, defaultCacheTTL is used.
func newDeviceAccessCache(ttl time.Duration) *deviceAccessCache {
	if ttl == 0 {
		ttl = defaultCacheTTL
	}
	return &deviceAccessCache{
		entries: make(map[int64]cacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// get returns the cached user IDs for a device if the entry exists and has not
// expired. The second return value indicates whether a valid entry was found.
func (c *deviceAccessCache) get(deviceID int64) ([]int64, bool) {
	c.mu.RLock()
	entry, ok := c.entries[deviceID]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if c.now().After(entry.expiresAt) {
		// Entry has expired. Remove it lazily under a write lock.
		c.mu.Lock()
		// Re-check: another goroutine may have refreshed the entry.
		if e, still := c.entries[deviceID]; still && c.now().After(e.expiresAt) {
			delete(c.entries, deviceID)
		}
		c.mu.Unlock()
		return nil, false
	}

	// Return a copy to prevent callers from mutating the cached slice.
	result := make([]int64, len(entry.userIDs))
	copy(result, entry.userIDs)
	return result, true
}

// set stores user IDs for a device with the configured TTL.
func (c *deviceAccessCache) set(deviceID int64, userIDs []int64) {
	// Store a copy so the caller cannot mutate the cached data.
	stored := make([]int64, len(userIDs))
	copy(stored, userIDs)

	c.mu.Lock()
	c.entries[deviceID] = cacheEntry{
		userIDs:   stored,
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// invalidate removes the cached entry for a specific device. Call this when
// user-device assignments change (assign or unassign).
func (c *deviceAccessCache) invalidate(deviceID int64) {
	c.mu.Lock()
	delete(c.entries, deviceID)
	c.mu.Unlock()
}

// invalidateAll removes all cached entries. Useful for bulk operations.
func (c *deviceAccessCache) invalidateAll() {
	c.mu.Lock()
	c.entries = make(map[int64]cacheEntry)
	c.mu.Unlock()
}

// len returns the number of entries currently in the cache (including expired
// ones that have not yet been lazily evicted). Mainly useful for testing.
func (c *deviceAccessCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
