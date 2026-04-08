package api

import (
	"log"
	// "net"
	// "google.golang.org/grpc"
	// "server/internal/api/proto" // Need protoc to generate this first
)

// GRPCServer manages the gRPC API endpoint for v2rayNG integration.
type GRPCServer struct {
	port int
	// server *grpc.Server
}

// NewGRPCServer initializes the gRPC service on the given port.
func NewGRPCServer(port int) *GRPCServer {
	return &GRPCServer{
		port: port,
	}
}

// Start listens and serves gRPC requests.
// Currently stubbed until protoc is run.
func (s *GRPCServer) Start() error {
	log.Printf("[grpc] API Server started on port %d (STUB)", s.port)
	return nil
}

func (s *GRPCServer) Stop() {
	log.Println("[grpc] Server stopped")
}
