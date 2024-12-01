package ags

import (
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// gRPC Handler implementation
type GRPCHandler struct {
	server *grpc.Server
}

func NewGRPCHandler(opts ...grpc.ServerOption) *GRPCHandler {
	server := grpc.NewServer(opts...)
	reflection.Register(server) // Enable reflection for debugging
	return &GRPCHandler{
		server: server,
	}
}

func (h *GRPCHandler) DetectProtocol(r *http.Request) bool {
	return r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc")
}

func (h *GRPCHandler) Handle(w http.ResponseWriter, r *http.Request) {
	h.server.ServeHTTP(w, r)
}

// RegisterGRPCService registers a gRPC service with the handler
func (h *Handler) RegisterGRPCService(sd *grpc.ServiceDesc, ss interface{}) {
	h.grpcServer.RegisterService(sd, ss)
}
