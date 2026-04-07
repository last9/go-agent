// Package grpc provides Last9 instrumentation for gRPC servers and clients.
// It wraps grpc server creation and client dial options to automatically add
// OpenTelemetry tracing and metrics without manual SDK setup.
package grpc

import (
	"log"

	agent "github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	googlegrpc "google.golang.org/grpc"
)

// NewServer creates a gRPC server with Last9 instrumentation automatically configured.
// It is a drop-in replacement for grpc.NewServer() that automatically adds
// OpenTelemetry tracing and metrics for all unary and streaming RPC calls.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    lis, _ := net.Listen("tcp", ":50051")
//	    s := grpcagent.NewServer()
//	    pb.RegisterGreeterServer(s, &server{})
//	    s.Serve(lis)
//	}
func NewServer(opts ...googlegrpc.ServerOption) *googlegrpc.Server {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (returning non-instrumented server)", err)
			return googlegrpc.NewServer(opts...)
		}
	}

	serverOpts := []googlegrpc.ServerOption{
		googlegrpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
		)),
	}
	serverOpts = append(serverOpts, opts...)
	return googlegrpc.NewServer(serverOpts...)
}

// NewClientDialOption returns a gRPC DialOption that instruments outgoing client
// connections with OpenTelemetry tracing and metrics.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	conn, err := grpc.NewClient(
//	    "localhost:50051",
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpcagent.NewClientDialOption(),
//	)
func NewClientDialOption() googlegrpc.DialOption {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (client tracing disabled)", err)
		}
	}

	return googlegrpc.WithStatsHandler(otelgrpc.NewClientHandler(
		otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
	))
}
