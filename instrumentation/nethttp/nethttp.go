// Package nethttp provides Last9 instrumentation for Go's standard net/http package.
//
// This package enables automatic tracing and metrics collection for HTTP servers
// built with the standard library, without requiring any framework.
//
// There are three main usage patterns:
//
// 1. Wrap individual handlers:
//
//	http.Handle("/api", nethttp.Handler(myHandler, "/api"))
//	http.ListenAndServe(":8080", nil)
//
// 2. Use the instrumented ServeMux (recommended):
//
//	mux := nethttp.NewServeMux()
//	mux.HandleFunc("/users", usersHandler)
//	mux.HandleFunc("/orders", ordersHandler)
//	http.ListenAndServe(":8080", mux)
//
// 3. Wrap an existing handler at the top level:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/users", usersHandler)
//	http.ListenAndServe(":8080", nethttp.WrapHandler(mux))
package nethttp

import (
	"context"
	"log"
	"net/http"

	"github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Handler wraps an http.Handler with OpenTelemetry instrumentation.
// The operation parameter is used as the span name and should describe
// the handler's purpose (e.g., "/users", "GetUser", "api.users.list").
//
// If operation is empty, it defaults to "HTTP".
//
// Example:
//
//	userHandler := nethttp.Handler(http.HandlerFunc(handleUsers), "/users")
//	http.Handle("/users", userHandler)
func Handler(h http.Handler, operation string) http.Handler {
	ensureAgentStarted()

	if operation == "" {
		operation = "HTTP"
	}

	return otelhttp.NewHandler(h, operation, buildOTelOptions()...)
}

// HandlerFunc wraps an http.HandlerFunc with OpenTelemetry instrumentation.
// This is a convenience wrapper around Handler for function handlers.
//
// Example:
//
//	http.Handle("/ping", nethttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	    w.Write([]byte("pong"))
//	}, "/ping"))
func HandlerFunc(f http.HandlerFunc, operation string) http.Handler {
	return Handler(f, operation)
}

// WrapHandler wraps an existing http.Handler (like http.ServeMux) with
// OpenTelemetry instrumentation at the top level.
//
// This is useful when you want to instrument an entire mux or handler chain
// with a single wrapper. The span name will be derived from the HTTP method
// and path.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/users", usersHandler)
//	mux.HandleFunc("/orders", ordersHandler)
//
//	// Wrap the entire mux
//	http.ListenAndServe(":8080", nethttp.WrapHandler(mux))
func WrapHandler(h http.Handler) http.Handler {
	ensureAgentStarted()
	return otelhttp.NewHandler(h, "", buildOTelOptions()...)
}

// ServeMux is an instrumented version of http.ServeMux.
// It automatically adds OpenTelemetry tracing to all registered handlers.
type ServeMux struct {
	mux *http.ServeMux
}

// NewServeMux creates a new instrumented ServeMux.
// All handlers registered with this mux will be automatically instrumented.
//
// Example:
//
//	mux := nethttp.NewServeMux()
//	mux.HandleFunc("/users", usersHandler)
//	mux.HandleFunc("/orders/{id}", orderHandler)
//	http.ListenAndServe(":8080", mux)
func NewServeMux() *ServeMux {
	ensureAgentStarted()
	return &ServeMux{
		mux: http.NewServeMux(),
	}
}

// Handle registers the handler for the given pattern with automatic instrumentation.
// The pattern is used as the span operation name.
func (m *ServeMux) Handle(pattern string, handler http.Handler) {
	m.mux.Handle(pattern, Handler(handler, pattern))
}

// HandleFunc registers the handler function for the given pattern with automatic instrumentation.
func (m *ServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.mux.Handle(pattern, Handler(http.HandlerFunc(handler), pattern))
}

// ServeHTTP implements http.Handler interface.
func (m *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mux.ServeHTTP(w, r)
}

// Handler returns the http.Handler for the given pattern.
// This method exists for compatibility with http.ServeMux.
func (m *ServeMux) Handler(r *http.Request) (h http.Handler, pattern string) {
	return m.mux.Handler(r)
}

// Middleware returns an HTTP middleware that adds OpenTelemetry instrumentation.
// Use this when you need to compose with other middleware.
//
// The operation parameter determines the span name. If empty, the HTTP method
// and path will be used.
//
// Example with standard middleware chain:
//
//	handler := nethttp.Middleware("api")(myHandler)
//	http.ListenAndServe(":8080", handler)
//
// Example with custom middleware:
//
//	handler := loggingMiddleware(nethttp.Middleware("api")(myHandler))
func Middleware(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Handler(next, operation)
	}
}

// ListenAndServe starts an HTTP server with the given handler wrapped in instrumentation.
// This is a drop-in replacement for http.ListenAndServe.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/ping", pingHandler)
//	nethttp.ListenAndServe(":8080", mux)
func ListenAndServe(addr string, handler http.Handler) error {
	ensureAgentStarted()

	if handler == nil {
		handler = http.DefaultServeMux
	}

	return http.ListenAndServe(addr, WrapHandler(handler))
}

// ListenAndServeTLS starts an HTTPS server with the given handler wrapped in instrumentation.
// This is a drop-in replacement for http.ListenAndServeTLS.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/ping", pingHandler)
//	nethttp.ListenAndServeTLS(":8443", "cert.pem", "key.pem", mux)
func ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	ensureAgentStarted()

	if handler == nil {
		handler = http.DefaultServeMux
	}

	return http.ListenAndServeTLS(addr, certFile, keyFile, WrapHandler(handler))
}

// Server wraps an http.Server with instrumented handler.
// Use this when you need more control over the server configuration.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/ping", pingHandler)
//
//	server := nethttp.Server(&http.Server{
//	    Addr:         ":8080",
//	    Handler:      mux,
//	    ReadTimeout:  5 * time.Second,
//	    WriteTimeout: 10 * time.Second,
//	})
//	server.ListenAndServe()
func Server(srv *http.Server) *http.Server {
	ensureAgentStarted()

	if srv.Handler != nil {
		srv.Handler = WrapHandler(srv.Handler)
	}

	return srv
}

// ExtractContext extracts the trace context from incoming HTTP request headers.
// Use this to manually propagate context in custom handlers or when
// integrating with non-standard frameworks.
//
// Example:
//
//	func myHandler(w http.ResponseWriter, r *http.Request) {
//	    ctx := nethttp.ExtractContext(r.Context(), r)
//	    // ctx now contains trace context from incoming headers
//	    doWork(ctx)
//	}
func ExtractContext(ctx context.Context, r *http.Request) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))
}

// InjectContext injects the trace context into outgoing HTTP request headers.
// Use this when making outgoing HTTP calls that don't use the instrumented client.
//
// Example:
//
//	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	nethttp.InjectContext(ctx, req)
//	// req.Header now contains trace propagation headers
func InjectContext(ctx context.Context, r *http.Request) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
}

// buildOTelOptions returns common otelhttp options: propagator, server name, and route filter.
func buildOTelOptions() []otelhttp.Option {
	opts := []otelhttp.Option{
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
	}

	cfg := agent.GetConfig()
	if cfg != nil {
		opts = append(opts, otelhttp.WithServerName(cfg.ServiceName))
	}

	rm := agent.GetRouteMatcher()
	if !rm.IsEmpty() {
		opts = append(opts, otelhttp.WithFilter(func(r *http.Request) bool {
			return !rm.ShouldExclude(r.URL.Path)
		}))
	}

	return opts
}

// ensureAgentStarted starts the agent if not already initialized
func ensureAgentStarted() {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for net/http: %v", err)
		}
	}
}
