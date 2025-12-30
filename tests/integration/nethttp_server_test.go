//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	"github.com/last9/go-agent/instrumentation/nethttp"
	"github.com/last9/go-agent/tests/testutil"
)

// httpGet is a helper that performs GET request with context
func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestNetHTTP_Handler(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create instrumented handler
	handler := nethttp.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello from handler"}`))
	}), "/api/hello")

	server := httptest.NewServer(handler)
	defer server.Close()

	// Make request
	resp := httpGet(t, server.URL+"/api/hello")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "hello from handler")

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	require.GreaterOrEqual(t, len(spans), 1, "should have HTTP server span")

	// Find HTTP server span
	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan, "HTTP server span not found")
	testutil.AssertSpanNoError(t, serverSpan)
}

func TestNetHTTP_HandlerFunc(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Use HandlerFunc convenience wrapper
	handler := nethttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	}, "/ping")

	server := httptest.NewServer(handler)
	defer server.Close()

	resp := httpGet(t, server.URL+"/ping")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan)
}

func TestNetHTTP_ServeMux(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Use instrumented ServeMux
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"Alice"}]`))
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":100}]`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Make requests to both endpoints
	resp := httpGet(t, server.URL+"/users")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp = httpGet(t, server.URL+"/orders")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	// Should have 2 server spans
	serverSpans := testutil.FindSpansByKind(spans, trace.SpanKindServer)
	require.Len(t, serverSpans, 2, "should have 2 HTTP server spans for 2 endpoints")
}

func TestNetHTTP_WrapHandler(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Create standard mux and wrap it
	mux := http.NewServeMux()
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"test"}`))
	})

	wrappedMux := nethttp.WrapHandler(mux)
	server := httptest.NewServer(wrappedMux)
	defer server.Close()

	resp := httpGet(t, server.URL+"/api/data")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan)
}

func TestNetHTTP_Middleware(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Use middleware pattern
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("middleware test"))
	})

	handler := nethttp.Middleware("api")(baseHandler)
	server := httptest.NewServer(handler)
	defer server.Close()

	resp := httpGet(t, server.URL+"/test")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan)
}

func TestNetHTTP_ContextPropagation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	var receivedTraceID trace.TraceID

	handler := nethttp.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace ID from context
		spanCtx := trace.SpanContextFromContext(r.Context())
		receivedTraceID = spanCtx.TraceID()

		w.WriteHeader(http.StatusOK)
	}), "/propagation")

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create request with trace context
	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-propagation-parent")
	parentTraceID := testutil.GetTraceIDFromContext(ctx)
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/propagation", nil)
	require.NoError(t, err)

	// Inject trace context into headers
	nethttp.InjectContext(ctx, req)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	// Verify trace was propagated
	assert.Equal(t, parentTraceID, receivedTraceID, "trace ID should be propagated to handler")

	spans := collector.GetSpans()
	testutil.AssertSpanInSameTrace(t, spans)
}

func TestNetHTTP_ErrorHandling(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	handler := nethttp.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"something went wrong"}`))
	}), "/error")

	server := httptest.NewServer(handler)
	defer server.Close()

	resp := httpGet(t, server.URL+"/error")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	// Server span should exist (status code is recorded as attribute)
	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan)
}

func TestNetHTTP_NotFound(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	mux := nethttp.NewServeMux()
	mux.HandleFunc("/exists", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the whole mux so 404s are also instrumented
	server := httptest.NewServer(nethttp.WrapHandler(mux))
	defer server.Close()

	resp := httpGet(t, server.URL+"/does-not-exist")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1, "should have span even for 404")
}

func TestNetHTTP_MultipleRequests(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	requestCount := 0
	handler := nethttp.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}), "/multi")

	server := httptest.NewServer(handler)
	defer server.Close()

	// Make 5 requests
	for i := 0; i < 5; i++ {
		resp := httpGet(t, server.URL+"/multi")
		resp.Body.Close()
	}

	assert.Equal(t, 5, requestCount)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	// Should have 5 server spans
	serverSpans := testutil.FindSpansByKind(spans, trace.SpanKindServer)
	require.Len(t, serverSpans, 5, "should have 5 HTTP server spans")
}

func TestNetHTTP_DifferentMethods(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	handler := nethttp.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Method))
	}), "/method")

	server := httptest.NewServer(handler)
	defer server.Close()

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, method, server.URL+"/method", nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, method, string(body))
		})
	}

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	serverSpans := testutil.FindSpansByKind(spans, trace.SpanKindServer)
	require.Len(t, serverSpans, 5, "should have spans for all HTTP methods")
}

func TestNetHTTP_ExtractContext(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	var extractedCtx context.Context

	// Handler that manually extracts context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extractedCtx = nethttp.ExtractContext(r.Context(), r)
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create request with trace context
	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "manual-extraction-parent")
	parentTraceID := testutil.GetTraceIDFromContext(ctx)
	defer parentSpan.End()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/extract", nil)
	require.NoError(t, err)
	nethttp.InjectContext(ctx, req)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Verify context was extracted with trace ID
	require.NotNil(t, extractedCtx)
	spanCtx := trace.SpanContextFromContext(extractedCtx)
	assert.Equal(t, parentTraceID, spanCtx.TraceID(), "extracted context should have parent trace ID")
}

func TestNetHTTP_Server(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("server test"))
	})

	// Use nethttp.Server to wrap
	srv := nethttp.Server(&http.Server{
		Handler: mux,
	})

	// Use httptest with the wrapped handler
	server := httptest.NewServer(srv.Handler)
	defer server.Close()

	resp := httpGet(t, server.URL+"/server")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	serverSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, serverSpan)
}
