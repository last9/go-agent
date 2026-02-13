// Package chi provides Last9 instrumentation for the Chi web framework.
//
// Due to Chi's middleware design, instrumentation must be applied AFTER routes
// are defined. Use the Use() function to wrap your router.
//
// Example:
//
//	import (
//	    "github.com/go-chi/chi/v5"
//	    "github.com/last9/go-agent"
//	    chiagent "github.com/last9/go-agent/instrumentation/chi"
//	)
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    r := chi.NewRouter()
//	    r.Get("/ping", pingHandler)
//	    r.Get("/users/{id}", getUserHandler)
//
//	    // Wrap with instrumentation AFTER defining routes
//	    handler := chiagent.Use(r)
//	    http.ListenAndServe(":8080", handler)
//	}
package chi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/last9/go-agent"
	"github.com/riandyrn/otelchi"
)

// New creates a new Chi router.
//
// IMPORTANT: This does NOT automatically add instrumentation because Chi requires
// middleware to be applied after routes are defined for proper route pattern capture.
// You must call Use(router) after defining your routes.
//
// Example:
//
//	r := chiagent.New()
//	r.Get("/ping", pingHandler)
//	r.Get("/users/{id}", getUserHandler)
//	handler := chiagent.Use(r)  // Apply instrumentation
//	http.ListenAndServe(":8080", handler)
func New() *chi.Mux {
	r := chi.NewRouter()
	ensureAgentStarted()
	return r
}

// NewRouter is an alias for New() to match chi.NewRouter() naming
func NewRouter() *chi.Mux {
	return New()
}

// Middleware returns the Last9 instrumentation middleware for Chi.
//
// NOTE: In Chi v5, calling router.Use() after routes are defined will panic.
// Use the Use() function instead, which returns a wrapped http.Handler.
//
// This function is provided for cases where you need the raw middleware,
// such as adding it to a router BEFORE defining routes (though this won't
// capture route patterns properly).
func Middleware(router *chi.Mux) func(next http.Handler) http.Handler {
	cfg := agent.GetConfig()
	serviceName := "chi-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}

	return otelchi.Middleware(
		serviceName,
		buildOptions(router)...,
	)
}

// buildOptions returns otelchi options with route info and optional filter.
func buildOptions(router *chi.Mux) []otelchi.Option {
	opts := []otelchi.Option{
		otelchi.WithChiRoutes(router),
	}
	rm := agent.GetRouteMatcher()
	if !rm.IsEmpty() {
		opts = append(opts, otelchi.WithFilter(func(r *http.Request) bool {
			return !rm.ShouldExclude(r.URL.Path)
		}))
	}
	return opts
}

// ensureAgentStarted starts the agent if not already initialized
func ensureAgentStarted() {
	if !agent.IsInitialized() {
		_ = agent.Start()
	}
}

// Use wraps a Chi router with Last9 instrumentation and returns an http.Handler.
// This is the recommended way to add instrumentation to Chi.
//
// IMPORTANT: Call this AFTER all routes are defined to ensure proper route
// pattern capture (e.g., "/users/{id}" instead of "/users/123").
//
// Example:
//
//	r := chi.NewRouter()
//	r.Get("/users/{id}", getUserHandler)
//	r.Post("/users", createUserHandler)
//
//	handler := chiagent.Use(r)  // Wrap AFTER routes
//	http.ListenAndServe(":8080", handler)
func Use(router *chi.Mux) http.Handler {
	ensureAgentStarted()

	cfg := agent.GetConfig()
	serviceName := "chi-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}

	middleware := otelchi.Middleware(
		serviceName,
		buildOptions(router)...,
	)
	return middleware(router)
}
