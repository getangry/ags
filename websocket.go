package ags

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket specific types
type WSHandleFunc func(*websocket.Conn)
type WSMiddlewareFunc func(WSHandleFunc) WSHandleFunc

// WebSocket connection wrapper
type WSConnection struct {
	*websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

// WebSocket configuration
type WSConfig struct {
	ReadBufferSize    int
	WriteBufferSize   int
	HandshakeTimeout  time.Duration
	EnableCompression bool
}

// WebSocket Handler implementation
type WebSocketHandler struct {
	upgrader   websocket.Upgrader
	routes     map[string]WSHandleFunc
	middleware []WSMiddlewareFunc
}

// Getter for WebSocketHandler routes
func (h *WebSocketHandler) GetRoutes() map[string]WSHandleFunc {
	return h.routes
}

func NewWebSocketHandler(config WSConfig) *WebSocketHandler {
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:    config.ReadBufferSize,
			WriteBufferSize:   config.WriteBufferSize,
			HandshakeTimeout:  config.HandshakeTimeout,
			EnableCompression: config.EnableCompression,
			CheckOrigin: func(r *http.Request) bool {
				return true // Override this in production
			},
		},
		routes:     make(map[string]WSHandleFunc),
		middleware: make([]WSMiddlewareFunc, 0),
	}
}

func (h *WebSocketHandler) DetectProtocol(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	handler, ok := h.routes[r.URL.Path]
	if !ok {
		http.Error(w, "WebSocket route not found", http.StatusNotFound)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Apply middleware chain
	handleFunc := handler
	for i := len(h.middleware) - 1; i >= 0; i-- {
		handleFunc = h.middleware[i](handleFunc)
	}

	// Create context for the connection
	ctx, cancel := context.WithCancel(r.Context())
	wsConn := &WSConnection{
		Conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	// Handle the WebSocket connection
	go func() {
		if wsConn != nil {
			defer wsConn.Conn.Close()
		}
		defer cancel()
		handleFunc(conn)
	}()
}

// RegisterWSRoute registers a WebSocket route with the handler
func (h *Handler) RegisterWSRoute(pattern string, handler WSHandleFunc) {
	h.wsHandler.routes[pattern] = handler
}

// Getter for Handler WebSocketHandler
func (h *Handler) GetWebSocketHandler() *WebSocketHandler {
	return h.wsHandler
}

// AddWSMiddleware adds middleware to the WebSocket handler
func (h *Handler) AddWSMiddleware(middleware WSMiddlewareFunc) {
	h.wsHandler.middleware = append(h.wsHandler.middleware, middleware)
}

func (h *Handler) StoreWSConnection(key string, conn *WSConnection) {
	h.wsConnections.Store(key, conn)
}

func (h *Handler) GetWSConnection(key string) (*WSConnection, bool) {
	if conn, ok := h.wsConnections.Load(key); ok {
		return conn.(*WSConnection), true
	}
	return nil, false
}

func (h *Handler) DeleteWSConnection(key string) {
	h.wsConnections.Delete(key)
}
