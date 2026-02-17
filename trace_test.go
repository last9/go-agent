//go:build test

package agent

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		tp.Shutdown(context.Background())
	})
	return exporter
}

func TestStartSpan(t *testing.T) {
	exporter := setupTestTracer(t)

	ctx := context.Background()
	ctx, span := StartSpan(ctx, "test-operation")

	// Span should be active in context
	if GetTraceID(ctx) == "" {
		t.Error("StartSpan should create a span with valid trace ID")
	}

	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test-operation" {
		t.Errorf("Expected span name 'test-operation', got %q", spans[0].Name)
	}
}

func TestStartSpan_PropagatesTraceID(t *testing.T) {
	exporter := setupTestTracer(t)

	ctx := context.Background()
	ctx, parent := StartSpan(ctx, "parent")
	parentTraceID := GetTraceID(ctx)

	ctx, child := StartSpan(ctx, "child")
	childTraceID := GetTraceID(ctx)

	child.End()
	parent.End()

	// Child should share parent's trace ID
	if parentTraceID != childTraceID {
		t.Errorf("Child trace ID %q should match parent %q", childTraceID, parentTraceID)
	}

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("Expected 2 spans, got %d", len(spans))
	}
}

func TestTraceFunction_Success(t *testing.T) {
	exporter := setupTestTracer(t)

	called := false
	err := TraceFunction(context.Background(), "success-op", func(ctx context.Context) error {
		called = true
		// Verify span is active inside the function
		if !IsTracing(ctx) {
			t.Error("Should be tracing inside TraceFunction")
		}
		return nil
	})

	if err != nil {
		t.Errorf("TraceFunction should return nil error, got: %v", err)
	}
	if !called {
		t.Error("Function should have been called")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "success-op" {
		t.Errorf("Expected span name 'success-op', got %q", spans[0].Name)
	}
	if spans[0].Status.Code != codes.Unset {
		t.Errorf("Expected status Unset for success, got %v", spans[0].Status.Code)
	}
}

func TestTraceFunction_Error(t *testing.T) {
	exporter := setupTestTracer(t)

	testErr := errors.New("something went wrong")
	err := TraceFunction(context.Background(), "error-op", func(ctx context.Context) error {
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("TraceFunction should return the function's error, got: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("Expected status Error, got %v", spans[0].Status.Code)
	}
	if spans[0].Status.Description != "something went wrong" {
		t.Errorf("Expected error description 'something went wrong', got %q", spans[0].Status.Description)
	}

	// Verify error was recorded as an event
	events := spans[0].Events
	hasErrorEvent := false
	for _, e := range events {
		if e.Name == "exception" {
			hasErrorEvent = true
			break
		}
	}
	if !hasErrorEvent {
		t.Error("Expected an 'exception' event from RecordError")
	}
}

func TestTraceFunctionWithResult_Success(t *testing.T) {
	exporter := setupTestTracer(t)

	result, err := TraceFunctionWithResult[int](context.Background(), "compute", func(ctx context.Context) (int, error) {
		return 42, nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != 42 {
		t.Errorf("Expected result 42, got %d", result)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "compute" {
		t.Errorf("Expected span name 'compute', got %q", spans[0].Name)
	}
	if spans[0].Status.Code != codes.Unset {
		t.Errorf("Expected status Unset, got %v", spans[0].Status.Code)
	}
}

func TestTraceFunctionWithResult_Error(t *testing.T) {
	exporter := setupTestTracer(t)

	testErr := errors.New("db connection failed")
	result, err := TraceFunctionWithResult[string](context.Background(), "fetch-data", func(ctx context.Context) (string, error) {
		return "", testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("Expected testErr, got: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty result on error, got %q", result)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("Expected status Error, got %v", spans[0].Status.Code)
	}
}

func TestTraceFunctionWithResult_PointerType(t *testing.T) {
	exporter := setupTestTracer(t)

	type User struct{ Name string }

	user, err := TraceFunctionWithResult[*User](context.Background(), "get-user", func(ctx context.Context) (*User, error) {
		return &User{Name: "Alice"}, nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if user == nil || user.Name != "Alice" {
		t.Errorf("Expected User{Alice}, got %v", user)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}
}
