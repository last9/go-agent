// Package gorilla provides Last9 instrumentation for the Gorilla Mux web framework
package gorilla

import (
	"log"

	"github.com/gorilla/mux"
	"github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

// NewRouter creates a new Gorilla Mux router with Last9 instrumentation automatically configured.
// It's a drop-in replacement for mux.NewRouter().
//
// The agent must be started before calling this function:
//
//	agent.Start()
//	defer agent.Shutdown()
//	r := gorilla.NewRouter()
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    r := gorilla.NewRouter()
//	    r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
//	        w.Write([]byte("pong"))
//	    }).Methods("GET")
//	    http.ListenAndServe(":8080", r)
//	}
func NewRouter() *mux.Router {
	r := mux.NewRouter()
	setupInstrumentation(r)
	return r
}

// Middleware returns the Last9 instrumentation middleware for Gorilla Mux.
// Use this if you want to add instrumentation to an existing router.
//
// Example:
//
//	r := mux.NewRouter()
//	r.Use(gorilla.Middleware())
func Middleware() mux.MiddlewareFunc {
	cfg := agent.GetConfig()
	serviceName := "gorilla-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}

	return otelmux.Middleware(serviceName)
}

// setupInstrumentation adds Last9 telemetry to a Gorilla Mux router
func setupInstrumentation(r *mux.Router) {
	if !agent.IsInitialized() {
		// Agent not initialized, try to start it
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for Gorilla middleware: %v (instrumentation will not be active)", err)
			return
		}
	}

	r.Use(Middleware())
}
