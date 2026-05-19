// Package cache provides a generic key-value cache interface and an
// in-memory TTL-based implementation.
package cache

import (
	"sync"
	"time"
)

// Cache is a generic key-value store safe for concurrent use.
// The interface is intentionally minimal so alternative implementations
// (LRU, disk-backed, no-op for tests, …) can be swapped in freely.
type Cache[K comparable, V any] interface {
	// Get returns the value for key and whether it was present and unexpired.
	Get(key K) (V, bool)
	// Set stores value under key, resetting any existing TTL.
	Set(key K, value V)
	// Clear removes all entries from the cache.
	Clear()
}

// entry pairs a cached value with its expiry instant.
type entry[V any] struct {
	value  V
	expiry time.Time
}

// TTL is an in-memory [Cache] that expires entries after a fixed duration.
// Expired entries are evicted lazily on [TTL.Get]; the underlying map will
// grow without bound if keys are written but never re-read. For workloads
// with many unique short-lived keys, wrap TTL with periodic sweeps or use
// an LRU implementation instead.
//
// The zero value is not usable; construct with [NewTTL].
type TTL[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]entry[V]
	ttl   time.Duration
}

// NewTTL returns a [Cache] that evicts entries after ttl has elapsed since
// the last [TTL.Set] for that key.
func NewTTL[K comparable, V any](ttl time.Duration) *TTL[K, V] {
	return &TTL[K, V]{
		items: make(map[K]entry[V]),
		ttl:   ttl,
	}
}

// Get returns the cached value for key and true, or the zero value and false
// if the key is absent or its TTL has elapsed.
func (c *TTL[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiry) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores value under key and resets its TTL.
func (c *TTL[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{value: value, expiry: time.Now().Add(c.ttl)}
}

// Clear removes all entries from the cache.
func (c *TTL[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[K]entry[V])
}
