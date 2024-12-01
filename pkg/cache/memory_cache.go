package cache

import (
	"context"
	"sync"
	"time"
)

type InMemoryCache struct {
	data        sync.Map
	ttl         time.Duration
	cleanupFreq time.Duration
	stopChan    chan struct{}
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// NewInMemoryCache creates a new cache with a given TTL
func NewInMemoryCache(ttl time.Duration, cleanupFreq time.Duration) *InMemoryCache {
	return &InMemoryCache{
		ttl:         ttl,
		cleanupFreq: cleanupFreq,
		stopChan:    make(chan struct{}),
	}
}

// Set stores a key-value pair in the cache
func (c *InMemoryCache) Set(ctx context.Context, key string, value interface{}) {
	expiry := time.Now().Add(c.ttl)
	c.data.Store(key, cacheEntry{value: value, expiresAt: expiry})
}

// Get retrieves a value from the cache and validates TTL
func (c *InMemoryCache) Get(ctx context.Context, key string) (interface{}, bool) {
	entry, ok := c.data.Load(key)
	if !ok {
		return nil, false
	}

	cacheEntry := entry.(cacheEntry)
	if time.Now().After(cacheEntry.expiresAt) {
		c.data.Delete(key)
		return nil, false
	}

	return cacheEntry.value, true
}

// Delete removes a key-value pair from the cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) {
	c.data.Delete(key)
}

// StartCleanup starts the periodic cleanup of expired cache entries
func (c *InMemoryCache) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupFreq)

	go func() {
		for {
			select {
			case <-ticker.C:
				c.purgeExpiredEntries()
			case <-ctx.Done():
				// Stop cleanup when context is canceled
				ticker.Stop()
				return
			case <-c.stopChan:
				// Stop cleanup when stop signal is received
				ticker.Stop()
				return
			}
		}
	}()
}

// StopCleanup sends a signal to stop the cleanup process
func (c *InMemoryCache) StopCleanup() {
	close(c.stopChan)
}

// purgeExpiredEntries removes all expired entries from the cache
func (c *InMemoryCache) purgeExpiredEntries() {
	now := time.Now()
	c.data.Range(func(key, value interface{}) bool {
		entry := value.(cacheEntry)
		if now.After(entry.expiresAt) {
			c.data.Delete(key)
		}
		return true
	})
}
