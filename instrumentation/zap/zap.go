// Package zap provides Last9 instrumentation for Uber's zap logging library.
//
// It provides two ways to inject OpenTelemetry trace_id and span_id into
// zap log entries when a valid span context is present.
//
// TraceFields helper — works with any existing *zap.Logger:
//
//	import (
//	    "go.uber.org/zap"
//	    zapagent "github.com/last9/go-agent/instrumentation/zap"
//	)
//
//	logger.Info("request handled", zapagent.TraceFields(ctx)...)
//
// Logger wrapper — provides *Context methods for a drop-in experience:
//
//	import (
//	    "go.uber.org/zap"
//	    zapagent "github.com/last9/go-agent/instrumentation/zap"
//	)
//
//	l := zapagent.New(logger, nil)
//	l.InfoContext(ctx, "user created", zap.String("user_id", "42"))
//	// Output: {"level":"info","msg":"user created","user_id":"42","trace_id":"abc...","span_id":"def..."}
//
// Custom attribute key names:
//
//	l := zapagent.New(logger, &zapagent.Options{
//	    TraceKey: "dd.trace_id",
//	    SpanKey:  "dd.span_id",
//	})
//
// Note on package naming: this package is named "zap" which shadows
// "go.uber.org/zap". Import it with an alias when using both:
//
//	import (
//	    "go.uber.org/zap"
//	    zapagent "github.com/last9/go-agent/instrumentation/zap"
//	)
package zap

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	// defaultTraceKey is the log field key for the OTel trace ID.
	defaultTraceKey = "trace_id"

	// defaultSpanKey is the log field key for the OTel span ID.
	defaultSpanKey = "span_id"
)

// Options configures the behavior of the Logger wrapper.
// All fields are optional; zero value produces sensible defaults.
type Options struct {
	// TraceKey is the log field key used for the trace ID.
	// Defaults to "trace_id" if empty.
	TraceKey string

	// SpanKey is the log field key used for the span ID.
	// Defaults to "span_id" if empty.
	SpanKey string
}

func (o *Options) resolvedTraceKey() string {
	if o != nil && o.TraceKey != "" {
		return o.TraceKey
	}
	return defaultTraceKey
}

func (o *Options) resolvedSpanKey() string {
	if o != nil && o.SpanKey != "" {
		return o.SpanKey
	}
	return defaultSpanKey
}

// TraceFields extracts the OTel span context from ctx and returns zap fields
// for trace_id and span_id. Returns nil if the span context is not valid.
//
// This is the simplest way to add trace correlation to existing zap code:
//
//	logger.Info("request handled",
//	    zap.String("path", r.URL.Path),
//	    zapagent.TraceFields(ctx)...,
//	)
//
// For custom field key names, use the Logger wrapper with Options instead.
func TraceFields(ctx context.Context) []zap.Field {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []zap.Field{
		zap.String(defaultTraceKey, sc.TraceID().String()),
		zap.String(defaultSpanKey, sc.SpanID().String()),
	}
}

// Logger wraps a *zap.Logger to provide *Context methods that automatically
// inject OTel trace_id and span_id fields into every log entry.
//
// Logger is safe for concurrent use. The zero value is not usable;
// use New to create one.
type Logger struct {
	base     *zap.Logger
	traceKey string
	spanKey  string
}

// New creates a Logger wrapping base with OTel trace correlation injection.
// opts may be nil, in which case defaults are used.
// Panics if base is nil.
//
// Example:
//
//	base, _ := zap.NewProduction()
//	l := zapagent.New(base, nil)
//	l.InfoContext(ctx, "started")
func New(base *zap.Logger, opts *Options) *Logger {
	if base == nil {
		panic("zap: New: base logger must not be nil")
	}
	return &Logger{
		base:     base,
		traceKey: opts.resolvedTraceKey(),
		spanKey:  opts.resolvedSpanKey(),
	}
}

// appendTraceFields returns fields with trace_id and span_id appended if a
// valid span context exists in ctx. Allocates a new slice to avoid mutating
// the caller's backing array.
func (l *Logger) appendTraceFields(ctx context.Context, fields []zap.Field) []zap.Field {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return fields
	}
	traceField := zap.String(l.traceKey, sc.TraceID().String())
	spanField := zap.String(l.spanKey, sc.SpanID().String())
	if len(fields) == 0 {
		return []zap.Field{traceField, spanField}
	}
	result := make([]zap.Field, len(fields), len(fields)+2)
	copy(result, fields)
	return append(result, traceField, spanField)
}

// DebugContext logs a message at DebugLevel with trace correlation.
func (l *Logger) DebugContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Debug(msg, l.appendTraceFields(ctx, fields)...)
}

// InfoContext logs a message at InfoLevel with trace correlation.
func (l *Logger) InfoContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Info(msg, l.appendTraceFields(ctx, fields)...)
}

// WarnContext logs a message at WarnLevel with trace correlation.
func (l *Logger) WarnContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Warn(msg, l.appendTraceFields(ctx, fields)...)
}

// ErrorContext logs a message at ErrorLevel with trace correlation.
func (l *Logger) ErrorContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Error(msg, l.appendTraceFields(ctx, fields)...)
}

// DPanicContext logs a message at DPanicLevel with trace correlation.
// In development mode, the logger then panics (DPanic means "development panic").
func (l *Logger) DPanicContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.DPanic(msg, l.appendTraceFields(ctx, fields)...)
}

// PanicContext logs a message at PanicLevel with trace correlation, then panics.
func (l *Logger) PanicContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Panic(msg, l.appendTraceFields(ctx, fields)...)
}

// FatalContext logs a message at FatalLevel with trace correlation, then
// calls os.Exit(1).
func (l *Logger) FatalContext(ctx context.Context, msg string, fields ...zap.Field) {
	l.base.Fatal(msg, l.appendTraceFields(ctx, fields)...)
}

// With creates a new Logger with the given fields pre-set on the base logger.
// The returned Logger continues to inject trace fields on *Context calls.
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{
		base:     l.base.With(fields...),
		traceKey: l.traceKey,
		spanKey:  l.spanKey,
	}
}

// Named adds a sub-scope to the logger's name.
// The returned Logger continues to inject trace fields on *Context calls.
func (l *Logger) Named(name string) *Logger {
	return &Logger{
		base:     l.base.Named(name),
		traceKey: l.traceKey,
		spanKey:  l.spanKey,
	}
}

// Unwrap returns the underlying *zap.Logger for use with code that
// expects the standard zap type.
func (l *Logger) Unwrap() *zap.Logger {
	return l.base
}
