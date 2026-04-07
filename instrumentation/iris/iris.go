// Package iris provides Last9 instrumentation for the Iris web framework.
// Since there is no official OTel contrib package for Iris, this package
// implements tracing natively using the propagation API.
package iris

import (
	"log"
	"net/http"

	"github.com/kataras/iris/v12"
	agent "github.com/last9/go-agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/last9/go-agent/instrumentation/iris"

// New creates a new Iris application with Last9 instrumentation automatically configured.
// It is a drop-in replacement for iris.New().
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    app := irisagent.New()
//	    app.Get("/ping", func(ctx iris.Context) {
//	        ctx.WriteString("pong")
//	    })
//	    app.Listen(":8080")
//	}
func New() *iris.Application {
	app := iris.New()
	setupInstrumentation(app)
	return app
}

// Middleware returns the Last9 instrumentation middleware for Iris.
// Use this to add instrumentation to an existing Iris application.
//
// Example:
//
//	app := iris.New()
//	app.Use(irisagent.Middleware())
func Middleware() iris.Handler {
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(ctx iris.Context) {
		r := ctx.Request()
		path := r.URL.Path

		rm := agent.GetRouteMatcher()
		if !rm.IsEmpty() && rm.ShouldExclude(path) {
			ctx.Next()
			return
		}

		parentCtx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		method := r.Method
		spanName := method + " " + path
		// Use the matched route template when available so that parametric routes
		// like /users/{id} produce a single span name instead of one per user ID.
		if route := ctx.GetCurrentRoute(); route != nil {
			if routePath := route.Path(); routePath != "" {
				spanName = method + " " + routePath
			}
		}

		spanCtx, span := tracer.Start(parentCtx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.URLPath(path),
				semconv.ServerAddress(r.Host),
			),
		)
		defer span.End()

		ctx.ResetRequest(r.WithContext(spanCtx))
		ctx.Next()

		statusCode := ctx.GetStatusCode()
		span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
		if statusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}
	}
}

// setupInstrumentation adds Last9 telemetry to an Iris application.
func setupInstrumentation(app *iris.Application) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for Iris middleware: %v (instrumentation will not be active)", err)
			return
		}
	}

	app.Use(Middleware())
}
