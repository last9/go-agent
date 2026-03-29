package beego_test

import (
	gocontext "context"
	"net/http"
	"net/http/httptest"
	"testing"

	beegocontext "github.com/beego/beego/v2/server/web/context"
	beegoagent "github.com/last9/go-agent/instrumentation/beego"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

// setupTracer registers an in-memory tracer provider globally and returns the
// exporter. Spans are available immediately after span.End() (WithSyncer, not batcher).
func setupTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.Cleanup(func() { _ = tp.Shutdown(gocontext.Background()) })
	return exp
}

// newBeegoCtx builds a minimal Beego context wrapping a test request/recorder.
func newBeegoCtx(method, target string, header http.Header) *beegocontext.Context {
	req := httptest.NewRequest(method, target, nil)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	ctx := beegocontext.NewContext()
	ctx.Reset(httptest.NewRecorder(), req)
	return ctx
}

// findAttr returns the value of the first attribute with the given key, or (zero, false).
func findAttr(attrs []attribute.KeyValue, key attribute.Key) (attribute.Value, bool) {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestMiddleware_CreatesSpan(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/users", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	if len(exp.GetSpans()) != 1 {
		t.Fatalf("expected 1 span, got %d", len(exp.GetSpans()))
	}
}

func TestMiddleware_SpanName(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("POST", "/api/orders", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusCreated)
	})(ctx)

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	want := "POST /api/orders"
	if got := spans[0].Name; got != want {
		t.Errorf("span name = %q, want %q", got, want)
	}
}

func TestMiddleware_HTTPMethodAttribute(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("DELETE", "/api/items/42", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusNoContent)
	})(ctx)

	attrs := exp.GetSpans()[0].Attributes
	val, ok := findAttr(attrs, semconv.HTTPRequestMethodKey)
	if !ok {
		t.Fatal("http.request.method attribute not set")
	}
	if val.AsString() != "DELETE" {
		t.Errorf("http.request.method = %q, want DELETE", val.AsString())
	}
}

func TestMiddleware_URLPathAttribute(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/products/123", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	attrs := exp.GetSpans()[0].Attributes
	val, ok := findAttr(attrs, semconv.URLPathKey)
	if !ok {
		t.Fatal("url.path attribute not set")
	}
	if val.AsString() != "/api/products/123" {
		t.Errorf("url.path = %q, want /api/products/123", val.AsString())
	}
}

func TestMiddleware_StatusCodeAttribute(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/users", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusAccepted)
	})(ctx)

	attrs := exp.GetSpans()[0].Attributes
	val, ok := findAttr(attrs, semconv.HTTPResponseStatusCodeKey)
	if !ok {
		t.Fatal("http.response.status_code attribute not set")
	}
	if val.AsInt64() != http.StatusAccepted {
		t.Errorf("http.response.status_code = %d, want %d", val.AsInt64(), http.StatusAccepted)
	}
}

func TestMiddleware_5xxSetsSpanError(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/crash", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusInternalServerError)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.Status.Code != codes.Error {
		t.Errorf("span status = %v, want Error for 5xx", span.Status.Code)
	}
}

func TestMiddleware_4xxNoSpanError(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/missing", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusNotFound)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.Status.Code == codes.Error {
		t.Errorf("span status = Error for 404, want no error")
	}
}

func TestMiddleware_2xxNoSpanError(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/users", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.Status.Code == codes.Error {
		t.Errorf("span status = Error for 200, want no error")
	}
}

func TestMiddleware_SpanKindIsServer(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.SpanKind != trace.SpanKindServer {
		t.Errorf("span kind = %v, want Server", span.SpanKind)
	}
}

func TestMiddleware_IncomingTraceContextCreatesChildSpan(t *testing.T) {
	exp := setupTracer(t)

	// Build a parent span with a separate tracer provider and inject its context
	// into request headers via W3C traceparent.
	parentTP := sdktrace.NewTracerProvider()
	parentCtx, parentSpan := parentTP.Tracer("parent").Start(gocontext.Background(), "parent-op")
	parentSC := trace.SpanContextFromContext(parentCtx)
	parentSpan.End()

	header := make(http.Header)
	otel.GetTextMapPropagator().Inject(parentCtx, propagation.HeaderCarrier(header))

	ctx := newBeegoCtx("GET", "/api/child", header)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	span := exp.GetSpans()[0]
	if !span.Parent.IsValid() {
		t.Fatal("expected parent span context from incoming traceparent header")
	}
	if span.Parent.TraceID() != parentSC.TraceID() {
		t.Errorf("parent trace ID = %s, want %s", span.Parent.TraceID(), parentSC.TraceID())
	}
}

func TestMiddleware_NoIncomingContext_CreatesRootSpan(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/root", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.Parent.IsValid() {
		t.Errorf("expected root span (no parent), got parent %s", span.Parent.SpanID())
	}
}

func TestMiddleware_RequestContextContainsSpan(t *testing.T) {
	setupTracer(t)

	var capturedCtx gocontext.Context
	ctx := newBeegoCtx("GET", "/api/users", nil)

	beegoagent.Middleware()(func(c *beegocontext.Context) {
		capturedCtx = c.Request.Context()
		c.ResponseWriter.WriteHeader(http.StatusOK)
	})(ctx)

	if !trace.SpanContextFromContext(capturedCtx).IsValid() {
		t.Error("request context inside next handler must contain a valid span context")
	}
}

func TestMiddleware_502AlsoSetsError(t *testing.T) {
	exp := setupTracer(t)

	ctx := newBeegoCtx("GET", "/api/gateway", nil)
	beegoagent.Middleware()(func(c *beegocontext.Context) {
		c.ResponseWriter.WriteHeader(http.StatusBadGateway)
	})(ctx)

	span := exp.GetSpans()[0]
	if span.Status.Code != codes.Error {
		t.Errorf("span status = %v, want Error for 502", span.Status.Code)
	}
}
