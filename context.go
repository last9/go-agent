package agent

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// GetTraceID returns the trace ID from the context as a hex string.
// Returns "" if the context has no active span or the span context is invalid.
func GetTraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// GetSpanID returns the span ID from the context as a hex string.
// Returns "" if the context has no active span or the span context is invalid.
func GetSpanID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}

// IsTracing returns true if the context has a valid, actively recording span.
// A sampled-out span is valid but not recording, so this returns false for it.
func IsTracing(ctx context.Context) bool {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	return sc.IsValid() && span.IsRecording()
}
