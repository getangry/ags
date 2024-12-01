package ags

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/getangry/ags/pkg/cache"
	"github.com/getangry/ags/pkg/middleware"
	"github.com/gorilla/websocket"
)

// DebugConfig holds the configuration for debugging.
// It includes a mutex for thread-safe access, a boolean to enable or disable debugging,
// and an authentication key for secure operations.
type DebugConfig struct {
	mu          sync.RWMutex
	enableDebug bool
	authKey     string
}

// Authorizer is an interface that defines a method for authorizing HTTP requests.
// Implementations of this interface should provide the logic to authorize a request
// based on the provided context and HTTP request.
//
// Authorize method takes a context and an HTTP request as parameters and returns an error
// if the authorization fails. If the authorization is successful, it should return nil.
type Authorizer interface {
	Authorize(ctx context.Context, r *http.Request) error
}

// ServerConfig holds the configuration for the server, including
// database connection, caching, logging, authorization, and request
// processing phases.
//
// Fields:
// - DB: Database connection.
// - Cache: Caching mechanism.
// - Log: Logger for logging messages.
// - Auth: Authorization handler.
// - PrePhase: Functions to be executed before the main request processing.
// - PostPhase: Functions to be executed after the main request processing.
type ServerConfig struct {
	DB        *sql.DB
	Cache     cache.Cacher
	Log       Logger
	Auth      Authorizer
	PrePhase  []PreRequestFunc
	PostPhase []PostRequestFunc
}

// PreRequestFunc defines functions that run before request handling
type PreRequestFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error)

// PostRequestFunc defines functions that run after request handling
type PostRequestFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, duration time.Duration)

// ResponseWriter wraps http.ResponseWriter to capture response details
type ResponseWriter struct {
	http.ResponseWriter
	status    int
	size      int64
	committed bool
}

// StandardResponse represents our standard API response structure
type StandardResponse struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message"`
	Results interface{} `json:"results,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// RouteConfig represents the configuration for a single route
type RouteConfig struct {
	Methods []string
	Handler http.HandlerFunc
}

// Protocol Handler interfaces
type ProtocolHandler interface {
	DetectProtocol(r *http.Request) bool
	Handle(w http.ResponseWriter, r *http.Request)
}

// Handler is a struct that encapsulates the configuration and state for handling
// HTTP and WebSocket requests. It includes context, server configuration, route
// configurations, middleware, and various handlers for different protocols.
//
// Fields:
// - ctx: The context for managing request-scoped values, cancellation, and deadlines.
// - cfg: Pointer to the server configuration.
// - routes: A map of route configurations indexed by route names.
// - middleware: A slice of middleware functions to be applied to the routes.
// - routeOrder: A slice that tracks the order in which routes are registered.
// - fileServer: Configuration for the file server if one is registered.
// - staticHandler: The HTTP handler for serving static files if a file server is registered.
// - protocols: A slice of protocol handlers for handling different protocols.
// - grpcServer: The gRPC server instance.
// - wsHandler: The WebSocket handler for managing WebSocket connections.
// - wsConnections: A concurrent map for storing active WebSocket connections.
// - upgrader: The WebSocket upgrader for upgrading HTTP connections to WebSocket connections.
// - logger: The logger instance for logging messages and errors.
// - debug: Pointer to the debug configuration.
type Handler struct {
	ctx           context.Context
	cfg           *ServerConfig
	routes        map[string]RouteConfig
	middleware    []Middleware
	routeOrder    []string          // Tracks route registration order
	fileServer    *fileServerConfig // Store file server config if registered
	staticHandler http.Handler      // Store file server handler if registered
	protocols     []ProtocolHandler
	grpcServer    *grpc.Server
	wsHandler     *WebSocketHandler
	wsConnections sync.Map
	upgrader      websocket.Upgrader
	logger        Logger
	debug         *DebugConfig
}

// RouteInfo represents the information about a specific route in the application.
// It includes the URL pattern, the HTTP methods allowed, and the handler function name.
type RouteInfo struct {
	Pattern string
	Methods []string
	Handler string
}

// GetRegisteredRoutes returns all registered routes for debugging
func (h *Handler) GetRegisteredRoutes() []RouteInfo {
	routes := make([]RouteInfo, 0, len(h.routeOrder))

	// Add regular routes first
	for _, pattern := range h.routeOrder {
		route := h.routes[pattern]
		handler := runtime.FuncForPC(reflect.ValueOf(route.Handler).Pointer()).Name()
		routes = append(routes, RouteInfo{
			Pattern: pattern,
			Methods: route.Methods,
			Handler: handler,
		})
	}

	// Add WebSocket routes
	for pattern := range h.wsHandler.GetRoutes() {
		routes = append(routes, RouteInfo{
			Pattern: pattern,
			Methods: []string{"GET"},
			Handler: "WebSocket",
		})
	}

	// Add file server if registered
	if h.fileServer != nil {
		routes = append(routes, RouteInfo{
			Pattern: "/*",
			Methods: []string{"GET"},
			Handler: "FileServer(" + h.fileServer.indexFile + ")",
		})
	}

	return routes
}

// PrintRoutes prints all registered routes
func (h *Handler) PrintRoutes() {
	routes := h.GetRegisteredRoutes()
	fmt.Println("\nRegistered Routes:")
	fmt.Println("==================")
	for _, route := range routes {
		fmt.Printf("Pattern: %-20s Methods: %-20s Handler: %s\n",
			route.Pattern,
			strings.Join(route.Methods, ","),
			route.Handler,
		)
	}
	fmt.Println("==================")
}

// HTTP Methods
const (
	MethodGet     = "GET"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodDelete  = "DELETE"
	MethodPatch   = "PATCH"
	MethodHead    = "HEAD"
	MethodOptions = "OPTIONS"
	MethodTrace   = "TRACE"
	MethodConnect = "CONNECT"
)

// type AppOption func(*ServerConfig)
// type HandlerOption func(*Handler)

// NewHandler creates a new unified handler
func NewHandler(cfg *ServerConfig) *Handler {
	if cfg.Log == nil {
		cfg.Log = NewDefaultLogger(InfoLevel)
	}

	// Initialize PrePhase and PostPhase if they're nil
	if cfg.PrePhase == nil {
		cfg.PrePhase = make([]PreRequestFunc, 0)
	}

	if cfg.PostPhase == nil {
		cfg.PostPhase = make([]PostRequestFunc, 0)
	}

	h := &Handler{
		cfg:           cfg,
		routes:        make(map[string]RouteConfig),
		routeOrder:    make([]string, 0),
		protocols:     make([]ProtocolHandler, 0),
		middleware:    make([]Middleware, 0), // Initialize middleware slice
		wsConnections: sync.Map{},
		logger:        cfg.Log, // Store logger reference
		debug: &DebugConfig{
			authKey: os.Getenv("DEBUG_AUTH_KEY"), // Get auth key from environment
		}, upgrader: websocket.Upgrader{
			ReadBufferSize:    1024,
			WriteBufferSize:   1024,
			HandshakeTimeout:  10 * time.Second,
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	h.Post("/_/debug/toggle", h.authenticateDebug(h.handleDebugToggle))

	// Health check
	h.Get("/_/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Failed to write response: %v", err)
		}
	})

	// Initialize handlers and middleware as before...
	grpcHandler := NewGRPCHandler()
	h.grpcServer = grpcHandler.server
	h.protocols = append(h.protocols, grpcHandler)

	wsConfig := WSConfig{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
	}
	wsHandler := NewWebSocketHandler(wsConfig)
	h.wsHandler = wsHandler
	h.protocols = append(h.protocols, wsHandler)

	h.AddPreRequestFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error) {
		w.Header().Set("X-PreReq", "valid")
		return ctx, nil
	})

	return h
}

// handleDebugToggle handles enabling/disabling debug mode
func (h *Handler) handleDebugToggle(w http.ResponseWriter, r *http.Request) {
	// Simple struct for request body
	var req struct {
		Enable bool `json:"enable"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, NewError(ErrCodeBadRequest, "Invalid request body"))
		return
	}

	h.debug.mu.Lock()
	h.debug.enableDebug = req.Enable
	h.debug.mu.Unlock()

	if err := RespondJSON(w, http.StatusOK, "Debug mode updated", map[string]interface{}{
		"debug_enabled": req.Enable,
	}); err != nil {
		h.cfg.Log.Error("failed to respond with JSON", "error", err)
	}
}

// isDebugEnabled safely checks if debug mode is enabled
func (h *Handler) isDebugEnabled() bool {
	h.debug.mu.RLock()
	defer h.debug.mu.RUnlock()
	return h.debug.enableDebug
}

// Add helper method to get logger with request context
func (h *Handler) Log(ctx context.Context) Logger {
	return h.logger.WithContext(ctx)
}

// ServeHTTP implements the http.Handler interface
// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for protocol-specific handlers first
		for _, ph := range h.protocols {
			if ph.DetectProtocol(r) {
				ph.Handle(w, r)
				return
			}
		}

		// Try regular routes next
		if route, exists := h.routes[r.URL.Path]; exists {
			if !isMethodAllowed(r.Method, route.Methods) {
				h.handleMethodNotAllowed(w, r, route.Methods)
				return
			}
			route.Handler(w, r)
			return
		}

		// Static file handling
		if h.staticHandler != nil {
			if h.fileServer.serveSPA {
				fullPath := filepath.Join(h.fileServer.distPath, r.URL.Path)
				if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
					h.staticHandler.ServeHTTP(w, r)
					return
				}
				// Serve index.html for SPA routes
				indexPath := filepath.Join(h.fileServer.distPath, h.fileServer.indexFile)
				http.ServeFile(w, r, indexPath)
				return
			}
			h.staticHandler.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	// Apply global middleware in reverse order
	for i := len(h.middleware) - 1; i >= 0; i-- {
		handler = h.middleware[i](handler)
	}

	handler.ServeHTTP(w, r)
}

// isMethodAllowed checks if the request method is allowed
func isMethodAllowed(method string, allowedMethods []string) bool {
	for _, m := range allowedMethods {
		if method == m {
			return true
		}
	}
	return false
}

// Route registers a new HTTP route
func (h *Handler) Route(pattern string, handler http.HandlerFunc, methods ...string) {
	if len(methods) == 0 {
		methods = []string{MethodGet}
	}

	wrapped := h.wrapHandler(handler)
	h.routes[pattern] = RouteConfig{
		Methods: methods,
		Handler: wrapped,
	}
	h.routeOrder = append(h.routeOrder, pattern)
}

// HTTP Method-specific routing helpers
func (h *Handler) Get(pattern string, handler http.HandlerFunc) {
	h.Route(pattern, handler, MethodGet)
}

func (h *Handler) Post(pattern string, handler http.HandlerFunc) {
	h.Route(pattern, handler, MethodPost)
}

func (h *Handler) Put(pattern string, handler http.HandlerFunc) {
	h.Route(pattern, handler, MethodPut)
}

func (h *Handler) Delete(pattern string, handler http.HandlerFunc) {
	h.Route(pattern, handler, MethodDelete)
}

func (h *Handler) Patch(pattern string, handler http.HandlerFunc) {
	h.Route(pattern, handler, MethodPatch)
}

// wrapHandler wraps a standard http.HandlerFunc with our custom logic
func (h *Handler) wrapHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()
		logger := h.Log(ctx)

		// Debug request dump if enabled
		if h.isDebugEnabled() {
			reqDump, err := httputil.DumpRequest(r, true)
			if err != nil {
				logger.Error("failed to dump request", "error", err)
			} else {
				logger.Debug("request dump", "dump", string(reqDump))
			}
		}

		// Create a response writer that can capture the response
		rw := &debugResponseWriter{
			ResponseWriter: &ResponseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			},
			handler: h,
			request: r,
		}

		// Pre-request phase
		var err error
		for _, pre := range h.cfg.PrePhase {
			if ctx, err = pre(ctx, rw, r); err != nil {
				logger.Error("pre-request middleware error",
					"error", err.Error(),
					"middleware", fmt.Sprintf("%T", pre))
				h.Error(rw, err)
				return
			}
		}

		// Execute the main handler
		handler(rw, r.WithContext(ctx))

		// Post-request phase
		duration := time.Since(start)
		logger.Debug("request completed",
			"status", rw.status,
			"duration_ms", duration.Milliseconds(),
			"size", rw.size)

		for _, post := range h.cfg.PostPhase {
			post(ctx, rw, r, duration)
		}
	}
}

// Helper methods for ResponseWriter
func (w *ResponseWriter) WriteHeader(status int) {
	if !w.committed {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
		w.committed = true
	}
}

func (w *ResponseWriter) Write(b []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	return n, err
}

func (h *Handler) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowedMethods []string) {
	w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
	if err := RespondJSON(w, http.StatusMethodNotAllowed, "Method not allowed", map[string]interface{}{
		"allowed_methods": allowedMethods,
		"current_method":  r.Method,
	}); err != nil {
		h.cfg.Log.Error("failed to respond with JSON", "error", err)
	}
}

// RespondJSON sends a standardized JSON response
func RespondJSON(w http.ResponseWriter, status int, message string, data interface{}) error {
	response := StandardResponse{
		OK:      status >= 200 && status < 300,
		Message: message,
		Results: data,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(response)
}

func (h *Handler) AddPreRequestFunc(fn PreRequestFunc) {
	h.cfg.PrePhase = append(h.cfg.PrePhase, fn)
}

// Start begins serving the application.
//
// Usage:
//
//	app := NewApp(...)
//	if err := app.Start(); err != nil {
//		log.Fatalf("Failed to start server: %v", err)
//	}
func (a *Handler) Start() error {
	if a.ctx == nil {
		a.ctx = context.Background()
	}

	srv := &http.Server{
		Addr:    ":7841",
		Handler: middleware.RequestID(a),
		// ErrorLog: a.Logger.Logger(),
	}

	shutdownSignal := make(chan os.Signal, 1)
	serverShutdown := make(chan struct{})
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		select {
		case <-shutdownSignal:
			log.Println("Shutdown signal received, shutting down server...")
		case <-a.ctx.Done():
			log.Println("Context canceled, shutting down server...")
		}

		// Gracefully shutdown the server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		close(serverShutdown)
	}()

	log.Printf("Server starting on %s", ":7841")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	<-serverShutdown
	log.Println("Server stopped.")
	return nil
}

// Middleware represents a function that wraps an http.Handler
type Middleware func(http.Handler) http.Handler

// Group represents a group of routes with shared middleware and prefix
type Group struct {
	handler    *Handler
	prefix     string
	middleware []Middleware
}

// Use adds middleware to the group
func (h *Handler) Use(middleware ...Middleware) {
	h.middleware = append(h.middleware, middleware...)
}

// Group creates a new route group with the given prefix
func (h *Handler) Group(prefix string, mw ...Middleware) *Group {
	group := &Group{
		handler:    h,
		prefix:     prefix,
		middleware: make([]Middleware, 0),
	}

	if len(mw) > 0 {
		group.middleware = append(group.middleware, mw...)
	}

	return group
}

// Use adds middleware to the group
func (g *Group) Use(middleware ...Middleware) *Group {
	g.middleware = append(g.middleware, middleware...)
	return g
}

// Group creates a sub-group with an additional prefix
func (g *Group) Group(prefix string) *Group {
	return &Group{
		handler:    g.handler,
		prefix:     path.Join(g.prefix, prefix),
		middleware: append([]Middleware{}, g.middleware...), // Copy parent middleware
	}
}

// Route adds a route to the group with the complete middleware chain
func (g *Group) Route(pattern string, handler http.HandlerFunc, methods ...string) {
	fullPath := path.Join(g.prefix, pattern)

	// Create the complete middleware chain
	wrapped := handler

	// Apply group middleware in reverse order
	for i := len(g.middleware) - 1; i >= 0; i-- {
		wrapped = g.middleware[i](http.HandlerFunc(wrapped)).ServeHTTP
	}

	// Apply handler's internal wrapping last
	g.handler.Route(fullPath, wrapped, methods...)
}

// Add convenience methods for HTTP verbs
func (g *Group) Get(pattern string, handler http.HandlerFunc) {
	g.Route(pattern, handler, MethodGet)
}

func (g *Group) Post(pattern string, handler http.HandlerFunc) {
	g.Route(pattern, handler, MethodPost)
}

func (g *Group) Put(pattern string, handler http.HandlerFunc) {
	g.Route(pattern, handler, MethodPut)
}

func (g *Group) Delete(pattern string, handler http.HandlerFunc) {
	g.Route(pattern, handler, MethodDelete)
}

func (g *Group) Patch(pattern string, handler http.HandlerFunc) {
	g.Route(pattern, handler, MethodPatch)
}
