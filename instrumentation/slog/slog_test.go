package slog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	slogagent "github.com/last9/go-agent/instrumentation/slog"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// startSpan creates a TracerProvider, starts a span, and returns the context,
// span, and a cleanup function. The caller must call cleanup when done.
func startSpan(t *testing.T) (context.Context, trace.Span, func()) {
	t.Helper()
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	return ctx, span, func() {
		span.End()
		_ = tp.Shutdown(context.Background())
	}
}

func parseJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	return entry
}

func TestHandler_NoSpan_NoInjection(t *testing.T) {
	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	logger := slog.New(h)

	logger.InfoContext(context.Background(), "hello")

	entry := parseJSON(t, &buf)
	if _, ok := entry["trace_id"]; ok {
		t.Error("trace_id must not be present when no span is active")
	}
	if _, ok := entry["span_id"]; ok {
		t.Error("span_id must not be present when no span is active")
	}
}

func TestHandler_ActiveSpan_InjectsIDs(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	logger := slog.New(h)

	logger.InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)

	traceID, ok := entry["trace_id"].(string)
	if !ok || traceID == "" {
		t.Fatalf("expected non-empty trace_id, got %v", entry["trace_id"])
	}
	if len(traceID) != 32 {
		t.Errorf("trace_id must be 32 hex chars, got %d: %q", len(traceID), traceID)
	}

	spanID, ok := entry["span_id"].(string)
	if !ok || spanID == "" {
		t.Fatalf("expected non-empty span_id, got %v", entry["span_id"])
	}
	if len(spanID) != 16 {
		t.Errorf("span_id must be 16 hex chars, got %d: %q", len(spanID), spanID)
	}
}

func TestHandler_SampledOutSpan_StillInjectsIDs(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(context.Background(), "sampled-out")
	defer span.End()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	slog.New(h).InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id should be injected even for sampled-out spans (sc.IsValid() == true)")
	}
	if _, ok := entry["span_id"]; !ok {
		t.Error("span_id should be injected even for sampled-out spans")
	}
}

func TestHandler_RemoteSpanContext_InjectsIDs(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), sc)

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	slog.New(h).InfoContext(ctx, "remote")

	entry := parseJSON(t, &buf)
	if got := entry["trace_id"]; got != traceID.String() {
		t.Errorf("trace_id = %q, want %q", got, traceID.String())
	}
	if got := entry["span_id"]; got != spanID.String() {
		t.Errorf("span_id = %q, want %q", got, spanID.String())
	}
}

func TestHandler_CustomKeys(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, &slogagent.Options{
		TraceKey: "dd.trace_id",
		SpanKey:  "dd.span_id",
	})
	slog.New(h).InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	if _, ok := entry["dd.trace_id"]; !ok {
		t.Error("expected dd.trace_id key")
	}
	if _, ok := entry["dd.span_id"]; !ok {
		t.Error("expected dd.span_id key")
	}
	if _, ok := entry["trace_id"]; ok {
		t.Error("default trace_id key must not appear when custom key is set")
	}
}

func TestHandler_WithAttrs_PreservesTraceInjection(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	h2 := h.WithAttrs([]slog.Attr{slog.String("service", "my-svc")})

	slog.New(h2).InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	if entry["service"] != "my-svc" {
		t.Error("pre-set attribute 'service' must be present")
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must still be injected after WithAttrs")
	}
	if _, ok := entry["span_id"]; !ok {
		t.Error("span_id must still be injected after WithAttrs")
	}
}

func TestHandler_WithGroup_PreservesTraceInjection(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	grouped := h.WithGroup("request")

	slog.New(grouped).InfoContext(ctx, "hello", "path", "/api")

	output := buf.String()
	if !strings.Contains(output, `"request"`) {
		t.Errorf("group name must appear in output, got: %s", output)
	}
	if !strings.Contains(output, "trace_id") {
		t.Errorf("trace_id must still be injected after WithGroup, got: %s", output)
	}
}

func TestHandler_WithGroup_EmptyName_ReturnsSelf(t *testing.T) {
	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	h2 := h.WithGroup("")
	if h2 != h {
		t.Error("WithGroup with empty name must return the same handler")
	}
}

func TestNewTextHandler_InjectsIDs(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewTextHandler(&buf, nil, nil)
	slog.New(h).InfoContext(ctx, "hello")

	output := buf.String()
	if !strings.Contains(output, "trace_id=") {
		t.Errorf("text handler output must contain trace_id=, got: %s", output)
	}
	if !strings.Contains(output, "span_id=") {
		t.Errorf("text handler output must contain span_id=, got: %s", output)
	}
}

func TestHandler_Enabled_DelegatesToInner(t *testing.T) {
	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}, nil)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Info level must be disabled when inner handler is configured for Warn+")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Warn level must be enabled")
	}
}

func TestNewHandler_NilInner_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewHandler with nil inner must panic")
		}
	}()
	slogagent.NewHandler(nil, nil)
}

func TestSetDefault_SetsGlobalLogger(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	// Save and restore original default logger.
	original := slog.Default()
	defer slog.SetDefault(original)

	var buf bytes.Buffer
	slogagent.SetDefault(&buf, nil, nil)

	slog.InfoContext(ctx, "global log")

	entry := parseJSON(t, &buf)
	if _, ok := entry["trace_id"]; !ok {
		t.Error("global slog logger must inject trace_id")
	}
}

func TestHandler_ConcurrentHandleCalls(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	logger := slog.New(h)

	const goroutines = 50
	done := make(chan struct{})
	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			logger.InfoContext(ctx, "concurrent", "n", n)
		}(i)
	}
	for range goroutines {
		<-done
	}

	// Each goroutine wrote one JSON line. Verify we got all of them and
	// none are corrupted (valid JSON with trace_id present).
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != goroutines {
		t.Fatalf("expected %d log lines, got %d", goroutines, len(lines))
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d: invalid JSON: %v\nraw: %s", i, err, line)
		}
		if _, ok := entry["trace_id"]; !ok {
			t.Errorf("line %d: missing trace_id", i)
		}
		if _, ok := entry["span_id"]; !ok {
			t.Errorf("line %d: missing span_id", i)
		}
	}
}

func TestHandler_DeeplyNestedWithAttrsAndGroups(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)

	// Build a chain: WithGroup("a") → WithAttrs(service) → WithGroup("b")
	nested := h.WithGroup("a").
		WithAttrs([]slog.Attr{slog.String("service", "test-svc")}).
		WithGroup("b")

	slog.New(nested).InfoContext(ctx, "deep", "key", "val")

	output := buf.String()
	// The groups and attrs must all appear.
	for _, want := range []string{"trace_id", "span_id", `"a"`, `"b"`, `"service"`, `"test-svc"`, `"key"`, `"val"`} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %s in output, got: %s", want, output)
		}
	}
}

func TestHandler_MultipleCallsDifferentContexts(t *testing.T) {
	// Create two independent spans with distinct trace/span IDs.
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx1, span1 := tp.Tracer("test").Start(context.Background(), "span-1")
	defer span1.End()
	ctx2, span2 := tp.Tracer("test").Start(context.Background(), "span-2")
	defer span2.End()

	sc1 := trace.SpanContextFromContext(ctx1)
	sc2 := trace.SpanContextFromContext(ctx2)

	// Log with ctx1, then ctx2, then ctx1 again — IDs must match their context each time.
	calls := []struct {
		ctx context.Context
		sc  trace.SpanContext
	}{
		{ctx1, sc1},
		{ctx2, sc2},
		{ctx1, sc1},
	}

	for i, c := range calls {
		var buf bytes.Buffer
		h := slogagent.NewJSONHandler(&buf, nil, nil)
		slog.New(h).InfoContext(c.ctx, "msg")

		entry := parseJSON(t, &buf)
		if got := entry["trace_id"]; got != c.sc.TraceID().String() {
			t.Errorf("call %d: trace_id = %q, want %q", i, got, c.sc.TraceID().String())
		}
		if got := entry["span_id"]; got != c.sc.SpanID().String() {
			t.Errorf("call %d: span_id = %q, want %q", i, got, c.sc.SpanID().String())
		}
	}
}

func TestHandler_UserAttrs_Preserved(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	h := slogagent.NewJSONHandler(&buf, nil, nil)
	slog.New(h).InfoContext(ctx, "hello", "user_id", 42, "action", "login")

	entry := parseJSON(t, &buf)
	if entry["user_id"] != float64(42) {
		t.Errorf("user_id = %v, want 42", entry["user_id"])
	}
	if entry["action"] != "login" {
		t.Errorf("action = %v, want login", entry["action"])
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must be present alongside user attrs")
	}
}
