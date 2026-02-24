// Package beego provides Last9 instrumentation for the Beego v2 web framework.
package beego

import (
	"fmt"
	"log"
	"net/http"

	"github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/server/web/context"
	agent "github.com/last9/go-agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/last9/go-agent/instrumentation/beego"

// New creates a new Beego HttpServer with Last9 instrumentation automatically configured.
// It's a drop-in replacement for web.NewHttpSever().
//
// The agent must be started before calling this function:
//
//	agent.Start()
//	defer agent.Shutdown()
//	app := beego.New()
//
// Example usage:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    app := beego.New()
//	    app.Get("/ping", func(ctx *context.Context) {
//	        ctx.Output.Body([]byte("pong"))
//	    })
//	    app.Run()
//	}
func New() *web.HttpServer {
	app := web.NewHttpSever()
	setupInstrumentation(app)
	return app
}

// Middleware returns the Last9 instrumentation middleware for Beego as a FilterChain.
// Use this if you want to add instrumentation to an existing Beego HttpServer.
//
// Example:
//
//	app := web.NewHttpSever()
//	app.InsertFilterChain("/*", beego.Middleware())
func Middleware() func(next web.FilterFunc) web.FilterFunc {
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(next web.FilterFunc) web.FilterFunc {
		return func(ctx *context.Context) {
			req := ctx.Request

			// Extract propagated trace context from incoming request
			parentCtx := propagator.Extract(req.Context(), propagation.HeaderCarrier(req.Header))

			// Uses raw path for span name. Beego does not expose the matched
			// route pattern in a FilterChain, so routes with path parameters
			// (e.g., /users/:id) will produce per-path span names.
			spanName := fmt.Sprintf("%s %s", req.Method, req.URL.Path)

			opts := []trace.SpanStartOption{
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(req.Method),
					semconv.URLPath(req.URL.Path),
					semconv.ServerAddress(req.Host),
				),
			}

			spanCtx, span := tracer.Start(parentCtx, spanName, opts...)
			defer span.End()

			// Inject trace context into the request so downstream code can access it
			ctx.Request = req.WithContext(spanCtx)

			// Execute the rest of the filter/router chain
			next(ctx)

			// Record response attributes
			statusCode := ctx.ResponseWriter.Status
			span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))

			if statusCode >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(statusCode))
			}
		}
	}
}

// setupInstrumentation adds Last9 telemetry to a Beego HttpServer
func setupInstrumentation(app *web.HttpServer) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for Beego middleware: %v (instrumentation will not be active)", err)
			return
		}
	}

	app.InsertFilterChain("/*", Middleware())
}
