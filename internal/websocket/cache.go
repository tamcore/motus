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
// Cached slices are shared with callers and must not be mutated.
type deviceAccessCache struct {
	mu      sync.RWMutex
	entries map[int64]cacheEntry // deviceID -> cacheEntry
	now     func() time.Time     // injectable clock for testing
}

func newDeviceAccessCache() *deviceAccessCache {
	return &deviceAccessCache{entries: make(map[int64]cacheEntry), now: time.Now}
}

// get returns the cached user IDs for a device if the entry exists and has not
// expired. The second return value indicates whether a valid entry was found.
func (c *deviceAccessCache) get(deviceID int64) ([]int64, bool) {
	c.mu.RLock()
	entry, ok := c.entries[deviceID]
	c.mu.RUnlock()
	if !ok || c.now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.userIDs, true
}

// set stores user IDs for a device with defaultCacheTTL.
func (c *deviceAccessCache) set(deviceID int64, userIDs []int64) {
	c.mu.Lock()
	c.entries[deviceID] = cacheEntry{userIDs: userIDs, expiresAt: c.now().Add(defaultCacheTTL)}
	c.mu.Unlock()
}

// invalidate removes the cached entry for a specific device. Call this when
// user-device assignments change (assign or unassign).
func (c *deviceAccessCache) invalidate(deviceID int64) {
	c.mu.Lock()
	delete(c.entries, deviceID)
	c.mu.Unlock()
}
