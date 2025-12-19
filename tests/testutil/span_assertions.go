package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// AssertSpanCount verifies that the expected number of spans were captured.
//
// Example:
//
//	spans := collector.GetSpans()
//	testutil.AssertSpanCount(t, spans, 3)
func AssertSpanCount(t *testing.T, spans []sdktrace.ReadOnlySpan, expected int) {
	t.Helper()
	assert.Len(t, spans, expected, "unexpected number of spans")
}

// AssertSpanAttribute verifies that a span has a specific attribute with the expected value.
//
// Example:
//
//	testutil.AssertSpanAttribute(t, span, "messaging.system", "kafka")
//	testutil.AssertSpanAttribute(t, span, "messaging.destination.name", "my-topic")
func AssertSpanAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key, expectedValue string) {
	t.Helper()

	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			assert.Equal(t, expectedValue, attr.Value.AsString(),
				"attribute %s has unexpected value", key)
			return
		}
	}

	t.Errorf("span %q does not have attribute %q", span.Name(), key)
}

// AssertSpanAttributeInt verifies that a span has a specific integer attribute with the expected value.
func AssertSpanAttributeInt(t *testing.T, span sdktrace.ReadOnlySpan, key string, expectedValue int64) {
	t.Helper()

	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			assert.Equal(t, expectedValue, attr.Value.AsInt64(),
				"attribute %s has unexpected value", key)
			return
		}
	}

	t.Errorf("span %q does not have attribute %q", span.Name(), key)
}

// AssertParentChild verifies that a child span is properly linked to its parent span.
// This is critical for verifying distributed tracing context propagation.
//
// Example:
//
//	producerSpan := spans[0]
//	consumerSpan := spans[1]
//	testutil.AssertParentChild(t, producerSpan, consumerSpan)
func AssertParentChild(t *testing.T, parent, child sdktrace.ReadOnlySpan) {
	t.Helper()

	// Same trace ID
	assert.Equal(t, parent.SpanContext().TraceID(), child.SpanContext().TraceID(),
		"child span should have same trace ID as parent")

	// Child's parent is the parent span
	assert.Equal(t, parent.SpanContext().SpanID(), child.Parent().SpanID(),
		"child span should reference parent span ID")
}

// AssertSpanKind verifies that a span has the expected span kind.
//
// Example:
//
//	testutil.AssertSpanKind(t, span, trace.SpanKindProducer)
func AssertSpanKind(t *testing.T, span sdktrace.ReadOnlySpan, expectedKind trace.SpanKind) {
	t.Helper()
	assert.Equal(t, expectedKind, span.SpanKind(), "unexpected span kind")
}

// AssertSpanName verifies that a span has the expected name.
func AssertSpanName(t *testing.T, span sdktrace.ReadOnlySpan, expectedName string) {
	t.Helper()
	assert.Equal(t, expectedName, span.Name(), "unexpected span name")
}

// AssertSpanError verifies that a span has an error recorded.
func AssertSpanError(t *testing.T, span sdktrace.ReadOnlySpan) {
	t.Helper()
	assert.NotEmpty(t, span.Events(), "span should have error events")

	// Check for error event
	hasError := false
	for _, event := range span.Events() {
		if event.Name == "exception" {
			hasError = true
			break
		}
	}

	assert.True(t, hasError, "span should have exception event")
}

// AssertSpanNoError verifies that a span does not have an error recorded.
func AssertSpanNoError(t *testing.T, span sdktrace.ReadOnlySpan) {
	t.Helper()

	for _, event := range span.Events() {
		if event.Name == "exception" {
			t.Errorf("span %q should not have exception event", span.Name())
		}
	}
}

// FindSpanByName finds a span by name in a list of spans.
// Returns nil if not found.
func FindSpanByName(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	return nil
}

// RequireSpanByName finds a span by name and fails the test if not found.
//
// Example:
//
//	kafkaSpan := testutil.RequireSpanByName(t, spans, "my-topic send")
func RequireSpanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	span := FindSpanByName(spans, name)
	require.NotNil(t, span, "span %q not found", name)
	return span
}

// AssertSpanInSameTrace verifies that all spans belong to the same trace.
func AssertSpanInSameTrace(t *testing.T, spans []sdktrace.ReadOnlySpan) {
	t.Helper()

	if len(spans) == 0 {
		return
	}

	expectedTraceID := spans[0].SpanContext().TraceID()
	for i, span := range spans {
		assert.Equal(t, expectedTraceID, span.SpanContext().TraceID(),
			"span %d should have same trace ID", i)
	}
}

// FindSpanByKind finds the first span with the specified kind.
// Returns nil if not found.
func FindSpanByKind(spans []sdktrace.ReadOnlySpan, kind trace.SpanKind) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.SpanKind() == kind {
			return span
		}
	}
	return nil
}

// FindSpansByKind finds all spans with the specified kind.
func FindSpansByKind(spans []sdktrace.ReadOnlySpan, kind trace.SpanKind) []sdktrace.ReadOnlySpan {
	var result []sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.SpanKind() == kind {
			result = append(result, span)
		}
	}
	return result
}

// FindSpanByNameContains finds a span whose name contains the substring.
func FindSpanByNameContains(spans []sdktrace.ReadOnlySpan, substr string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if contains(span.Name(), substr) {
			return span
		}
	}
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(s != "" && substr != "" && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
