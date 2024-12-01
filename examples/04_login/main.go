package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"

	"github.com/getangry/ags"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// User represents our user model
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // Never send password in response
}

// LoginRequest represents login form data
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// AuthMiddleware checks for valid auth token

// AuthMiddleware creates a new authentication middleware using the provided database connection
func AuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if token == "" {
				if err := ags.RespondJSON(w, http.StatusUnauthorized, "No authorization token provided", nil); err != nil {
					log.Printf("Failed to respond with unauthorized error: %v", err)
				}
				return
			}

			// Check token in sessions table
			var username string
			err := db.QueryRow("SELECT username FROM sessions WHERE token = ? AND expires_at > datetime('now')", token).Scan(&username)
			if err != nil {
				if err == sql.ErrNoRows {
					ags.RespondJSON(w, http.StatusUnauthorized, "Invalid or expired token", nil)
					return
				}
				// Handle internal error
				appErr := ags.NewError(ags.ErrCodeInternal, "Database error").WithError(err)
				ags.RespondJSON(w, http.StatusInternalServerError, appErr.Message, nil)
				return
			}

			// Add username to context and continue
			ctx := context.WithValue(r.Context(), "username", username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUsername retrieves the username from the context
// Helper function to make it easier to get the username in handlers
func GetUsername(ctx context.Context) (string, bool) {
	username, ok := ctx.Value("username").(string)
	return username, ok
}

func main() {
	// Initialize SQLite database
	db, err := sql.Open("sqlite3", "./auth.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
            UNIQUE(username)
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY(username) REFERENCES users(username)
		);
        INSERT OR IGNORE INTO users (username, password) VALUES ('testuser', '$2a$10$qIxyTPvnSJK09QJn3kffz.xn8QwTqmVLQ9wX1qLimyQ2roHG5NagK');
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new logger
	logger := ags.NewDefaultLogger(ags.DebugLevel)

	// Initialize server config
	cfg := &ags.ServerConfig{
		DB:  db,
		Log: logger,
	}

	// Create handler
	h := ags.NewHandler(cfg)

	// Serve static files (HTML, CSS, JS)
	err = h.RegisterFileServer("./static", ags.WithSPASupport(true))
	if err != nil {
		log.Fatal(err)
	}

	// Login endpoint
	h.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeBadRequest, "Invalid request body"))
			return
		}

		// Get user from database
		var user User
		err := db.QueryRow("SELECT id, username, password FROM users WHERE username = ?",
			req.Username).Scan(&user.ID, &user.Username, &user.Password)

		if err == sql.ErrNoRows {
			h.Error(w, ags.NewError(ags.ErrCodeUnauthorized, "Invalid credentials"))
			return
		} else if err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeInternal, "Database error").WithError(err))
			return
		}

		// Check password
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeUnauthorized, "Invalid credentials"))
			return
		}

		// Generate token
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeInternal, "Token generation failed"))
			return
		}
		token := base64.URLEncoding.EncodeToString(tokenBytes)

		// Store token in database
		_, err = db.Exec(`
			INSERT INTO sessions (token, username, expires_at)
			VALUES (?, ?, datetime('now', '+24 hours'))`,
			token, user.Username)
		if err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeInternal, "Session creation failed"))
			return
		}

		// Send response
		if err := ags.RespondJSON(w, http.StatusOK, "Login successful", LoginResponse{
			Token:    token,
			Username: user.Username,
		}); err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeInternal, "Failed to respond with login success").WithError(err))
			return
		}
	})

	// Protected endpoint example
	protected := h.Group("/api")
	protected.Use(AuthMiddleware(db))

	// Add protected routes to the group
	protected.Get("/me", func(w http.ResponseWriter, r *http.Request) {
		username := r.Context().Value("username").(string)
		if err := ags.RespondJSON(w, http.StatusOK, "Profile retrieved", map[string]string{
			"username": username,
		}); err != nil {
			h.Error(w, ags.NewError(ags.ErrCodeInternal, "Failed to respond with profile").WithError(err))
			return
		}
	})
	if err := h.Start(); err != nil {
		log.Fatal(err)
	}
}
