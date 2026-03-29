package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "last9-agent"

// StartSpan creates a new span and returns the updated context.
// The caller must call span.End() when done.
//
// Example:
//
//	ctx, span := agent.StartSpan(ctx, "process-order")
//	defer span.End()
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, opts...)
}

// TraceFunction wraps a function with a span. The span is automatically ended
// when the function returns. If fn returns an error, it is recorded on the span
// and the span status is set to error.
//
// Example:
//
//	err := agent.TraceFunction(ctx, "validate-input", func(ctx context.Context) error {
//	    return validate(ctx, input)
//	})
func TraceFunction(ctx context.Context, name string, fn func(ctx context.Context) error) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, name)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// TraceFunctionWithResult wraps a function with a span and returns its result.
// The span is automatically ended. If fn returns an error, it is recorded on the span.
//
// Example:
//
//	user, err := agent.TraceFunctionWithResult[*User](ctx, "fetch-user", func(ctx context.Context) (*User, error) {
//	    return db.GetUser(ctx, id)
//	})
func TraceFunctionWithResult[T any](ctx context.Context, name string, fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, name)
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}
