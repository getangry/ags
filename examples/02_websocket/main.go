package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/getangry/ags"
	"github.com/gorilla/websocket"
)

//go:embed dist
var fs embed.FS

type Message struct {
	Type    string `json:"type"`
	User    string `json:"user"`
	Content string `json:"content"`
}

type ChatServer struct {
	clients   sync.Map // map[*websocket.Conn]string
	broadcast chan Message
	userCount int
	handler   *ags.Handler
	mu        sync.Mutex
}

func NewChatServer() *ChatServer {
	return &ChatServer{
		broadcast: make(chan Message),
	}
}

func (cs *ChatServer) handleWebSocket(conn *websocket.Conn) {
	// Generate user name
	cs.mu.Lock()
	cs.userCount++
	username := fmt.Sprintf("User%d", cs.userCount)
	cs.mu.Unlock()

	// Store client connection
	cs.clients.Store(conn, username)

	// Announce new user
	cs.broadcast <- Message{
		Type:    "system",
		Content: username + " has joined the chat",
	}

	// Handle incoming messages
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("error reading message: %v", err)
			break
		}

		msg.User = username
		cs.broadcast <- msg
	}

	// Clean up on disconnect
	cs.clients.Delete(conn)
	cs.broadcast <- Message{
		Type:    "system",
		Content: username + " has left the chat",
	}
}

func (cs *ChatServer) broadcastMessages() {
	for msg := range cs.broadcast {
		cs.clients.Range(func(key, value interface{}) bool {
			conn := key.(*websocket.Conn)
			err := conn.WriteJSON(msg)
			if err != nil {
				log.Printf("error sending message: %v", err)
				conn.Close()
				cs.clients.Delete(conn)
			}
			return true
		})
	}
}

func main() {
	// Create chat server
	chatServer := NewChatServer()
	go chatServer.broadcastMessages()

	// Create server config
	cfg := &ags.ServerConfig{
		DB:   &sql.DB{}, // Add your DB configuration
		Log:  createLogger(),
		Auth: createAuthorizer(),
	}

	// Create handler
	handler := ags.NewHandler(cfg)

	// Register WebSocket route
	handler.RegisterWSRoute("/chat", chatServer.handleWebSocket)

	// Register HTTP routes
	// handler.Get("/", func(w http.ResponseWriter, r *http.Request) {
	// 	ags.RespondJSON(w, http.StatusOK, "Chat server running", nil)
	// })

	// Register file server last
	handler.RegisterFileServer("./dist",
		ags.WithSPASupport(true),
		ags.WithIndexFile("index.html"),
	)

	// Print routes for debugging
	handler.PrintRoutes()

	// Start server
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}

func createLogger() ags.Logger {
	return &SimpleLogger{}
}

type SimpleLogger struct {
	ctx   context.Context
	level ags.LogLevel
	data  map[string]interface{}
}

func (l *SimpleLogger) Info(msg string, fields ...interface{}) { log.Printf("[INFO] "+msg, fields...) }
func (l *SimpleLogger) Warn(msg string, fields ...interface{}) { log.Printf("[WARN] "+msg, fields...) }
func (l *SimpleLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] "+msg, fields...)
}
func (l *SimpleLogger) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] "+msg, fields...)
}
func (l *SimpleLogger) WithFields(fields map[string]interface{}) ags.Logger {
	l.data = fields
	return l
}
func (l *SimpleLogger) WithContext(ctx context.Context) ags.Logger {
	l.ctx = ctx
	return l
}
func (l *SimpleLogger) SetLevel(level ags.LogLevel) {
	l.level = level
}
func (l *SimpleLogger) GetLevel() ags.LogLevel {
	return l.level
}

func createAuthorizer() ags.Authorizer {
	return &SimpleAuthorizer{}
}

type SimpleAuthorizer struct{}

func (a *SimpleAuthorizer) Authorize(ctx context.Context, r *http.Request) error {
	return nil // Allow all connections in this example
}
