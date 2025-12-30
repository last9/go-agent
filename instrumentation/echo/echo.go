// Package echo provides Last9 instrumentation for the Echo web framework
package echo

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// New creates a new Echo instance with Last9 instrumentation automatically configured.
// It's a drop-in replacement for echo.New().
//
// The agent must be started before calling this function:
//
//	agent.Start()
//	defer agent.Shutdown()
//	e := echo.New()
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    e := echo.New()
//	    e.GET("/ping", func(c echo.Context) error {
//	        return c.String(200, "pong")
//	    })
//	    e.Start(":8080")
//	}
func New() *echo.Echo {
	e := echo.New()
	setupInstrumentation(e)
	return e
}

// Middleware returns the Last9 instrumentation middleware for Echo.
// Use this if you want to add instrumentation to an existing Echo instance.
//
// Example:
//
//	e := echo.New()
//	e.Use(echo.Middleware())
func Middleware() echo.MiddlewareFunc {
	cfg := agent.GetConfig()
	serviceName := "echo-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}

	return otelecho.Middleware(serviceName)
}

// setupInstrumentation adds Last9 telemetry to an Echo instance
func setupInstrumentation(e *echo.Echo) {
	if !agent.IsInitialized() {
		// Agent not initialized, try to start it
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for Echo middleware: %v (instrumentation will not be active)", err)
			return
		}
	}

	e.Use(Middleware())
}
