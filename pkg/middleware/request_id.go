package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"sync/atomic"
)

type ctxKeyRequestID string

const RequestIDKey ctxKeyRequestID = "req_id"

var (
	RequestIDHeader = "X-ReqId"
	prefix          string
	counterShards   = 64
	counters        [64]uint64
)

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}

	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("could not initialize request ID: %v", err))
	}

	b64 := base64.RawURLEncoding.EncodeToString(buf[:])[:10]

	prefix = fmt.Sprintf("%s/%s", hostname, b64)
}

// shardCounter returns a pointer to the counter shard based on a hash of the input string.
func shardCounter(key string) *uint64 {
	hash := fnv.New64a()
	hash.Write([]byte(key))
	return &counters[hash.Sum64()%uint64(counterShards)]
}

// nextRequestID generates the next request ID using a shard based on the remote address.
func nextRequestID(key string) uint64 {
	return atomic.AddUint64(shardCounter(key), 1)
}

// RequestID is a middleware that injects a request ID into the context of each request.
// If the header already exists, it appends the new ID using "/" as a separator.
func RequestID(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Use the remote address or another unique attribute of the request as a shard key
		newID := fmt.Sprintf("%s-%06d", prefix, nextRequestID(r.RemoteAddr))

		existingRequestID := r.Header.Get(RequestIDHeader)
		if existingRequestID != "" {
			// Append to the existing header
			newID = fmt.Sprintf("%s/%s", existingRequestID, newID)
		}

		r.Header.Set(RequestIDHeader, newID)

		// Add the final ID to the request context
		ctx = context.WithValue(ctx, RequestIDKey, newID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(fn)
}

// GetReqID retrieves the request ID from the context.
func GetReqID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}
