// Package fasthttp provides Last9 instrumentation for the fasthttp web framework.
// Since fasthttp uses its own context type instead of net/http, this package
// implements OTel tracing natively using the propagation API.
package fasthttp

import (
	"context"
	"log"
	"net/http"

	agent "github.com/last9/go-agent"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName     = "github.com/last9/go-agent/instrumentation/fasthttp"
	spanContextKey = "__last9_otel_ctx"
)

// headerCarrier adapts fasthttp request headers to propagation.TextMapCarrier
// so that trace context can be extracted from incoming requests.
type headerCarrier struct {
	header *fasthttp.RequestHeader
}

func (c headerCarrier) Get(key string) string {
	return string(c.header.Peek(key))
}

func (c headerCarrier) Set(key, val string) {
	c.header.Set(key, val)
}

func (c headerCarrier) Keys() []string {
	var keys []string
	for key := range c.header.All() {
		keys = append(keys, string(key))
	}
	return keys
}

// Middleware wraps a fasthttp.RequestHandler with Last9 instrumentation.
// It extracts incoming trace context, creates a server span for each request,
// and stores the context so handlers can create child spans via ContextFromRequest.
//
// Requests matching the agent's excluded path rules are passed through without tracing.
//
// The agent will be automatically initialized if not already done.
//
// Example:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//
//	    handler := func(ctx *fasthttp.RequestCtx) {
//	        ctx.WriteString("hello")
//	    }
//	    fasthttp.ListenAndServe(":8080", fasthttpagent.Middleware(handler))
//	}
func Middleware(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (instrumentation disabled)", err)
			return next
		}
	}

	tracer := otel.GetTracerProvider().Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()
	rm := agent.GetRouteMatcher()

	return func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		if !rm.IsEmpty() && rm.ShouldExclude(path) {
			next(ctx)
			return
		}

		carrier := headerCarrier{header: &ctx.Request.Header}
		parentCtx := propagator.Extract(context.Background(), carrier)

		method := string(ctx.Method())
		spanCtx, span := tracer.Start(parentCtx, method+" "+path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.URLPath(path),
				semconv.ServerAddress(string(ctx.Host())),
			),
		)
		defer span.End()

		ctx.SetUserValue(spanContextKey, spanCtx)
		next(ctx)

		statusCode := ctx.Response.StatusCode()
		span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
		if statusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}
	}
}

// ContextFromRequest retrieves the OpenTelemetry context stored by Middleware.
// Use this in handlers to create child spans tied to the incoming request trace.
//
// Example:
//
//	func handler(ctx *fasthttp.RequestCtx) {
//	    otelCtx := fasthttpagent.ContextFromRequest(ctx)
//	    _, span := otel.Tracer("my-service").Start(otelCtx, "my-op")
//	    defer span.End()
//	    // ...
//	}
func ContextFromRequest(ctx *fasthttp.RequestCtx) context.Context {
	if spanCtx, ok := ctx.UserValue(spanContextKey).(context.Context); ok {
		return spanCtx
	}
	return context.Background()
}
