package main

import (
	"context"
	"log"
	"net/http"

	"github.com/getangry/ags"
	"github.com/getangry/ags/pkg/middleware"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	// Import the generated protobuf code
	pb "github.com/getangry/ags/examples/03_grpc/gen"
)

// UserService implements the UserService server interface
type UserService struct {
	pb.UnimplementedUserServiceServer
	log ags.Logger
}

// CreateUser implements the CreateUser RPC method
func (s *UserService) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.User, error) {
	// Generate a new UUID for the user
	id := uuid.New().String()

	// Create the user response
	user := &pb.User{
		Id:    id,
		Name:  req.GetName(),
		Email: req.GetEmail(),
	}

	// Log the operation
	s.log.Info("created user",
		"id", user.Id,
		"name", user.Name,
		"email", user.Email,
	)

	return user, nil
}

// GetUser implements the GetUser RPC method
func (s *UserService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	// In a real implementation, you would fetch from the database
	user := &pb.User{
		Id:    req.GetId(),
		Name:  "John Doe",
		Email: "john@example.com",
	}

	s.log.Info("retrieved user", "id", user.Id)
	return user, nil
}

// Logger implementation
type Logger struct{}

func (l *Logger) Info(msg string, fields ...interface{}) {
	log.Printf("INFO: %s %v", msg, fields)
}

func (l *Logger) Error(msg string, fields ...interface{}) {
	log.Printf("ERROR: %s %v", msg, fields)
}

// Authorizer implementation
type Authorizer struct{}

func (a *Authorizer) Authorize(ctx context.Context, r *http.Request) error {
	return nil
}

func main() {
	// Initialize logger
	logger := ags.NewDefaultLogger(ags.DebugLevel)

	// Create server configuration
	cfg := &ags.ServerConfig{
		Log:  logger,
		Auth: &Authorizer{},
	}

	// Create new handler
	handler := ags.NewHandler(cfg)

	// Create UserService implementation
	userService := &UserService{
		log: logger,
	}

	// Register the gRPC service implementation
	handler.RegisterGRPCService(&pb.UserService_ServiceDesc, userService)

	// Add a health check endpoint
	handler.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := ags.RespondJSON(w, http.StatusOK, "Service is healthy", nil); err != nil {
			logger.Error("failed to respond with JSON", "error", err)
		}
	})

	// Create the middleware chain
	wrapped := middleware.RequestID(handler)

	// Print registered routes for debugging
	handler.PrintRoutes()

	h2cHandler := allowH2C(wrapped)
	server := &http.Server{
		Addr:    ":7841",
		Handler: h2cHandler,
	}

	// Enable HTTP/2 support
	if err := http2.ConfigureServer(server, &http2.Server{}); err != nil {
		log.Fatalf("Failed to configure HTTP/2 server: %v", err)
	}

	// Start the server
	log.Println("Starting server on :7841...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// allowH2C enables H2C support
func allowH2C(h http.Handler) http.Handler {
	config := &http2.Server{}
	return h2c.NewHandler(h, config)
}
