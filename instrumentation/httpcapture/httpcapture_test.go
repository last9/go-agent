package httpcapture

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/last9/go-agent/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// defaultCfg returns a config with body capture enabled and sensible defaults.
func defaultCfg() *config.Config {
	return &config.Config{
		BodyCaptureEnabled:      true,
		BodyCaptureMaxBytes:     8192,
		BodyCaptureContentTypes: []string{"application/json", "text/plain"},
	}
}

// setupTracer installs a recording tracer and returns the span exporter.
func setupTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider() // capture before replacing
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

// spanAttrs returns attributes from the single finished span, keyed by name.
// Fails the test if not exactly one span was recorded.
func spanAttrs(rec *tracetest.SpanRecorder) map[string]string {
	spans := rec.Ended()
	if len(spans) != 1 {
		return nil
	}
	m := make(map[string]string)
	for _, a := range spans[0].Attributes() {
		m[string(a.Key)] = a.Value.AsString()
	}
	return m
}

// handlerWithSpan wraps h so every request runs inside a named OTel span.
func handlerWithSpan(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := otel.Tracer("test").Start(r.Context(), "test-span")
		defer span.End()
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestIsAllowedContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		allowed     []string
		want        bool
	}{
		{"empty allowed list allows all", "application/json", nil, true},
		{"exact match", "application/json", []string{"application/json"}, true},
		{"prefix match with charset param", "application/json; charset=utf-8", []string{"application/json"}, true},
		{"mismatch", "image/png", []string{"application/json", "text/plain"}, false},
		{"case-insensitive", "Application/JSON", []string{"application/json"}, true},
		{"text/plain match", "text/plain; charset=utf-8", []string{"text/plain"}, true},
		{"empty content-type", "", []string{"application/json"}, false},
		{"whitespace trimmed in allowed list", "application/json", []string{" application/json "}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedContentType(tt.contentType, tt.allowed)
			if got != tt.want {
				t.Errorf("isAllowedContentType(%q, %v) = %v, want %v", tt.contentType, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestLimitedBuffer(t *testing.T) {
	t.Run("captures up to max bytes", func(t *testing.T) {
		buf := newLimitedBuffer(5)
		n, err := buf.Write([]byte("hello world"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 11 {
			t.Errorf("Write returned n=%d, want 11 (original len)", n)
		}
		if buf.String() != "hello" {
			t.Errorf("buf = %q, want %q", buf.String(), "hello")
		}
	})

	t.Run("exact max bytes", func(t *testing.T) {
		buf := newLimitedBuffer(5)
		buf.Write([]byte("hello")) //nolint:errcheck
		if buf.String() != "hello" {
			t.Errorf("buf = %q, want %q", buf.String(), "hello")
		}
	})

	t.Run("discards writes after max reached", func(t *testing.T) {
		buf := newLimitedBuffer(3)
		buf.Write([]byte("abc")) //nolint:errcheck
		n, err := buf.Write([]byte("def"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Errorf("second Write returned n=%d, want 3 (original len)", n)
		}
		if buf.String() != "abc" {
			t.Errorf("buf = %q, want %q", buf.String(), "abc")
		}
	})

	t.Run("zero max discards everything", func(t *testing.T) {
		buf := newLimitedBuffer(0)
		n, err := buf.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 5 {
			t.Errorf("Write returned n=%d, want 5", n)
		}
		if buf.Len() != 0 {
			t.Errorf("buf should be empty, got %q", buf.String())
		}
	})
}

func TestMiddleware_Disabled(t *testing.T) {
	makeHandler := func(called *bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
		}
	}

	t.Run("nil config", func(t *testing.T) {
		called := false
		mw := newMiddleware(makeHandler(&called), nil)
		req := httptest.NewRequest("GET", "/", nil)
		rw := httptest.NewRecorder()
		mw.ServeHTTP(rw, req)
		if !called {
			t.Error("handler not called")
		}
		if rw.Code != http.StatusTeapot {
			t.Errorf("status = %d, want 418", rw.Code)
		}
	})

	t.Run("enabled=false", func(t *testing.T) {
		called := false
		cfg := defaultCfg()
		cfg.BodyCaptureEnabled = false
		mw := newMiddleware(makeHandler(&called), cfg)
		req := httptest.NewRequest("GET", "/", nil)
		rw := httptest.NewRecorder()
		mw.ServeHTTP(rw, req)
		if !called {
			t.Error("handler not called")
		}
		if rw.Code != http.StatusTeapot {
			t.Errorf("status = %d, want 418", rw.Code)
		}
	})
}

func TestMiddleware_CapturesRequestBody(t *testing.T) {
	rec := setupTracer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck — consume so TeeReader copies to buf
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})

	mw := handlerWithSpan(newMiddleware(inner, defaultCfg()))
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	attrs := spanAttrs(rec)
	if got := attrs["http.request.body"]; got != `{"name":"test"}` {
		t.Errorf("http.request.body = %q, want %q", got, `{"name":"test"}`)
	}
}

func TestMiddleware_CapturesResponseBody(t *testing.T) {
	rec := setupTracer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	mw := handlerWithSpan(newMiddleware(inner, defaultCfg()))
	req := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	attrs := spanAttrs(rec)
	if got := attrs["http.response.body"]; got != `{"status":"ok"}` {
		t.Errorf("http.response.body = %q, want %q", got, `{"status":"ok"}`)
	}
}

func TestMiddleware_ContentTypeFilter(t *testing.T) {
	rec := setupTracer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("binary-data")) //nolint:errcheck
	})

	mw := handlerWithSpan(newMiddleware(inner, defaultCfg()))
	req := httptest.NewRequest("POST", "/", strings.NewReader("binary-data"))
	req.Header.Set("Content-Type", "image/png")
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	attrs := spanAttrs(rec)
	if _, ok := attrs["http.request.body"]; ok {
		t.Error("http.request.body should not be set for image/png request")
	}
	if _, ok := attrs["http.response.body"]; ok {
		t.Error("http.response.body should not be set for image/png response")
	}
}

func TestMiddleware_MaxBytes(t *testing.T) {
	rec := setupTracer(t)

	cfg := defaultCfg()
	cfg.BodyCaptureMaxBytes = 5

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"longresponse":true}`)) //nolint:errcheck
	})

	mw := handlerWithSpan(newMiddleware(inner, cfg))
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"longrequest":true}`))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	attrs := spanAttrs(rec)
	if got := attrs["http.request.body"]; got != `{"lon` {
		t.Errorf("http.request.body = %q, want truncated to 5 bytes", got)
	}
	if got := attrs["http.response.body"]; got != `{"lon` {
		t.Errorf("http.response.body = %q, want truncated to 5 bytes", got)
	}
}

func TestMiddleware_OnErrorOnly(t *testing.T) {
	t.Run("skips 200", func(t *testing.T) {
		rec := setupTracer(t)
		cfg := defaultCfg()
		cfg.BodyCaptureOnErrorOnly = true

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.ReadAll(r.Body) //nolint:errcheck
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
		})

		mw := handlerWithSpan(newMiddleware(inner, cfg))
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"x":1}`))
		req.Header.Set("Content-Type", "application/json")
		rw := httptest.NewRecorder()
		mw.ServeHTTP(rw, req)

		attrs := spanAttrs(rec)
		if _, ok := attrs["http.request.body"]; ok {
			t.Error("http.request.body should not be set on 200 when OnErrorOnly=true")
		}
	})

	t.Run("captures 500", func(t *testing.T) {
		rec := setupTracer(t)
		cfg := defaultCfg()
		cfg.BodyCaptureOnErrorOnly = true

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.ReadAll(r.Body) //nolint:errcheck
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"boom"}`)) //nolint:errcheck
		})

		mw := handlerWithSpan(newMiddleware(inner, cfg))
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"x":1}`))
		req.Header.Set("Content-Type", "application/json")
		rw := httptest.NewRecorder()
		mw.ServeHTTP(rw, req)

		attrs := spanAttrs(rec)
		if got := attrs["http.response.body"]; got != `{"error":"boom"}` {
			t.Errorf("http.response.body = %q, want %q", got, `{"error":"boom"}`)
		}
	})
}

func TestMiddleware_OnErrorOnly_LazyAlloc(t *testing.T) {
	cfg := defaultCfg()
	cfg.BodyCaptureOnErrorOnly = true

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})

	// Reach into the captureResponseWriter to verify buf was never allocated.
	var capturedRW *captureResponseWriter
	spy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r)
		capturedRW, _ = w.(*captureResponseWriter)
	})

	mw := newMiddleware(spy, cfg)
	req := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	if capturedRW != nil && capturedRW.buf != nil {
		t.Error("buf should not be allocated for 200 response when OnErrorOnly=true")
	}
}

func TestMiddleware_NoSpan(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})

	// No span in context — should not panic, response should still be proxied
	mw := newMiddleware(inner, defaultCfg())
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"x":1}`))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rw.Code)
	}
	if rw.Body.String() != `{"ok":true}` {
		t.Errorf("body = %q, want %q", rw.Body.String(), `{"ok":true}`)
	}
}

func TestMiddleware_ResponsePassthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created")) //nolint:errcheck
	})

	mw := newMiddleware(inner, defaultCfg())
	req := httptest.NewRequest("POST", "/", nil)
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	if rw.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rw.Code)
	}
	if rw.Header().Get("X-Custom") != "value" {
		t.Errorf("X-Custom header not passed through")
	}
	if rw.Body.String() != "created" {
		t.Errorf("body = %q, want %q", rw.Body.String(), "created")
	}
}

func TestMiddleware_NilBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) //nolint:errcheck
	})

	// GET with no body — should not panic
	mw := newMiddleware(inner, defaultCfg())
	req := httptest.NewRequest("GET", "/", nil)
	req.Body = nil
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rw.Code)
	}
}

// Compile-time checks that captureResponseWriter satisfies the optional
// ResponseWriter interfaces it must forward. http.Hijacker is what live tail's
// WebSocket upgrade depends on (ENG-1278); embedding the http.ResponseWriter
// interface alone does NOT promote these.
var (
	_ http.ResponseWriter = (*captureResponseWriter)(nil)
	_ http.Hijacker       = (*captureResponseWriter)(nil)
	_ http.Flusher        = (*captureResponseWriter)(nil)
	_ http.Pusher         = (*captureResponseWriter)(nil)
)

// rwSpy is a ResponseWriter that also implements Hijacker, Flusher, and Pusher,
// recording whether each was forwarded. Used to verify the wrapper promotes
// these to itself — not merely to its embedded writer.
type rwSpy struct {
	http.ResponseWriter
	pushed       string
	hijacked     bool
	flushed      bool
	readDeadline bool
}

func (s *rwSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	s.hijacked = true
	return nil, nil, nil
}

func (s *rwSpy) Flush() { s.flushed = true }

func (s *rwSpy) Push(target string, _ *http.PushOptions) error {
	s.pushed = target
	return nil
}

// SetReadDeadline is reachable only via http.ResponseController, which walks the
// wrapper chain through Unwrap(). It is deliberately NOT one of the methods the
// wrapper forwards explicitly — that is what makes it a test of Unwrap().
func (s *rwSpy) SetReadDeadline(time.Time) error {
	s.readDeadline = true
	return nil
}

// bareRW implements only http.ResponseWriter — none of the optional interfaces.
// Used to exercise the unsupported/no-op branches of the forwarders.
type bareRW struct{ header http.Header }

func (b *bareRW) Header() http.Header         { return b.header }
func (b *bareRW) Write(p []byte) (int, error) { return len(p), nil }
func (b *bareRW) WriteHeader(int)             {}

// TestCaptureResponseWriter_InterfacePromotion asserts on the wrapper itself
// (interface{}(rw).(...)), not on rw.ResponseWriter. This is the assertion the
// old Flusher test missed, which is why the missing Hijack() slipped through
// and broke live tail through the body-capture middleware.
func TestCaptureResponseWriter_InterfacePromotion(t *testing.T) {
	t.Run("forwards Hijack to embedded writer", func(t *testing.T) {
		spy := &rwSpy{ResponseWriter: httptest.NewRecorder()}
		rw := &captureResponseWriter{ResponseWriter: spy}

		hj, ok := interface{}(rw).(http.Hijacker)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Hijacker so WebSocket/SSE upgrades work")
		}
		if _, _, err := hj.Hijack(); err != nil {
			t.Fatalf("Hijack() returned error: %v", err)
		}
		if !spy.hijacked {
			t.Error("Hijack() did not forward to the embedded writer")
		}
	})

	t.Run("forwards Flush to embedded writer", func(t *testing.T) {
		spy := &rwSpy{ResponseWriter: httptest.NewRecorder()}
		rw := &captureResponseWriter{ResponseWriter: spy}

		f, ok := interface{}(rw).(http.Flusher)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Flusher")
		}
		f.Flush()
		if !spy.flushed {
			t.Error("Flush() did not forward to the embedded writer")
		}
	})

	t.Run("forwards Push to embedded writer", func(t *testing.T) {
		spy := &rwSpy{ResponseWriter: httptest.NewRecorder()}
		rw := &captureResponseWriter{ResponseWriter: spy}

		p, ok := interface{}(rw).(http.Pusher)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Pusher")
		}
		if err := p.Push("/style.css", nil); err != nil {
			t.Fatalf("Push() returned error: %v", err)
		}
		if spy.pushed != "/style.css" {
			t.Errorf("Push() forwarded target = %q, want %q", spy.pushed, "/style.css")
		}
	})

	t.Run("degrades gracefully when embedded writer supports nothing", func(t *testing.T) {
		// bareRW implements only http.ResponseWriter.
		rw := &captureResponseWriter{ResponseWriter: &bareRW{header: http.Header{}}}

		hj, ok := interface{}(rw).(http.Hijacker)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Hijacker")
		}
		if _, _, err := hj.Hijack(); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Hijack() err = %v, want http.ErrNotSupported", err)
		}

		p, ok := interface{}(rw).(http.Pusher)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Pusher")
		}
		if err := p.Push("/x", nil); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Push() err = %v, want http.ErrNotSupported", err)
		}

		f, ok := interface{}(rw).(http.Flusher)
		if !ok {
			t.Fatal("captureResponseWriter must implement http.Flusher")
		}
		f.Flush() // must be a no-op, not a panic, when the writer cannot flush
	})

	t.Run("Unwrap lets http.ResponseController reach the underlying writer", func(t *testing.T) {
		// SetReadDeadline is not forwarded explicitly; ResponseController can only
		// reach it by walking the chain through Unwrap().
		spy := &rwSpy{ResponseWriter: httptest.NewRecorder()}
		rw := &captureResponseWriter{ResponseWriter: spy}

		rc := http.NewResponseController(rw)
		if err := rc.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("SetReadDeadline via ResponseController returned error: %v", err)
		}
		if !spy.readDeadline {
			t.Error("SetReadDeadline did not reach the underlying writer through Unwrap()")
		}
	})
}

// TestMiddleware_PreservesHijackerThroughChain verifies the writer the handler
// actually receives from newMiddleware still satisfies http.Hijacker and that
// Hijack() reaches the real underlying writer. This guards the construction site
// itself — a refactor that dropped the wrapping (or wrapped in a type without
// the forwarders) would regress ENG-1278 even if the captureResponseWriter unit
// tests still passed.
func TestMiddleware_PreservesHijackerThroughChain(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("handler's ResponseWriter is not an http.Hijacker")
			return
		}
		if _, _, err := hj.Hijack(); err != nil {
			t.Errorf("Hijack() returned error: %v", err)
		}
	})

	spy := &rwSpy{ResponseWriter: httptest.NewRecorder()}
	mw := newMiddleware(handler, defaultCfg())
	mw.ServeHTTP(spy, httptest.NewRequest("GET", "/", nil))

	if !spy.hijacked {
		t.Error("Hijack() did not reach the underlying writer through the middleware chain")
	}
}

func TestMiddleware_SpanAttributes(t *testing.T) {
	rec := setupTracer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":42}`)) //nolint:errcheck
	})

	mw := handlerWithSpan(newMiddleware(inner, defaultCfg()))
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"input":1}`))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, req)

	spans := rec.Ended()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}

	attrMap := make(map[attribute.Key]attribute.Value)
	for _, a := range spans[0].Attributes() {
		attrMap[a.Key] = a.Value
	}

	if v, ok := attrMap["http.request.body"]; !ok || v.AsString() != `{"input":1}` {
		t.Errorf("http.request.body = %q, want %q", v.AsString(), `{"input":1}`)
	}
	if v, ok := attrMap["http.response.body"]; !ok || v.AsString() != `{"result":42}` {
		t.Errorf("http.response.body = %q, want %q", v.AsString(), `{"result":42}`)
	}
}
