// Package grpcgateway provides OpenTelemetry instrumentation for gRPC-gateway services.
// It instruments the full stack: HTTP server -> grpc-gateway -> gRPC server.
package grpcgateway

import (
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
)

// NewGatewayMux creates a new grpc-gateway ServeMux.
// This ServeMux handles gRPC-to-JSON transcoding, converting HTTP/JSON requests into gRPC calls.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	gwMux := grpcgateway.NewGatewayMux()
//	pb.RegisterYourServiceHandler(ctx, gwMux, conn)
func NewGatewayMux(opts ...runtime.ServeMuxOption) *runtime.ServeMux {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (instrumentation disabled)", err)
		}
	}

	return runtime.NewServeMux(opts...)
}

// NewGrpcServer creates a gRPC server with OpenTelemetry interceptors.
// The server automatically traces all unary and streaming RPC calls.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	grpcServer := grpcgateway.NewGrpcServer()
//	pb.RegisterYourServiceServer(grpcServer, &serviceImpl{})
//	grpcServer.Serve(listener)
func NewGrpcServer(opts ...grpc.ServerOption) *grpc.Server {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (returning non-instrumented server)", err)
			return grpc.NewServer(opts...)
		}
	}

	// Add OpenTelemetry interceptors for tracing
	interceptorOpts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	// Combine with user-provided options
	interceptorOpts = append(interceptorOpts, opts...)

	return grpc.NewServer(interceptorOpts...)
}

// WrapHTTPMux wraps an http.ServeMux with OpenTelemetry HTTP instrumentation.
// This should be the outermost layer, capturing HTTP-level metrics and traces.
//
// The serviceName parameter is used for naming the HTTP operation spans.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	httpMux := http.NewServeMux()
//	httpMux.Handle("/api/", gwMux)
//	handler := grpcgateway.WrapHTTPMux(httpMux, "my-service")
//	http.ListenAndServe(":8080", handler)
func WrapHTTPMux(mux *http.ServeMux, serviceName string) http.Handler {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (returning unwrapped handler)", err)
			return mux
		}
	}

	return otelhttp.NewHandler(mux, serviceName)
}

// NewDialOption returns a gRPC dial option that instruments client connections.
// Use this when connecting the grpc-gateway to your gRPC server to trace client calls.
//
// Example:
//
//	conn, err := grpc.NewClient(
//	    "localhost:9090",
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpcgateway.NewDialOption(),
//	)
func NewDialOption() grpc.DialOption {
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler())
}
