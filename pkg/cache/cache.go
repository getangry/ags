package cache

import (
	"context"
)

// Cacher is an interface that defines methods for a cache system.
// It provides methods to set, get, and delete cache entries.
type Cacher interface {
	// Set stores a value in the cache with the specified key.
	Set(ctx context.Context, key string, value interface{})
	// Get retrieves a value from the cache using the specified key.
	// It returns the value and a boolean indicating whether the key was found.
	Get(ctx context.Context, key string) (interface{}, bool)
	// Delete removes a value from the cache using the specified key.
	Delete(ctx context.Context, key string)
}
