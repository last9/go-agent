// Package slog provides Last9 instrumentation for Go's log/slog package.
//
// It wraps any slog.Handler to automatically inject OpenTelemetry trace_id
// and span_id attributes into every log record when a valid span context
// is present in the log call's context.
//
// Basic usage - wrap an existing handler:
//
//	import (
//	    "log/slog"
//	    slogagent "github.com/last9/go-agent/instrumentation/slog"
//	)
//
//	handler := slogagent.NewJSONHandler(os.Stdout, nil, nil)
//	logger := slog.New(handler)
//	logger.InfoContext(ctx, "user created", "user_id", 42)
//	// Output: {"time":"...","level":"INFO","msg":"user created","user_id":42,"trace_id":"abc...","span_id":"def..."}
//
// Set as the global default logger:
//
//	slogagent.SetDefault(os.Stdout, nil, nil)
//	slog.InfoContext(ctx, "started") // automatically has trace_id and span_id
//
// Custom attribute key names:
//
//	handler := slogagent.NewJSONHandler(os.Stdout, nil, &slogagent.Options{
//	    TraceKey: "dd.trace_id",
//	    SpanKey:  "dd.span_id",
//	})
//
// Note on package naming: this package is named "slog" which shadows the
// standard library "log/slog". Import it with an alias when using both:
//
//	import (
//	    "log/slog"
//	    slogagent "github.com/last9/go-agent/instrumentation/slog"
//	)
package slog

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

const (
	// defaultTraceKey is the log attribute key for the OTel trace ID.
	defaultTraceKey = "trace_id"

	// defaultSpanKey is the log attribute key for the OTel span ID.
	defaultSpanKey = "span_id"
)

// Options configures the behavior of the Handler.
// All fields are optional; zero value produces sensible defaults.
type Options struct {
	// TraceKey is the log attribute key used for the trace ID.
	// Defaults to "trace_id" if empty.
	TraceKey string

	// SpanKey is the log attribute key used for the span ID.
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

// Handler is an slog.Handler that injects OpenTelemetry trace_id and span_id
// into every log record when the context contains a valid span context.
//
// Handler is safe for concurrent use. The zero value is not usable;
// use NewHandler, NewJSONHandler, or NewTextHandler.
type Handler struct {
	inner    slog.Handler
	traceKey string
	spanKey  string
}

// Compile-time assertion that Handler implements slog.Handler.
var _ slog.Handler = (*Handler)(nil)

// NewHandler wraps inner with OTel trace correlation injection.
// opts may be nil, in which case defaults are used.
// Panics if inner is nil.
//
// Example:
//
//	base := slog.NewJSONHandler(os.Stdout, nil)
//	h := slogagent.NewHandler(base, nil)
//	logger := slog.New(h)
func NewHandler(inner slog.Handler, opts *Options) *Handler {
	if inner == nil {
		panic("slog: NewHandler: inner handler must not be nil")
	}
	return &Handler{
		inner:    inner,
		traceKey: opts.resolvedTraceKey(),
		spanKey:  opts.resolvedSpanKey(),
	}
}

// NewJSONHandler creates a Handler that wraps slog.NewJSONHandler(w, handlerOpts).
// This is the most common drop-in replacement for slog.NewJSONHandler.
//
// handlerOpts and opts may both be nil to use defaults.
func NewJSONHandler(w io.Writer, handlerOpts *slog.HandlerOptions, opts *Options) *Handler {
	return NewHandler(slog.NewJSONHandler(w, handlerOpts), opts)
}

// NewTextHandler creates a Handler that wraps slog.NewTextHandler(w, handlerOpts).
// This is the most common drop-in replacement for slog.NewTextHandler.
//
// handlerOpts and opts may both be nil to use defaults.
func NewTextHandler(w io.Writer, handlerOpts *slog.HandlerOptions, opts *Options) *Handler {
	return NewHandler(slog.NewTextHandler(w, handlerOpts), opts)
}

// SetDefault creates a slog.Logger backed by a JSON handler wrapped with OTel
// trace correlation, and sets it as the global default via slog.SetDefault.
// Returns the logger for direct use.
//
// Example:
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//	    slogagent.SetDefault(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}, nil)
//
//	    // From here, slog.InfoContext(ctx, ...) will include trace_id/span_id
//	}
func SetDefault(w io.Writer, handlerOpts *slog.HandlerOptions, opts *Options) *slog.Logger {
	h := NewJSONHandler(w, handlerOpts, opts)
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

// Enabled implements slog.Handler. It delegates to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler. If the context contains a valid OTel span
// context, it appends trace_id and span_id attributes to a clone of the record
// before delegating to the inner handler.
//
// Injection occurs when sc.IsValid() is true, which requires both a non-zero
// TraceID and a non-zero SpanID. Sampled-out spans (valid but not recording)
// still get their IDs injected, which is correct for log-trace correlation.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String(h.traceKey, sc.TraceID().String()),
			slog.String(h.spanKey, sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs implements slog.Handler. Returns a new Handler that wraps
// inner.WithAttrs(attrs), preserving trace injection on subsequent records.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		inner:    h.inner.WithAttrs(attrs),
		traceKey: h.traceKey,
		spanKey:  h.spanKey,
	}
}

// WithGroup implements slog.Handler. Returns a new Handler that wraps
// inner.WithGroup(name), scoping subsequent attributes under the group name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		inner:    h.inner.WithGroup(name),
		traceKey: h.traceKey,
		spanKey:  h.spanKey,
	}
}
