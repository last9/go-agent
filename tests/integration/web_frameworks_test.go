//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/mux"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	chiagent "github.com/last9/go-agent/instrumentation/chi"
	echoagent "github.com/last9/go-agent/instrumentation/echo"
	ginagent "github.com/last9/go-agent/instrumentation/gin"
	gorillaagent "github.com/last9/go-agent/instrumentation/gorilla"
	"github.com/last9/go-agent/tests/testutil"
)

func init() {
	// Set Gin to test mode to reduce noise
	gin.SetMode(gin.TestMode)
}

// TestGin_Instrumentation tests Gin framework instrumentation
func TestGin_Instrumentation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	// Create instrumented Gin router
	r := ginagent.New()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})
	r.GET("/users/:id", func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(200, gin.H{"id": id})
	})

	// Test simple endpoint
	t.Run("simple endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/ping", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "pong")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)

		// Find HTTP span
		var httpSpan = testutil.FindSpanByKind(spans, trace.SpanKindServer)
		require.NotNil(t, httpSpan, "HTTP server span not found")
		testutil.AssertSpanNoError(t, httpSpan)
	})

	// Test parameterized endpoint
	t.Run("parameterized endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/users/123", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "123")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})
}

// TestGin_Default tests Gin Default() with logging and recovery
func TestGin_Default(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	r := ginagent.Default()
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "OK")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	time.Sleep(100 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)
}

// TestGin_Middleware tests adding middleware to existing router
func TestGin_Middleware(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	r := gin.New()
	r.Use(ginagent.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "OK")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	time.Sleep(100 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)
}

// TestChi_Instrumentation tests Chi router instrumentation
func TestChi_Instrumentation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	// Create standard Chi router and define routes
	r := chi.NewRouter()
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	})
	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
	})

	// Apply instrumentation AFTER routes are defined (Chi requirement)
	handler := chiagent.Use(r)

	t.Run("simple endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/ping", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "pong")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})

	t.Run("parameterized endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/users/456", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "456")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})
}

// TestChi_Use tests adding instrumentation to existing router
func TestChi_Use(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	r := chi.NewRouter()
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK"))
	})
	// Use returns an http.Handler wrapper since chi.Router.Use() panics
	// if routes are already defined
	handler := chiagent.Use(r)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	time.Sleep(100 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)
}

// TestEcho_Instrumentation tests Echo framework instrumentation
func TestEcho_Instrumentation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	// Create instrumented Echo instance
	e := echoagent.New()
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"message": "pong"})
	})
	e.GET("/users/:id", func(c echo.Context) error {
		id := c.Param("id")
		return c.JSON(200, map[string]string{"id": id})
	})

	t.Run("simple endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/ping", nil)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "pong")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})

	t.Run("parameterized endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/users/789", nil)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "789")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})
}

// TestGorilla_Instrumentation tests Gorilla Mux instrumentation
func TestGorilla_Instrumentation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	// Create instrumented Gorilla router
	r := gorillaagent.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	}).Methods("GET")
	r.HandleFunc("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
	}).Methods("GET")

	t.Run("simple endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/ping", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "pong")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})

	t.Run("parameterized endpoint", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/users/101", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "101")

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})
}

// TestGorilla_Middleware tests adding middleware to existing router
func TestGorilla_Middleware(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	r := mux.NewRouter()
	r.Use(gorillaagent.Middleware())
	r.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK"))
	}).Methods("GET")

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	time.Sleep(100 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)
}

// TestWebFrameworks_ErrorHandling tests error span recording
func TestWebFrameworks_ErrorHandling(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	r := ginagent.New()
	r.GET("/error", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "internal error"})
	})
	r.GET("/not-found", func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})

	t.Run("500 error", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/error", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 500, w.Code)

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})

	t.Run("404 error", func(t *testing.T) {
		collector.Reset()

		req := httptest.NewRequest("GET", "/not-found", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 404, w.Code)

		time.Sleep(100 * time.Millisecond)

		spans := collector.GetSpans()
		require.GreaterOrEqual(t, len(spans), 1)
	})
}

// TestWebFrameworks_ContextPropagation tests trace context is available in handlers
func TestWebFrameworks_ContextPropagation(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(t.Context())

	var capturedTraceID string

	r := ginagent.New()
	r.GET("/trace", func(c *gin.Context) {
		// Get trace ID from context
		span := trace.SpanFromContext(c.Request.Context())
		if span.SpanContext().IsValid() {
			capturedTraceID = span.SpanContext().TraceID().String()
		}
		c.JSON(200, gin.H{"traced": true})
	})

	req := httptest.NewRequest("GET", "/trace", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.NotEmpty(t, capturedTraceID, "trace ID should be captured in handler")

	time.Sleep(100 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	// Verify the captured trace ID matches the span
	httpSpan := testutil.FindSpanByKind(spans, trace.SpanKindServer)
	require.NotNil(t, httpSpan)
	assert.Equal(t, capturedTraceID, httpSpan.SpanContext().TraceID().String())
}

// Helper to read response body
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
