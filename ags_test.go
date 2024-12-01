package ags_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getangry/ags"
	"github.com/getangry/ags/pkg/middleware"
	"github.com/gorilla/websocket"
	"gotest.tools/assert"
)

type mockLogger struct {
	lastError string
}

func (m *mockLogger) Error(msg string, args ...interface{}) {
	m.lastError = msg
}

func (m *mockLogger) Info(msg string, fields ...interface{})  {}
func (m *mockLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockLogger) GetLevel() ags.LogLevel {
	return ags.DebugLevel
}
func (m *mockLogger) SetLevel(level ags.LogLevel) {}
func (m *mockLogger) WithFields(fields map[string]interface{}) ags.Logger {
	return m
}
func (m *mockLogger) WithContext(ctx context.Context) ags.Logger {
	return m
}

type mockAuthorizer struct {
	shouldFail bool
}

func (m *mockAuthorizer) Authorize(ctx context.Context, r *http.Request) error {
	if m.shouldFail {
		return errors.New("unauthorized")
	}
	return nil
}

type mockCache struct{}

func (m *mockCache) Get(ctx context.Context, key string) (interface{}, bool) { return nil, false }
func (m *mockCache) Set(ctx context.Context, key string, value interface{})  {}
func (m *mockCache) Delete(ctx context.Context, key string)                  {}

// Helper function to create a new Handler
func newTestHandler() *ags.Handler {
	cfg := &ags.ServerConfig{
		DB:        &sql.DB{},
		Cache:     &mockCache{},
		Log:       &mockLogger{},
		Auth:      &mockAuthorizer{},
		PrePhase:  []ags.PreRequestFunc{},
		PostPhase: []ags.PostRequestFunc{},
	}
	return ags.NewHandler(cfg)
}

// Test ServeHTTP with a simple route
func TestHandler_ServeHTTP(t *testing.T) {
	h := newTestHandler()

	h.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		if err := ags.RespondJSON(w, http.StatusOK, "success", map[string]string{"key": "value"}); err != nil {
			t.Errorf("failed to respond with JSON: %v", err)
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, status)
	}

	var resp ags.StandardResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.OK || resp.Message != "success" || resp.Results.(map[string]interface{})["key"] != "value" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// Test handling of WebSocket upgrade
func TestWebSocketHandler_Upgrade(t *testing.T) {
	h := newTestHandler()

	// Register the WebSocket route with the main handler
	h.RegisterWSRoute("/ws", func(conn *websocket.Conn) {
		// Echo received messages
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	})

	s := httptest.NewServer(h)
	defer s.Close()

	// Connect to WebSocket
	url := "ws" + strings.TrimPrefix(s.URL, "http") + "/ws"
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect to WebSocket: %v", err)
	}
	defer wsConn.Close()

	// Test echo functionality
	err = wsConn.WriteMessage(websocket.TextMessage, []byte("test"))
	if err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	_, resp, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	if string(resp) != "test" {
		t.Errorf("unexpected response: got %s, want %s", resp, "test")
	}
}

// Test gRPC handler detection
func TestGRPCHandler_DetectProtocol(t *testing.T) {
	grpcHandler := ags.NewGRPCHandler()

	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.ProtoMajor = 2
	req.Header.Set("Content-Type", "application/grpc")

	if !grpcHandler.DetectProtocol(req) {
		t.Error("gRPC protocol not detected")
	}

	req.Header.Set("Content-Type", "application/json")
	if grpcHandler.DetectProtocol(req) {
		t.Error("false positive for gRPC protocol detection")
	}
}

// Test method not allowed handling
func TestHandler_MethodNotAllowed(t *testing.T) {
	h := newTestHandler()

	h.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("OK")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	req, _ := http.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, status)
	}

	expected := "Method not allowed"
	if !bytes.Contains(rr.Body.Bytes(), []byte(expected)) {
		t.Errorf("expected response to contain %q", expected)
	}
}

func TestRequestIDHeader(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() context.Context
		expectedHeader string
	}{
		{
			name: "without request ID in context",
			setupContext: func() context.Context {
				return context.Background()
			},
			expectedHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new handler with basic config
			h := ags.NewHandler(&ags.ServerConfig{})

			// Create a simple test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Register the test handler
			h.Get("/test", testHandler)

			// Create test request with the setup context
			req := httptest.NewRequest("GET", "/test", nil)
			req = req.WithContext(tt.setupContext())

			// Create response recorder
			rec := httptest.NewRecorder()

			// Serve the request
			h.ServeHTTP(rec, req)

			// Check response
			assert.Equal(t, http.StatusOK, rec.Code)

			// Check Request ID header
			actualHeader := rec.Header().Get(middleware.RequestIDHeader)
			assert.Equal(t, tt.expectedHeader, actualHeader,
				"Expected Request ID header to be %q, got %q",
				tt.expectedHeader,
				actualHeader)
		})
	}
}

func TestGroupRouting(t *testing.T) {
	tests := []struct {
		name           string
		setupRoutes    func(*ags.Handler)
		method         string
		path           string
		expectedStatus int
		expectedBody   string
		expectedHeader map[string]string
	}{
		{
			name: "basic group route",
			setupRoutes: func(h *ags.Handler) {
				api := h.Group("/api")
				api.Get("/users", func(w http.ResponseWriter, r *http.Request) {
					ags.RespondJSON(w, http.StatusOK, "users", nil)
				})
			},
			method:         "GET",
			path:           "/api/users",
			expectedStatus: http.StatusOK,
			expectedBody: `{"ok":true,"message":"users"}
`,
		},
		{
			name: "nested groups",
			setupRoutes: func(h *ags.Handler) {
				api := h.Group("/api")
				v1 := api.Group("/v1")
				v1.Get("/items", func(w http.ResponseWriter, r *http.Request) {
					ags.RespondJSON(w, http.StatusOK, "v1 items", nil)
				})
			},
			method:         "GET",
			path:           "/api/v1/items",
			expectedStatus: http.StatusOK,
			expectedBody: `{"ok":true,"message":"v1 items"}
`,
		},
		{
			name: "group with middleware",
			setupRoutes: func(h *ags.Handler) {
				api := h.Group("/api")
				api.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("X-Test", "middleware-ran")
						next.ServeHTTP(w, r)
					})
				})
				api.Get("/protected", func(w http.ResponseWriter, r *http.Request) {
					ags.RespondJSON(w, http.StatusOK, "protected", nil)
				})
			},
			method:         "GET",
			path:           "/api/protected",
			expectedStatus: http.StatusOK,
			expectedBody: `{"ok":true,"message":"protected"}
`,
			expectedHeader: map[string]string{
				"X-Test": "middleware-ran",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := ags.NewHandler(&ags.ServerConfig{})
			tt.setupRoutes(h)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rec.Body.String())
			}
			for k, v := range tt.expectedHeader {
				assert.Equal(t, v, rec.Header().Get(k))
			}
		})
	}
}

func TestMiddlewareOrder(t *testing.T) {
	h := ags.NewHandler(&ags.ServerConfig{})
	order := make([]string, 0)

	// Global middleware
	h.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "global")
			next.ServeHTTP(w, r)
		})
	})

	// Group middleware
	api := h.Group("/api")
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "group")
			next.ServeHTTP(w, r)
		})
	})

	// Nested group middleware
	v1 := api.Group("/v1")
	v1.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "nested")
			next.ServeHTTP(w, r)
		})
	})

	v1.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		ags.RespondJSON(w, http.StatusOK, "test", nil)
	})

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// Middleware should execute from outside in
	expected := []string{"global", "group", "nested", "handler"}
	assert.DeepEqual(t, expected, order)
}

func TestMultipleGroups(t *testing.T) {
	h := ags.NewHandler(&ags.ServerConfig{})

	// Create multiple groups with different middleware
	authMiddleware := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(fmt.Sprintf("X-Auth-%s", name), "true")
				next.ServeHTTP(w, r)
			})
		}
	}

	// Public API group
	public := h.Group("/public")
	public.Use(authMiddleware("public"))

	// Admin API group
	admin := h.Group("/admin")
	admin.Use(authMiddleware("admin"))

	// Add routes to both groups
	public.Get("/info", func(w http.ResponseWriter, r *http.Request) {
		ags.RespondJSON(w, http.StatusOK, "public info", nil)
	})

	admin.Get("/info", func(w http.ResponseWriter, r *http.Request) {
		ags.RespondJSON(w, http.StatusOK, "admin info", nil)
	})

	tests := []struct {
		name           string
		path           string
		expectedHeader string
		expectedBody   string
	}{
		{
			name:           "public route",
			path:           "/public/info",
			expectedHeader: "X-Auth-public",
			expectedBody: `{"ok":true,"message":"public info"}
`,
		},
		{
			name:           "admin route",
			path:           "/admin/info",
			expectedHeader: "X-Auth-admin",
			expectedBody: `{"ok":true,"message":"admin info"}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "true", rec.Header().Get(tt.expectedHeader))
			assert.Equal(t, tt.expectedBody, rec.Body.String())
		})
	}
}
