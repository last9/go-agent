//go:build test

package agent

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceFunctionWithResult_NestsDatastoreSpanUnderBusinessSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
		otel.SetTracerProvider(previousProvider)
	})

	ctx, operationSpan := otel.Tracer("test-graphql").Start(
		context.Background(),
		"query GetLeaderboard",
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	result, err := TraceFunctionWithResult(ctx, "LeaderboardStore.GetLeaderboard", func(ctx context.Context) (string, error) {
		_, datastoreSpan := otel.Tracer("test-datastore").Start(
			ctx,
			"redis.get",
			trace.WithSpanKind(trace.SpanKindClient),
		)
		datastoreSpan.End()
		return "leaderboard", nil
	})
	operationSpan.End()

	if err != nil {
		t.Fatalf("TraceFunctionWithResult returned an unexpected error: %v", err)
	}
	if result != "leaderboard" {
		t.Fatalf("TraceFunctionWithResult result = %q, want leaderboard", result)
	}
	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tracerProvider.ForceFlush(flushCtx); err != nil {
		t.Fatalf("flush spans: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected operation, business, and datastore spans; got %d", len(spans))
	}

	byName := make(map[string]trace.SpanContext, len(spans))
	parents := make(map[string]trace.SpanContext, len(spans))
	kinds := make(map[string]trace.SpanKind, len(spans))
	for _, span := range spans {
		byName[span.Name] = span.SpanContext
		parents[span.Name] = span.Parent
		kinds[span.Name] = span.SpanKind
	}

	operation := byName["query GetLeaderboard"]
	business := byName["LeaderboardStore.GetLeaderboard"]
	datastore := byName["redis.get"]
	if !operation.IsValid() || !business.IsValid() || !datastore.IsValid() {
		t.Fatalf("expected all three named spans, got %v", byName)
	}

	if parents["LeaderboardStore.GetLeaderboard"].SpanID() != operation.SpanID() {
		t.Error("business span is not a direct child of the operation span")
	}
	if parents["redis.get"].SpanID() != business.SpanID() {
		t.Error("datastore span is not a direct child of the business span")
	}
	if kinds["LeaderboardStore.GetLeaderboard"] != trace.SpanKindInternal {
		t.Errorf("business span kind = %v, want INTERNAL", kinds["LeaderboardStore.GetLeaderboard"])
	}
}
