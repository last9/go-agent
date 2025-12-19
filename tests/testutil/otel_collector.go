// Package testutil provides testing utilities for the go-agent integration tests.
// It includes a mock OTLP collector, span assertion helpers, and context utilities.
package testutil

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// MockCollector provides a mock OTLP collector for testing.
// It captures spans in memory for verification without requiring a real OTLP endpoint.
type MockCollector struct {
	spanRecorder   *tracetest.SpanRecorder
	tracerProvider *sdktrace.TracerProvider
}

// NewMockCollector creates a new mock OTLP collector.
// The collector captures all spans for verification in tests.
//
// Example:
//
//	collector := testutil.NewMockCollector()
//	defer collector.Shutdown(context.Background())
//
//	// Your test code that generates spans...
//
//	spans := collector.GetSpans()
//	assert.Len(t, spans, 2)
func NewMockCollector() *MockCollector {
	spanRecorder := tracetest.NewSpanRecorder()

	// Create tracer provider with span recorder
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("test-service"),
		)),
	)

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	// Set text map propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &MockCollector{
		spanRecorder:   spanRecorder,
		tracerProvider: tp,
	}
}

// GetSpans returns all spans captured by the mock collector.
// Spans are returned in the order they were ended.
func (m *MockCollector) GetSpans() []sdktrace.ReadOnlySpan {
	return m.spanRecorder.Ended()
}

// Reset clears all captured spans.
// Useful for running multiple sub-tests within the same test function.
func (m *MockCollector) Reset() {
	m.spanRecorder.Reset()
}

// Shutdown shuts down the tracer provider and flushes any pending spans.
func (m *MockCollector) Shutdown(ctx context.Context) error {
	return m.tracerProvider.Shutdown(ctx)
}

// GetTracerProvider returns the underlying tracer provider.
// Useful for tests that need to create custom tracers.
func (m *MockCollector) GetTracerProvider() *sdktrace.TracerProvider {
	return m.tracerProvider
}
