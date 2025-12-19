// Package gin provides Last9 instrumentation for the Gin web framework
package gin

import (
	"github.com/gin-gonic/gin"
	"github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// New creates a new Gin engine with Last9 instrumentation automatically configured.
// It's a drop-in replacement for gin.New() for most use cases.
//
// Note: If you need to pass gin.OptionFunc options, use gin.New() directly
// and add instrumentation with Middleware():
//
//	r := gin.New(gin.WithRedirectTrailingSlash(true))
//	r.Use(ginagent.Middleware())
//
// The agent must be started before calling this function:
//
//	agent.Start()
//	defer agent.Shutdown()
//	r := gin.New()
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    r := gin.New()
//	    r.GET("/ping", func(c *gin.Context) {
//	        c.JSON(200, gin.H{"message": "pong"})
//	    })
//	    r.Run(":8080")
//	}
func New() *gin.Engine {
	r := gin.New()
	setupInstrumentation(r)
	return r
}

// Default creates a new Gin engine with Logger, Recovery middleware and Last9 instrumentation.
// It's a drop-in replacement for gin.Default() for most use cases.
//
// Note: If you need to pass gin.OptionFunc options, use gin.Default() directly
// and add instrumentation with Middleware():
//
//	r := gin.Default(gin.WithRedirectTrailingSlash(true))
//	r.Use(ginagent.Middleware())
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    r := gin.Default() // Includes logging, recovery, and telemetry
//	    r.GET("/ping", func(c *gin.Context) {
//	        c.JSON(200, gin.H{"message": "pong"})
//	    })
//	    r.Run(":8080")
//	}
func Default() *gin.Engine {
	r := gin.Default()
	setupInstrumentation(r)
	return r
}

// Middleware returns the Last9 instrumentation middleware for Gin.
// Use this if you want to add instrumentation to an existing Gin engine.
//
// Example:
//
//	r := gin.New()
//	r.Use(gin.Middleware())
func Middleware() gin.HandlerFunc {
	cfg := agent.GetConfig()
	serviceName := "gin-service"
	if cfg != nil {
		serviceName = cfg.ServiceName
	}
	return otelgin.Middleware(serviceName)
}

// setupInstrumentation adds Last9 telemetry to a Gin engine
func setupInstrumentation(r *gin.Engine) {
	if !agent.IsInitialized() {
		// Agent not initialized, try to start it
		if err := agent.Start(); err != nil {
			// Log error but continue - instrumentation will be disabled
			msg := "[Last9 Agent] Failed to auto-start: " + err.Error() + "\n"
			if _, writeErr := gin.DefaultWriter.Write([]byte(msg)); writeErr != nil {
				// Can't log the error anywhere, silent failure
				return
			}
			return
		}
	}

	r.Use(Middleware())
}
