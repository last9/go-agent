package zap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	zapagent "github.com/last9/go-agent/instrumentation/zap"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newTestLogger creates a *zap.Logger that writes JSON to buf with
// deterministic output (no timestamp, no caller).
func newTestLogger(buf *bytes.Buffer) *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = ""
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(buf),
		zapcore.DebugLevel,
	)
	return zap.New(core)
}

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

// ---------------------------------------------------------------------------
// TraceFields tests
// ---------------------------------------------------------------------------

func TestTraceFields_NoSpan_ReturnsNil(t *testing.T) {
	fields := zapagent.TraceFields(context.Background())
	if fields != nil {
		t.Errorf("expected nil fields when no span, got %v", fields)
	}
}

func TestTraceFields_ActiveSpan(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	fields := zapagent.TraceFields(ctx)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	// Verify the fields produce correct JSON when logged.
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("hello", fields...)

	entry := parseJSON(t, &buf)
	traceID, ok := entry["trace_id"].(string)
	if !ok || len(traceID) != 32 {
		t.Errorf("trace_id must be 32 hex chars, got %q", traceID)
	}
	spanID, ok := entry["span_id"].(string)
	if !ok || len(spanID) != 16 {
		t.Errorf("span_id must be 16 hex chars, got %q", spanID)
	}
}

func TestTraceFields_SampledOut(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(context.Background(), "sampled-out")
	defer span.End()

	fields := zapagent.TraceFields(ctx)
	if len(fields) != 2 {
		t.Errorf("expected 2 fields for sampled-out span (sc.IsValid() == true), got %d", len(fields))
	}
}

func TestTraceFields_RemoteSpanContext(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), sc)

	fields := zapagent.TraceFields(ctx)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	// Log and verify exact IDs.
	var buf bytes.Buffer
	newTestLogger(&buf).Info("remote", fields...)
	entry := parseJSON(t, &buf)
	if got := entry["trace_id"]; got != traceID.String() {
		t.Errorf("trace_id = %q, want %q", got, traceID.String())
	}
	if got := entry["span_id"]; got != spanID.String() {
		t.Errorf("span_id = %q, want %q", got, spanID.String())
	}
}

// ---------------------------------------------------------------------------
// Logger wrapper tests
// ---------------------------------------------------------------------------

func TestLogger_NoSpan_NoInjection(t *testing.T) {
	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), nil)

	l.InfoContext(context.Background(), "hello")

	entry := parseJSON(t, &buf)
	if _, ok := entry["trace_id"]; ok {
		t.Error("trace_id must not be present when no span is active")
	}
	if _, ok := entry["span_id"]; ok {
		t.Error("span_id must not be present when no span is active")
	}
}

func TestLogger_ActiveSpan_InjectsIDs(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), nil)
	l.InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	traceID, ok := entry["trace_id"].(string)
	if !ok || len(traceID) != 32 {
		t.Fatalf("expected 32-char trace_id, got %v", entry["trace_id"])
	}
	spanID, ok := entry["span_id"].(string)
	if !ok || len(spanID) != 16 {
		t.Fatalf("expected 16-char span_id, got %v", entry["span_id"])
	}
}

func TestLogger_CustomKeys(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), &zapagent.Options{
		TraceKey: "dd.trace_id",
		SpanKey:  "dd.span_id",
	})
	l.InfoContext(ctx, "hello")

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

func TestLogger_With_PreservesInjection(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), nil)
	l2 := l.With(zap.String("service", "my-svc"))
	l2.InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	if entry["service"] != "my-svc" {
		t.Error("pre-set field 'service' must be present")
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must still be injected after With")
	}
	if _, ok := entry["span_id"]; !ok {
		t.Error("span_id must still be injected after With")
	}
}

func TestLogger_Named_PreservesInjection(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), nil)
	named := l.Named("subsystem")
	named.InfoContext(ctx, "hello")

	entry := parseJSON(t, &buf)
	if entry["logger"] != "subsystem" {
		t.Errorf("expected logger name 'subsystem', got %v", entry["logger"])
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must still be injected after Named")
	}
}

func TestLogger_UserFields_Preserved(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	var buf bytes.Buffer
	l := zapagent.New(newTestLogger(&buf), nil)
	l.InfoContext(ctx, "hello", zap.Int("user_id", 42), zap.String("action", "login"))

	entry := parseJSON(t, &buf)
	if entry["user_id"] != float64(42) {
		t.Errorf("user_id = %v, want 42", entry["user_id"])
	}
	if entry["action"] != "login" {
		t.Errorf("action = %v, want login", entry["action"])
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must be present alongside user fields")
	}
}

func TestLogger_MultipleContexts(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx1, span1 := tp.Tracer("test").Start(context.Background(), "span-1")
	defer span1.End()
	ctx2, span2 := tp.Tracer("test").Start(context.Background(), "span-2")
	defer span2.End()

	sc1 := trace.SpanContextFromContext(ctx1)
	sc2 := trace.SpanContextFromContext(ctx2)

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
		l := zapagent.New(newTestLogger(&buf), nil)
		l.InfoContext(c.ctx, "msg")

		entry := parseJSON(t, &buf)
		if got := entry["trace_id"]; got != c.sc.TraceID().String() {
			t.Errorf("call %d: trace_id = %q, want %q", i, got, c.sc.TraceID().String())
		}
		if got := entry["span_id"]; got != c.sc.SpanID().String() {
			t.Errorf("call %d: span_id = %q, want %q", i, got, c.sc.SpanID().String())
		}
	}
}

func TestLogger_ConcurrentCalls(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	// Use a locked writer because bytes.Buffer is not thread-safe and
	// zap's core does not serialize writes internally (unlike slog).
	var buf bytes.Buffer
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = ""
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(zapcore.AddSync(&buf)),
		zapcore.DebugLevel,
	)
	l := zapagent.New(zap.New(core), nil)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			l.InfoContext(ctx, "concurrent", zap.Int("n", n))
		}(i)
	}
	wg.Wait()

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

func TestLogger_AllLevels(t *testing.T) {
	ctx, _, cleanup := startSpan(t)
	defer cleanup()

	// Test each level individually to isolate output.
	levels := []struct {
		log  func(*zapagent.Logger, context.Context)
		name string
	}{
		{func(l *zapagent.Logger, ctx context.Context) { l.DebugContext(ctx, "d") }, "debug"},
		{func(l *zapagent.Logger, ctx context.Context) { l.WarnContext(ctx, "w") }, "warn"},
		{func(l *zapagent.Logger, ctx context.Context) { l.ErrorContext(ctx, "e") }, "error"},
		// DPanic only panics in development mode; production mode (default) logs normally.
		{func(l *zapagent.Logger, ctx context.Context) { l.DPanicContext(ctx, "dp") }, "dpanic"},
	}

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := zapagent.New(newTestLogger(&buf), nil)
			tt.log(l, ctx)

			entry := parseJSON(t, &buf)
			if _, ok := entry["trace_id"]; !ok {
				t.Errorf("%s: missing trace_id", tt.name)
			}
			if _, ok := entry["span_id"]; !ok {
				t.Errorf("%s: missing span_id", tt.name)
			}
		})
	}
}

func TestNew_NilBase_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("New with nil base must panic")
		}
	}()
	zapagent.New(nil, nil)
}

func TestLogger_Unwrap(t *testing.T) {
	base := zap.NewNop()
	l := zapagent.New(base, nil)
	if l.Unwrap() != base {
		t.Error("Unwrap must return the base logger")
	}
}
