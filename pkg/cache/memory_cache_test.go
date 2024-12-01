package cache

import (
	"context"
	"testing"
	"time"
)

func TestStartCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := NewInMemoryCache(100*time.Millisecond, 50*time.Millisecond)
	cache.Set(ctx, "key1", "value1")
	cache.Set(ctx, "key2", "value2")

	// Start the cleanup process
	cache.StartCleanup(ctx)

	// Wait for entries to expire
	time.Sleep(200 * time.Millisecond)

	// Check if the entries are removed after TTL
	if _, found := cache.Get(ctx, "key1"); found {
		t.Errorf("Expected key1 to be expired and removed from cache")
	}
	if _, found := cache.Get(ctx, "key2"); found {
		t.Errorf("Expected key2 to be expired and removed from cache")
	}

	// Stop the cleanup process
	cache.StopCleanup()
}

func TestStopCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := NewInMemoryCache(100*time.Millisecond, 50*time.Millisecond)
	cache.Set(ctx, "key1", "value1")

	// Start the cleanup process
	cache.StartCleanup(ctx)

	// Stop the cleanup process immediately
	cache.StopCleanup()

	// Wait for entries to expire
	time.Sleep(200 * time.Millisecond)

	// Check if the entry is still present after stopping cleanup
	if _, found := cache.Get(ctx, "key1"); found {
		t.Errorf("Expected key1 to be expired and removed from cache")
	}
}
