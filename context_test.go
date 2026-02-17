//go:build test

package agent

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestGetTraceID_NoSpan(t *testing.T) {
	ctx := context.Background()
	if id := GetTraceID(ctx); id != "" {
		t.Errorf("GetTraceID with no span should return empty, got %q", id)
	}
}

func TestGetSpanID_NoSpan(t *testing.T) {
	ctx := context.Background()
	if id := GetSpanID(ctx); id != "" {
		t.Errorf("GetSpanID with no span should return empty, got %q", id)
	}
}

func TestIsTracing_NoSpan(t *testing.T) {
	ctx := context.Background()
	if IsTracing(ctx) {
		t.Error("IsTracing with no span should return false")
	}
}

func TestGetTraceID_ActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	traceID := GetTraceID(ctx)
	if traceID == "" {
		t.Fatal("GetTraceID with active span should return non-empty")
	}
	if len(traceID) != 32 {
		t.Errorf("TraceID should be 32 hex chars, got %d: %q", len(traceID), traceID)
	}
}

func TestGetSpanID_ActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	spanID := GetSpanID(ctx)
	if spanID == "" {
		t.Fatal("GetSpanID with active span should return non-empty")
	}
	if len(spanID) != 16 {
		t.Errorf("SpanID should be 16 hex chars, got %d: %q", len(spanID), spanID)
	}
}

func TestIsTracing_ActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	if !IsTracing(ctx) {
		t.Error("IsTracing with active span should return true")
	}
}

func TestIsTracing_SampledOutSpan(t *testing.T) {
	// NeverSample creates spans that are valid but not recording
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "sampled-out-span")
	defer span.End()

	// The span context should still have valid IDs
	if GetTraceID(ctx) == "" {
		t.Error("GetTraceID should return non-empty even for sampled-out span")
	}

	// But IsTracing should be false because the span is not recording
	if IsTracing(ctx) {
		t.Error("IsTracing should return false for sampled-out (not recording) span")
	}
}

func TestGetTraceID_RemoteSpanContext(t *testing.T) {
	// Simulate a remote span context (e.g., from incoming HTTP headers)
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), sc)

	got := GetTraceID(ctx)
	if got != traceID.String() {
		t.Errorf("Expected trace ID %q, got %q", traceID.String(), got)
	}

	gotSpan := GetSpanID(ctx)
	if gotSpan != spanID.String() {
		t.Errorf("Expected span ID %q, got %q", spanID.String(), gotSpan)
	}
}
