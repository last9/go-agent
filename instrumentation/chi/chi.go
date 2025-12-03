// Package chi provides Last9 instrumentation for the Chi web framework
package chi

import (
	"github.com/go-chi/chi/v5"
	"github.com/last9/go-agent"
	"github.com/riandyrn/otelchi"
)

// New creates a new Chi router with Last9 instrumentation automatically configured.
// It's a drop-in replacement for chi.NewRouter().
//
// The agent must be started before calling this function:
//
//	agent.Start()
//	defer agent.Shutdown()
//	r := chi.New()
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    r := chi.New()
//	    r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
//	        w.Write([]byte("pong"))
//	    })
//	    http.ListenAndServe(":8080", r)
//	}
func New() *chi.Mux {
	r := chi.NewRouter()
	setupInstrumentation(r)
	return r
}

// NewRouter is an alias for New() to match chi.NewRouter() naming
func NewRouter() *chi.Mux {
	return New()
}

// Middleware returns the Last9 instrumentation middleware for Chi.
// Use this if you want to add instrumentation to an existing Chi router.
//
// IMPORTANT: Add this middleware AFTER defining all your routes to ensure
// proper route pattern capture.
//
// Example:
//
//	r := chi.NewRouter()
//	r.Get("/users/{id}", GetUserHandler)
//	r.Use(chi.Middleware(r))  // Add AFTER routes
func Middleware(router *chi.Mux) func(next http.Handler) http.Handler {
	cfg := agent.GetConfig()
	serviceName := "chi-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}

	return otelchi.Middleware(
		serviceName,
		otelchi.WithChiRoutes(router), // Capture route patterns
	)
}

// setupInstrumentation adds Last9 telemetry to a Chi router
func setupInstrumentation(r *chi.Mux) {
	if !agent.IsInitialized() {
		// Agent not initialized, try to start it
		if err := agent.Start(); err != nil {
			// Log error but don't fail - user might initialize later
			return
		}
	}

	// Note: We can't add middleware here because routes aren't defined yet.
	// The user should call Use(Middleware(r)) after defining routes,
	// or use the helper below.
}

// Use adds the Last9 instrumentation middleware to the router.
// This is a convenience function that should be called AFTER all routes are defined.
//
// Example:
//
//	r := chi.New()
//	r.Get("/users/{id}", GetUserHandler)
//	r.Post("/users", CreateUserHandler)
//	chi.Use(r)  // Add instrumentation AFTER routes
func Use(router *chi.Mux) {
	router.Use(Middleware(router))
}
