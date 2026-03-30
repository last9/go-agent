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
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
)

// buildHTTPFilterOptions returns otelhttp options with a route exclusion filter
// if the agent has exclusion rules configured.
func buildHTTPFilterOptions() []otelhttp.Option {
	var opts []otelhttp.Option
	rm := agent.GetRouteMatcher()
	if !rm.IsEmpty() {
		opts = append(opts, otelhttp.WithFilter(func(r *http.Request) bool {
			return !rm.ShouldExclude(r.URL.Path)
		}))
	}
	return opts
}

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
// Also collects metrics for request duration, request/response sizes.
//
// Metrics collected:
//   - rpc.server.duration (histogram)
//   - rpc.server.request.size (histogram)
//   - rpc.server.response.size (histogram)
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

	// Add OpenTelemetry stats handler for tracing
	// Explicitly pass the global propagator to ensure context propagation works
	interceptorOpts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
		)),
	}

	// Combine with user-provided options
	interceptorOpts = append(interceptorOpts, opts...)

	return grpc.NewServer(interceptorOpts...)
}

// WrapHTTPMux wraps an http.ServeMux with OpenTelemetry HTTP instrumentation.
// This should be the outermost layer, capturing HTTP-level metrics and traces.
//
// Metrics collected:
//   - http.server.request.duration (histogram)
//   - http.server.request.body.size (histogram)
//   - http.server.response.body.size (histogram)
//   - http.server.active_requests (gauge)
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

	opts := buildHTTPFilterOptions()
	return otelhttp.NewHandler(mux, serviceName, opts...)
}

// NewDialOption returns a gRPC dial option that instruments client connections.
// Use this when connecting to a gRPC server to automatically trace client calls.
//
// The returned StatsHandler automatically propagates trace context and creates
// client-side spans for all gRPC calls.
//
// Example:
//
//	conn, err := grpc.NewClient(
//	    "localhost:9090",
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpcgateway.NewDialOption(),
//	)
func NewDialOption() grpc.DialOption {
	// Use StatsHandler with explicit propagator for context propagation
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler(
		otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
	))
}
