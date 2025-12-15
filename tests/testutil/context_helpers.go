package testutil

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ContextCarrier is a simple TextMapCarrier implementation for testing context propagation.
// It stores headers in a map for easy inspection and manipulation.
type ContextCarrier struct {
	headers map[string]string
}

// NewContextCarrier creates a new context carrier for testing.
func NewContextCarrier() *ContextCarrier {
	return &ContextCarrier{
		headers: make(map[string]string),
	}
}

// Get retrieves a header value.
func (c *ContextCarrier) Get(key string) string {
	return c.headers[key]
}

// Set sets a header value.
func (c *ContextCarrier) Set(key, value string) {
	c.headers[key] = value
}

// Keys returns all header keys.
func (c *ContextCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}

// GetHeaders returns all headers for inspection.
func (c *ContextCarrier) GetHeaders() map[string]string {
	return c.headers
}

// InjectContext injects trace context into a carrier using the global propagator.
//
// Example:
//
//	carrier := testutil.NewContextCarrier()
//	testutil.InjectContext(ctx, carrier)
//	// Check that traceparent header was set
//	assert.NotEmpty(t, carrier.Get("traceparent"))
func InjectContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, carrier)
}

// ExtractContext extracts trace context from a carrier using the global propagator.
//
// Example:
//
//	carrier := testutil.NewContextCarrier()
//	carrier.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
//	ctx := testutil.ExtractContext(context.Background(), carrier)
//	// ctx now contains the extracted trace context
func ExtractContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, carrier)
}

// CreateTestSpan creates a test span for use in tests.
// The span is started but not ended - caller must call span.End().
//
// Example:
//
//	ctx, span := testutil.CreateTestSpan(ctx, "test-operation")
//	defer span.End()
func CreateTestSpan(ctx context.Context, operationName string) (context.Context, trace.Span) {
	tracer := otel.Tracer("test-tracer")
	return tracer.Start(ctx, operationName)
}

// GetTraceIDFromContext extracts the trace ID from a context.
// Returns an invalid trace ID if no span is present.
func GetTraceIDFromContext(ctx context.Context) trace.TraceID {
	spanContext := trace.SpanContextFromContext(ctx)
	return spanContext.TraceID()
}

// GetSpanIDFromContext extracts the span ID from a context.
// Returns an invalid span ID if no span is present.
func GetSpanIDFromContext(ctx context.Context) trace.SpanID {
	spanContext := trace.SpanContextFromContext(ctx)
	return spanContext.SpanID()
}

// HasTraceContext checks if a context contains trace information.
func HasTraceContext(ctx context.Context) bool {
	spanContext := trace.SpanContextFromContext(ctx)
	return spanContext.IsValid()
}
