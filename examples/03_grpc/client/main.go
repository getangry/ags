package main

import (
	"context"
	"log"
	"time"

	pb "github.com/getangry/ags/examples/03_grpc/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Set up connection to server
	conn, err := grpc.Dial("localhost:7841",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create client
	client := pb.NewUserServiceClient(conn)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a user
	user, err := client.CreateUser(ctx, &pb.CreateUserRequest{
		Name:  "Alice Smith",
		Email: "alice@example.com",
	})
	if err != nil {
		log.Fatalf("Could not create user: %v", err)
	}
	log.Printf("Created user: %v", user)

	// Get the user
	fetchedUser, err := client.GetUser(ctx, &pb.GetUserRequest{
		Id: user.Id,
	})
	if err != nil {
		log.Fatalf("Could not get user: %v", err)
	}
	log.Printf("Retrieved user: %v", fetchedUser)
}
