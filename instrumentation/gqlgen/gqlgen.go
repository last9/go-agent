package gqlgen

import (
	"context"
	"unicode/utf8"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
)

const (
	tracerName = "github.com/last9/go-agent/instrumentation/gqlgen"

	// errorTypeKey and graphqlErrorCountKey are not covered by
	// go.opentelemetry.io/otel/semconv/v1.25.0 (error.type was standardized
	// in a later semconv revision, and graphql.error.count is not an OTel
	// semantic convention at all). Declared locally, matching the shape
	// Last9's mobile RUM SDK already ships for GraphQL errors, so backend and
	// mobile traces carry the same attribute shape.
	errorTypeKey         = attribute.Key("error.type")
	graphqlErrorCountKey = attribute.Key("graphql.error.count")

	graphQLErrorType = "GraphQLError"

	// maxCapturedTextLen bounds graphql.document and the raw GraphQL error
	// text (both only captured when IncludeQueryDocument is true) so a large
	// introspection query or an operation with many field-level errors can't
	// grow a single span large enough to push an OTLP batch export over the
	// collector's message-size limit and drop telemetry for unrelated
	// requests sharing that batch.
	maxCapturedTextLen = 8 * 1024
)

// truncate caps s to maxCapturedTextLen, appending a marker when truncated.
// The cut point backs up to the nearest rune boundary so a multi-byte UTF-8
// character at the cutoff is never split, which would otherwise emit an
// invalid-UTF-8 tail into the span attribute or status description.
func truncate(s string) string {
	if len(s) <= maxCapturedTextLen {
		return s
	}
	cut := maxCapturedTextLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "...(truncated)"
}

// Config configures the gqlgen instrumentation.
type Config struct {
	// IncludeQueryDocument, when true, includes the raw GraphQL query
	// document text and raw GraphQL error messages in spans.
	// Disable in production environments that handle PII or sensitive data.
	IncludeQueryDocument bool
}

// Tracer implements graphql.HandlerExtension and graphql.ResponseInterceptor,
// creating one INTERNAL span per GraphQL operation. Field-level resolver
// spans and GraphQL subscriptions are not instrumented.
//
// The zero value Tracer{} is valid: InterceptResponse falls back to the
// globally registered OTel tracer provider when tracer is nil, so
// constructing a Tracer directly (bypassing New/Use) cannot panic.
type Tracer struct {
	cfg    Config
	tracer oteltrace.Tracer
}

// resolveTracer returns t.tracer, or resolves it from the current global
// tracer provider when t was constructed as a zero value.
func (t Tracer) resolveTracer() oteltrace.Tracer {
	if t.tracer != nil {
		return t.tracer
	}
	return otel.Tracer(tracerName)
}

var (
	_ graphql.HandlerExtension    = Tracer{}
	_ graphql.ResponseInterceptor = Tracer{}
)

// New creates a Tracer configured per cfg, using whatever OTel tracer
// provider is currently registered globally. Most callers should use Use
// instead; New does not start the agent — it is exposed for composing with
// other gqlgen extensions, or in tests that configure their own tracer
// provider (e.g. a MockCollector) and need it to take effect immediately.
func New(cfg Config) Tracer {
	return Tracer{
		cfg:    cfg,
		tracer: otel.Tracer(tracerName),
	}
}

// Use wires Last9 instrumentation into a gqlgen server.
//
// IMPORTANT: This does NOT require agent.Start() to have been called first —
// it starts the agent automatically if needed, matching instrumentation/chi's
// ensureAgentStarted guarantee.
//
// Example:
//
//	srv := handler.NewDefaultServer(schema)
//	gqlgenagent.Use(srv, gqlgen.Config{})
func Use(srv *handler.Server, cfg Config) {
	ensureAgentStarted()
	srv.Use(New(cfg))
}

// ExtensionName returns the extension's identifier, shown in gqlgen's stats
// and logging.
func (t Tracer) ExtensionName() string {
	return "Last9GraphQL"
}

// Validate is a no-op; this extension does not depend on schema shape.
func (t Tracer) Validate(_ graphql.ExecutableSchema) error {
	return nil
}

// InterceptResponse creates one INTERNAL span per GraphQL operation. It
// passes through without creating a span for subscriptions, since gqlgen
// invokes ResponseInterceptor once per streamed message for a subscription's
// lifetime, not once per operation (R4).
func (t Tracer) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	if !graphql.HasOperationContext(ctx) {
		return next(ctx)
	}
	oc := graphql.GetOperationContext(ctx)
	if isSubscription(oc) {
		return next(ctx)
	}

	ctx, span := t.resolveTracer().Start(ctx, spanName(oc), oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()
	span.SetAttributes(baseAttributes(oc, t.cfg.IncludeQueryDocument)...)

	resp := next(ctx)

	if resp != nil && len(resp.Errors) > 0 {
		errDesc := "graphql response errors"
		if t.cfg.IncludeQueryDocument {
			errDesc = truncate(resp.Errors.Error())
		}
		span.SetStatus(codes.Error, errDesc)
		span.SetAttributes(
			graphqlErrorCountKey.Int(len(resp.Errors)),
			errorTypeKey.String(graphQLErrorType),
		)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return resp
}

// ensureAgentStarted starts the agent if not already initialized, mirroring
// instrumentation/chi's guarantee that this package works even if the user
// forgot to call agent.Start() first.
func ensureAgentStarted() {
	if !agent.IsInitialized() {
		_ = agent.Start()
	}
}
