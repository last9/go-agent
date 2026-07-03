package gqlgen_test

import (
	"context"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/testserver"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/last9/go-agent/instrumentation/gqlgen"
	"github.com/last9/go-agent/tests/testutil"
)

// operationContext builds a minimal graphql.OperationContext for testing
// InterceptResponse without a live gqlgen server.
func operationContext(operation ast.Operation, name, rawQuery string) *graphql.OperationContext {
	return &graphql.OperationContext{
		RawQuery: rawQuery,
		Operation: &ast.OperationDefinition{
			Operation: operation,
			Name:      name,
		},
	}
}

func withOperationContext(oc *graphql.OperationContext) context.Context {
	return graphql.WithOperationContext(context.Background(), oc)
}

func TestInterceptResponse_HappyPath(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", "query GetUser { user { id } }"))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{Data: []byte(`{"user":{"id":"1"}}`)}
	}

	resp := tracer.InterceptResponse(ctx, next)
	require.NotNil(t, resp)

	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 1)
	require.Len(t, spans, 1)
	assert.Equal(t, "query GetUser", spans[0].Name())
	assert.Equal(t, codes.Ok.String(), spans[0].Status().Code.String(), "happy path should be Ok")
	testutil.AssertSpanAttribute(t, spans[0], "graphql.operation.name", "GetUser")
	testutil.AssertSpanAttribute(t, spans[0], "graphql.operation.type", "query")
}

func TestInterceptResponse_ErrorPath_RedactedByDefault(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{IncludeQueryDocument: false})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", "query GetUser { user(email: \"a@b.com\") { id } }"))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{
			Errors: gqlerror.List{{Message: "user with email a@b.com not found"}},
		}
	}

	tracer.InterceptResponse(ctx, next)

	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 1)
	require.Len(t, spans, 1)
	assert.Equal(t, "Error", spans[0].Status().Code.String())
	assert.NotContains(t, spans[0].Status().Description, "a@b.com", "raw error text must not appear when IncludeQueryDocument is false")
	testutil.AssertSpanAttributeInt(t, spans[0], "graphql.error.count", 1)
	testutil.AssertSpanAttribute(t, spans[0], "error.type", "GraphQLError")
}

func TestInterceptResponse_ErrorPath_IncludedWhenOptedIn(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{IncludeQueryDocument: true})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", "query GetUser { user { id } }"))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{
			Errors: gqlerror.List{{Message: "user with email a@b.com not found"}},
		}
	}

	tracer.InterceptResponse(ctx, next)

	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 1)
	require.Len(t, spans, 1)
	assert.Contains(t, spans[0].Status().Description, "a@b.com")
}

func TestInterceptResponse_MultipleErrors_CountMatches(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{})
	ctx := withOperationContext(operationContext(ast.Mutation, "CreateTeam", ""))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{
			Errors: gqlerror.List{{Message: "error one"}, {Message: "error two"}, {Message: "error three"}},
		}
	}

	tracer.InterceptResponse(ctx, next)

	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 1)
	require.Len(t, spans, 1)
	testutil.AssertSpanAttributeInt(t, spans[0], "graphql.error.count", 3)
}

func TestInterceptResponse_NilResponse_NoPanic(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", ""))

	next := func(context.Context) *graphql.Response { return nil }

	assert.NotPanics(t, func() {
		resp := tracer.InterceptResponse(ctx, next)
		assert.Nil(t, resp)
	})

	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 1)
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Ok.String(), spans[0].Status().Code.String())
}

func TestInterceptResponse_NoOperationContext_PassesThrough(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{})

	called := false
	next := func(context.Context) *graphql.Response {
		called = true
		return &graphql.Response{}
	}

	tracer.InterceptResponse(context.Background(), next)

	assert.True(t, called, "next should still be invoked without an operation context")
	testutil.AssertSpanCount(t, collector.GetSpans(), 0)
}

func TestInterceptResponse_Subscription_NoSpanAcrossMultipleMessages(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{})
	ctx := withOperationContext(operationContext(ast.Subscription, "OnMessage", ""))

	callCount := 0
	next := func(context.Context) *graphql.Response {
		callCount++
		return &graphql.Response{Data: []byte(`{"onMessage":{}}`)}
	}

	// Simulate multiple streamed messages for a single subscription.
	for i := 0; i < 3; i++ {
		tracer.InterceptResponse(ctx, next)
	}

	assert.Equal(t, 3, callCount)
	testutil.AssertSpanCount(t, collector.GetSpans(), 0)
}

func TestInterceptResponse_ErrorText_Truncated(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	tracer := gqlgen.New(gqlgen.Config{IncludeQueryDocument: true})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", ""))

	bigMessage := strings.Repeat("e", 9000)
	next := func(context.Context) *graphql.Response {
		return &graphql.Response{Errors: gqlerror.List{{Message: bigMessage}}}
	}

	tracer.InterceptResponse(ctx, next)

	spans := collector.GetSpans()
	require.Len(t, spans, 1)
	assert.Less(t, len(spans[0].Status().Description), len(bigMessage), "error status description should be truncated")
}

// TestInterceptResponse_NestsUnderParentSpan verifies the GraphQL operation
// span nests under whatever SERVER span is already active in context — the
// same relationship an HTTP framework instrumentation (chi, gin, etc.) would
// establish before gqlgen's handler runs. R1 requires this nesting.
func TestInterceptResponse_NestsUnderParentSpan(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	parentCtx, parentSpan := otel.Tracer("test-http-framework").Start(context.Background(), "/graphql")

	tracer := gqlgen.New(gqlgen.Config{})
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", ""))
	ctx = graphql.WithOperationContext(parentCtx, graphql.GetOperationContext(ctx))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{Data: []byte(`{}`)}
	}
	tracer.InterceptResponse(ctx, next)
	parentSpan.End()

	spans := collector.GetSpans()
	require.Len(t, spans, 2)

	var parent, child sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == "/graphql" {
			parent = s
		} else {
			child = s
		}
	}
	require.NotNil(t, parent)
	require.NotNil(t, child)
	testutil.AssertParentChild(t, parent, child)
}

// TestTracer_ZeroValue_ResolvesTracerFromGlobalProvider verifies a
// directly-constructed Tracer{} (bypassing New/Use) does not panic and
// still produces a span, via resolveTracer's nil fallback.
func TestTracer_ZeroValue_ResolvesTracerFromGlobalProvider(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	var tracer gqlgen.Tracer
	ctx := withOperationContext(operationContext(ast.Query, "GetUser", ""))

	next := func(context.Context) *graphql.Response {
		return &graphql.Response{Data: []byte(`{}`)}
	}

	assert.NotPanics(t, func() {
		tracer.InterceptResponse(ctx, next)
	})

	spans := collector.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "query GetUser", spans[0].Name())
}

func TestTracer_SatisfiesHandlerExtension(t *testing.T) {
	var _ graphql.HandlerExtension = gqlgen.New(gqlgen.Config{})
	var _ graphql.ResponseInterceptor = gqlgen.New(gqlgen.Config{})
}

func TestUse_DefaultConfig(t *testing.T) {
	srv := handler.New(&graphql.ExecutableSchemaMock{})
	assert.NotPanics(t, func() {
		gqlgen.Use(srv, gqlgen.Config{})
	})
}

func TestUse_IncludeQueryDocumentEnabled(t *testing.T) {
	srv := handler.New(&graphql.ExecutableSchemaMock{})
	assert.NotPanics(t, func() {
		gqlgen.Use(srv, gqlgen.Config{IncludeQueryDocument: true})
	})
}

func TestUse_BeforeAgentStart_DoesNotPanic(t *testing.T) {
	srv := handler.New(&graphql.ExecutableSchemaMock{})
	assert.NotPanics(t, func() {
		gqlgen.Use(srv, gqlgen.Config{})
	})
}

// TestUse_EndToEnd wires the tracer into a real gqlgen server via Use and
// drives an actual request through gqlgen's own response-interceptor chain
// (gqlgen's handler/testserver + client packages), confirming the extension
// is genuinely invoked at request time — not just that construction doesn't
// panic.
func TestUse_EndToEnd(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	srv := testserver.New()
	srv.AddTransport(transport.POST{})
	gqlgen.Use(srv.Server, gqlgen.Config{})
	c := client.New(srv)

	var resp struct {
		Name string
	}
	c.MustPost(`query GetName { name }`, &resp)

	spans := collector.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "query GetName", spans[0].Name())
	testutil.AssertSpanAttribute(t, spans[0], "graphql.operation.type", "query")
	testutil.AssertSpanAttribute(t, spans[0], "graphql.operation.name", "GetName")
}

// TestUse_EndToEnd_ErrorPath drives a real request that produces a GraphQL
// error (testserver's Mutation always errors) through the wired extension.
func TestUse_EndToEnd_ErrorPath(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	srv := testserver.New()
	srv.AddTransport(transport.POST{})
	gqlgen.Use(srv.Server, gqlgen.Config{})
	c := client.New(srv)

	var resp struct {
		Name string
	}
	err := c.Post(`mutation { name }`, &resp)
	require.Error(t, err)

	spans := collector.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error.String(), spans[0].Status().Code.String())
	testutil.AssertSpanAttributeInt(t, spans[0], "graphql.error.count", 1)
}
