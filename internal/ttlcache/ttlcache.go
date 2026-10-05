// Package ttlcache provides a thread-safe in-memory map with per-entry expiry.
package ttlcache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a thread-safe TTL map. Expired entries are ignored on read and
// removed by Cleanup.
type Cache[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]entry[V]
	ttl     time.Duration
	Now     func() time.Time
}

// New creates a cache whose entries expire ttl after they are set.
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{entries: make(map[K]entry[V]), ttl: ttl, Now: time.Now}
}

// Get returns the value for key if present and not expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || c.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores value for key.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	c.entries[key] = entry[V]{value: value, expiresAt: c.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes key.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Size returns the number of entries, including expired ones.
func (c *Cache[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Cleanup removes expired entries and returns how many were removed.
func (c *Cache[K, V]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	removed := 0
	for key, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, key)
			removed++
		}
	}
	return removed
}
