package codeattr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

// newTestProvider returns a SpanRecorder and a Tracer backed by the codeattr Processor.
func newTestProvider(t *testing.T) (*tracetest.SpanRecorder, trace.Tracer) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(New()),
		sdktrace.WithSpanProcessor(rec),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return rec, tp.Tracer("test")
}

// allowAgentFrames temporarily removes the go-agent module prefix from
// skipPrefixes so that test functions in this module appear as application
// frames during stack walking.
func allowAgentFrames(t *testing.T) {
	t.Helper()
	orig := skipPrefixes
	filtered := make([]string, 0, len(orig))
	for _, p := range orig {
		if p != "github.com/last9/go-agent" {
			filtered = append(filtered, p)
		}
	}
	skipPrefixes = filtered
	t.Cleanup(func() { skipPrefixes = orig })
}

func spanHasAttr(s sdktrace.ReadOnlySpan, key string) bool {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return true
		}
	}
	return false
}

func spanAttrString(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// TestProcessor_OutboundSpanKindsGetCodeAttributes verifies that Client,
// Producer, and Consumer spans all receive code.* attributes.
//
// allowAgentFrames removes the broad module skip so that frames inside this
// package are visible. Because callerFrame itself lives in processor.go (same
// package), the first frame found will be processor.go — that is expected and
// still validates that the processor correctly enriches the span. Exact
// filepath/function values are covered by TestIsSkipped and TestShortFuncName.
func TestProcessor_OutboundSpanKindsGetCodeAttributes(t *testing.T) {
	kinds := []trace.SpanKind{
		trace.SpanKindClient,
		trace.SpanKindProducer,
		trace.SpanKindConsumer,
	}
	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			allowAgentFrames(t)
			rec, tracer := newTestProvider(t)

			_, span := tracer.Start(context.Background(), "test", trace.WithSpanKind(kind))
			span.End()

			spans := rec.Ended()
			require.Len(t, spans, 1)
			s := spans[0]

			assert.True(t, spanHasAttr(s, string(semconv.CodeFunctionKey)), "code.function missing for %s span", kind)
			assert.True(t, spanHasAttr(s, string(semconv.CodeFilepathKey)), "code.filepath missing for %s span", kind)
			assert.True(t, spanHasAttr(s, string(semconv.CodeLineNumberKey)), "code.lineno missing for %s span", kind)

			fn := spanAttrString(s, string(semconv.CodeFunctionKey))
			assert.NotEmpty(t, fn, "code.function should be non-empty for %s span", kind)
		})
	}
}

// TestProcessor_ServerAndInternalSpansSkipped verifies that Server, Internal,
// and Unspecified spans are not enriched.
func TestProcessor_ServerAndInternalSpansSkipped(t *testing.T) {
	kinds := []trace.SpanKind{
		trace.SpanKindServer,
		trace.SpanKindInternal,
		trace.SpanKindUnspecified,
	}
	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			rec, tracer := newTestProvider(t)

			_, span := tracer.Start(context.Background(), "test", trace.WithSpanKind(kind))
			span.End()

			spans := rec.Ended()
			require.Len(t, spans, 1)
			s := spans[0]

			assert.False(t, spanHasAttr(s, string(semconv.CodeFunctionKey)), "code.function should not be set for %s span", kind)
			assert.False(t, spanHasAttr(s, string(semconv.CodeFilepathKey)), "code.filepath should not be set for %s span", kind)
			assert.False(t, spanHasAttr(s, string(semconv.CodeLineNumberKey)), "code.lineno should not be set for %s span", kind)
		})
	}
}

func TestIsSkipped(t *testing.T) {
	cases := []struct {
		fn      string
		skipped bool
	}{
		// OTel SDK
		{"go.opentelemetry.io/otel/sdk/trace.(*tracer).Start", true},
		{"go.nhat.io/otelsql.(*driver).Open", true},
		// This agent
		{"github.com/last9/go-agent/instrumentation/codeattr.(*Processor).OnStart", true},
		// Stdlib
		{"database/sql.(*DB).QueryContext", true},
		{"net/http.(*Transport).roundTrip", true},
		{"runtime.Callers", true},
		{"testing.tRunner", true},
		{"reflect.Value.Call", true},
		// Drivers
		{"github.com/IBM/sarama.(*client).Brokers", true},
		{"github.com/redis/go-redis/v9.(*Client).Get", true},
		{"go.mongodb.org/mongo-driver/mongo.(*Collection).Find", true},
		{"github.com/aws/aws-sdk-go-v2/service/s3.(*Client).GetObject", true},
		// Application frames — must NOT be skipped
		{"main.(*Server).handleRequest", false},
		{"github.com/myorg/myapp/handlers.(*UserHandler).GetUser", false},
		{"mypackage.doWork", false},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			assert.Equal(t, tc.skipped, isSkipped(tc.fn))
		})
	}
}

func TestShortFuncName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"github.com/myorg/app/handlers.(*Server).Handle", "handlers.(*Server).Handle"},
		{"github.com/last9/go-agent/instrumentation/codeattr.(*Processor).OnStart", "codeattr.(*Processor).OnStart"},
		{"main.doWork", "main.doWork"},
		{"runtime.Callers", "runtime.Callers"},
		{"noSlash", "noSlash"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, shortFuncName(tc.input))
		})
	}
}
