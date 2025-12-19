//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	httpagent "github.com/last9/go-agent/integrations/http"
	"github.com/last9/go-agent/tests/testutil"
)

func TestHTTPClient_Get(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	// Create instrumented client
	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-http-get-parent")
	defer parentSpan.End()

	// Make request
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "hello")

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	require.GreaterOrEqual(t, len(spans), 2, "should have parent + HTTP client spans")

	// Find HTTP client span
	clientSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, clientSpan, "HTTP client span not found")
	testutil.AssertSpanNoError(t, clientSpan)
}

func TestHTTPClient_Post(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create test server that echoes back the body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-http-post-parent")
	defer parentSpan.End()

	// Make POST request
	reqBody := strings.NewReader(`{"name":"test"}`)
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/data", reqBody)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)

	clientSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, clientSpan)
}

func TestHTTPClient_Error(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-http-error-parent")
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/error", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err) // HTTP errors are not Go errors
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)

	// HTTP client span should exist (error status is recorded in attributes, not as span error)
	clientSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, clientSpan)
}

func TestHTTPClient_Timeout(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Client with short timeout
	client := httpagent.NewClient(&http.Client{
		Timeout: 100 * time.Millisecond,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-http-timeout-parent")
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/slow", nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err) // Should timeout

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1, "should have at least parent span")
}

func TestHTTPClient_ContextPropagation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Track if trace headers were received
	var receivedTraceParent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceParent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-propagation-parent")
	parentTraceID := testutil.GetTraceIDFromContext(ctx)
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/propagation", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	// Verify trace header was propagated
	assert.NotEmpty(t, receivedTraceParent, "traceparent header should be propagated")
	assert.Contains(t, receivedTraceParent, parentTraceID.String(), "trace ID should be in traceparent header")

	spans := collector.GetSpans()
	testutil.AssertSpanInSameTrace(t, spans)
}

func TestHTTPClient_MultipleRequests(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-multi-request-parent")
	defer parentSpan.End()

	// Make multiple requests
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/multi", nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	assert.Equal(t, 3, requestCount)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	// Should have parent + 3 HTTP client spans
	require.GreaterOrEqual(t, len(spans), 4, "should have parent + 3 HTTP spans")

	// All spans should be in the same trace
	testutil.AssertSpanInSameTrace(t, spans)
}

func TestHTTPClient_DifferentMethods(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Method))
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-methods-parent")
	defer parentSpan.End()

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var body io.Reader
			if method == "POST" || method == "PUT" || method == "PATCH" {
				body = strings.NewReader(`{"test":true}`)
			}

			req, err := http.NewRequestWithContext(ctx, method, server.URL+"/method", body)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	// Parent + 5 HTTP method spans
	require.GreaterOrEqual(t, len(spans), 6, "should have parent + 5 HTTP spans")
}

func TestHTTPClient_CustomHeaders(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-headers-parent")
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/headers", nil)
	require.NoError(t, err)
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify custom headers were sent
	assert.Equal(t, "custom-value", receivedHeaders.Get("X-Custom-Header"))
	assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"))

	// Verify trace headers were also added
	assert.NotEmpty(t, receivedHeaders.Get("traceparent"))

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)
}
