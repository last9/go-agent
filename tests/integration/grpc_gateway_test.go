//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/last9/go-agent"
	"github.com/last9/go-agent/instrumentation/grpcgateway"
	testproto "github.com/last9/go-agent/tests/integration/testdata"
	"github.com/last9/go-agent/tests/testutil"
)

// testServer implements the TestService gRPC service
type testServer struct {
	testproto.UnimplementedTestServiceServer
}

func (s *testServer) SayHello(ctx context.Context, req *testproto.HelloRequest) (*testproto.HelloResponse, error) {
	return &testproto.HelloResponse{
		Message: fmt.Sprintf("Hello, %s!", req.Name),
	}, nil
}

func TestGrpcGateway_NewGrpcServer(t *testing.T) {
	// Initialize agent
	agent.Start()
	defer agent.Shutdown()

	// Create instrumented gRPC server
	server := grpcgateway.NewGrpcServer()
	require.NotNil(t, server)

	// Verify it's a *grpc.Server
	var _ *grpc.Server = server
}

func TestGrpcGateway_WrapHTTPMux(t *testing.T) {
	// Initialize agent
	agent.Start()
	defer agent.Shutdown()

	// Create HTTP mux
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with instrumentation
	handler := grpcgateway.WrapHTTPMux(mux, "test-service")
	require.NotNil(t, handler)

	// Verify it's an http.Handler
	var _ http.Handler = handler
}

func TestGrpcGateway_NewDialOption(t *testing.T) {
	// Initialize agent
	agent.Start()
	defer agent.Shutdown()

	// Create dial option
	dialOption := grpcgateway.NewDialOption()
	require.NotNil(t, dialOption)

	// Verify it's a grpc.DialOption
	var _ grpc.DialOption = dialOption
}

func TestGrpcGateway_FullStack_Tracing(t *testing.T) {
	// Setup mock collector
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Initialize agent
	agent.Start()
	defer agent.Shutdown()

	ctx := context.Background()

	// 1. Start gRPC server
	grpcServer := grpcgateway.NewGrpcServer()
	testproto.RegisterTestServiceServer(grpcServer, &testServer{})

	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcAddr := grpcListener.Addr().String()
	t.Logf("gRPC server listening on: %s", grpcAddr)

	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Wait for gRPC server to be ready
	time.Sleep(500 * time.Millisecond)

	// 2. Create gRPC-Gateway mux
	gwMux := grpcgateway.NewGatewayMux()

	// 3. Register gateway handler with instrumented client
	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcgateway.NewDialOption(),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = testproto.RegisterTestServiceHandler(ctx, gwMux, conn)
	require.NoError(t, err)

	// 4. Start HTTP server with instrumented mux
	httpMux := http.NewServeMux()
	httpMux.Handle("/", gwMux)
	handler := grpcgateway.WrapHTTPMux(httpMux, "test-gateway")

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpAddr := httpListener.Addr().String()
	t.Logf("HTTP gateway listening on: %s", httpAddr)

	httpServer := &http.Server{Handler: handler}
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			t.Logf("HTTP server error: %v", err)
		}
	}()
	defer httpServer.Shutdown(ctx)

	// Wait for HTTP server to be ready
	time.Sleep(500 * time.Millisecond)

	// 5. Make HTTP request
	requestBody := map[string]string{"name": "World"}
	jsonBody, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/v1/test/hello", httpAddr), bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var responseData map[string]string
	err = json.Unmarshal(responseBody, &responseData)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", responseData["message"])

	// Wait for spans to be recorded
	time.Sleep(1 * time.Second)

	// 6. Verify trace spans
	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v, trace: %s)", i, span.Name(), span.SpanKind(), span.SpanContext().TraceID())
	}

	// We should have multiple spans:
	// - HTTP server span (from WrapHTTPMux)
	// - gRPC client span (from NewDialOption)
	// - gRPC server span (from NewGrpcServer)
	require.GreaterOrEqual(t, len(spans), 3, "should have at least 3 spans (HTTP + gRPC client + gRPC server)")

	// Find HTTP span (WrapHTTPMux creates a span with the service name)
	var httpSpan sdktrace.ReadOnlySpan
	httpSpanFound := false
	for _, span := range spans {
		// HTTP span is created by WrapHTTPMux with service name
		if span.SpanKind() == trace.SpanKindServer && span.Name() == "test-gateway" {
			httpSpan = span
			httpSpanFound = true
			break
		}
	}
	require.True(t, httpSpanFound, "HTTP server span not found (looking for 'test-gateway')")

	// Find gRPC server span
	var grpcServerSpan sdktrace.ReadOnlySpan
	grpcSpanFound := false
	for _, span := range spans {
		if span.Name() == "testproto.TestService/SayHello" && span.SpanKind() == trace.SpanKindServer {
			grpcServerSpan = span
			grpcSpanFound = true
			break
		}
	}
	require.True(t, grpcSpanFound, "gRPC server span not found (looking for 'testproto.TestService/SayHello')")

	// Verify all spans are in the same trace
	testutil.AssertSpanInSameTrace(t, spans)

	// Verify span kinds
	testutil.AssertSpanKind(t, httpSpan, trace.SpanKindServer)
	testutil.AssertSpanKind(t, grpcServerSpan, trace.SpanKindServer)

	// Verify no errors in spans
	testutil.AssertSpanNoError(t, httpSpan)
	testutil.AssertSpanNoError(t, grpcServerSpan)
}

func TestGrpcGateway_FullStack_ErrorHandling(t *testing.T) {
	// Setup mock collector
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// Initialize agent
	agent.Start()
	defer agent.Shutdown()

	ctx := context.Background()

	// Start gRPC server
	grpcServer := grpcgateway.NewGrpcServer()
	testproto.RegisterTestServiceServer(grpcServer, &testServer{})

	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcAddr := grpcListener.Addr().String()

	go func() {
		grpcServer.Serve(grpcListener)
	}()
	defer grpcServer.Stop()

	time.Sleep(500 * time.Millisecond)

	// Create gateway with instrumented client
	gwMux := grpcgateway.NewGatewayMux()

	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcgateway.NewDialOption(),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = testproto.RegisterTestServiceHandler(ctx, gwMux, conn)
	require.NoError(t, err)

	// Start HTTP server
	httpMux := http.NewServeMux()
	httpMux.Handle("/", gwMux)
	handler := grpcgateway.WrapHTTPMux(httpMux, "test-gateway-error")

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpAddr := httpListener.Addr().String()

	httpServer := &http.Server{Handler: handler}
	go func() {
		httpServer.Serve(httpListener)
	}()
	defer httpServer.Shutdown(ctx)

	time.Sleep(500 * time.Millisecond)

	// Make HTTP request with invalid JSON (should cause error)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/v1/test/hello", httpAddr), bytes.NewBufferString("invalid json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should get error response
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Wait for spans
	time.Sleep(1 * time.Second)

	// Verify spans were captured (even with error)
	spans := collector.GetSpans()
	assert.GreaterOrEqual(t, len(spans), 1, "should have at least 1 span for error case")
}
