// Package httpcapture provides HTTP request/response body capture as OpenTelemetry
// span attributes.
//
// It is a framework-agnostic net/http middleware. Wrap your handler or mux with it,
// placing it *inside* the OTel tracing middleware so the span exists in context:
//
//	// net/http — httpcapture must be inside otelhttp so trace.SpanFromContext finds a recording span
//	mux := http.NewServeMux()
//	mux.HandleFunc("/api", myHandler)
//	http.ListenAndServe(":8080", nethttp.WrapHandler(httpcapture.Middleware(mux)))
//
//	// Gin — wrap the entire engine before Gin touches the ResponseWriter
//	r := ginagent.New()
//	http.ListenAndServe(":8080", httpcapture.Middleware(r))
//
//	// Echo
//	e := echoagent.New()
//	http.ListenAndServe(":8080", httpcapture.Middleware(e))
//
// Configuration via environment variables (same pattern as other Last9 config):
//
//	LAST9_BODY_CAPTURE_ENABLED         true/false          (default: false)
//	LAST9_BODY_CAPTURE_MAX_BYTES       integer > 0         (default: 8192)
//	LAST9_BODY_CAPTURE_ON_ERROR_ONLY   true/false          (default: false)
//	LAST9_BODY_CAPTURE_CONTENT_TYPES   comma-separated     (default: application/json,application/xml,text/plain)
//
// Span attributes set:
//
//	http.request.body   — captured request body (truncated to LAST9_BODY_CAPTURE_MAX_BYTES)
//	http.response.body  — captured response body (truncated to LAST9_BODY_CAPTURE_MAX_BYTES)
//
// Note: http.request.body and http.response.body are not in OTel semconv; they follow
// the convention established by last9/dotnet-otel-body-capture.
//
// PII/PHI: body capture is opt-in (disabled by default). For production, prefer
// handling sensitive data redaction at the collector layer using transform processors.
package httpcapture

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/last9/go-agent"
	"github.com/last9/go-agent/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Middleware returns an http.Handler middleware that captures request/response
// bodies onto the active OTel span as http.request.body and http.response.body.
//
// Config is read once at construction time from agent.GetConfig().
// No-ops when LAST9_BODY_CAPTURE_ENABLED is false (default) or no span is recording.
func Middleware(next http.Handler) http.Handler {
	return newMiddleware(next, agent.GetConfig())
}

// newMiddleware is the testable core; Middleware delegates here.
func newMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	if cfg == nil || !cfg.BodyCaptureEnabled {
		return next
	}

	maxBytes := cfg.BodyCaptureMaxBytes
	onErrorOnly := cfg.BodyCaptureOnErrorOnly
	contentTypes := cfg.BodyCaptureContentTypes

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture request body via TeeReader — handler still reads the original stream.
		var reqBodyBuf *limitedBuffer
		if r.Body != nil && isAllowedContentType(r.Header.Get("Content-Type"), contentTypes) {
			reqBodyBuf = newLimitedBuffer(maxBytes)
			r.Body = io.NopCloser(io.TeeReader(r.Body, reqBodyBuf))
		}

		// Wrap response writer. When onErrorOnly=true, buf is nil until WriteHeader
		// receives a status >= 400 — no allocation on the happy path.
		rw := &captureResponseWriter{
			ResponseWriter: w,
			maxBytes:       maxBytes,
			contentTypes:   contentTypes,
			onErrorOnly:    onErrorOnly,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rw, r)

		if onErrorOnly && rw.status < 400 {
			return
		}

		span := trace.SpanFromContext(r.Context())
		if !span.IsRecording() {
			return
		}

		if reqBodyBuf != nil && reqBodyBuf.Len() > 0 {
			span.SetAttributes(attribute.String("http.request.body", reqBodyBuf.String()))
		}

		// Use Content-Type snapshotted at WriteHeader time, not post-ServeHTTP header map.
		if isAllowedContentType(rw.respContentType, contentTypes) && rw.buf != nil && rw.buf.Len() > 0 {
			span.SetAttributes(attribute.String("http.response.body", rw.buf.String()))
		}
	})
}

// captureResponseWriter wraps http.ResponseWriter to record status code and body.
// Embedding preserves interface promotions (http.Flusher, http.Hijacker, etc.).
//
// When onErrorOnly=true, buf is allocated lazily in WriteHeader only for error responses,
// keeping the successful-request path allocation-free.
//
// Field ordering is optimized for GC pointer scan bytes (pointer fields precede scalars).
type captureResponseWriter struct {
	http.ResponseWriter
	buf             *limitedBuffer // nil until WriteHeader when onErrorOnly=true
	respContentType string         // Content-Type snapshotted at WriteHeader time
	contentTypes    []string
	maxBytes        int64
	status          int
	onErrorOnly     bool
	wroteHeader     bool
}

func (rw *captureResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.respContentType = rw.Header().Get("Content-Type")
		if !rw.onErrorOnly || code >= 400 {
			rw.buf = newLimitedBuffer(rw.maxBytes)
		}
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *captureResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	if rw.buf != nil {
		_, _ = rw.buf.Write(b)
	}
	return rw.ResponseWriter.Write(b)
}

// limitedBuffer is a bytes.Buffer that stops accepting writes after max bytes.
type limitedBuffer struct {
	bytes.Buffer
	max     int64
	written int64
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{max: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	remaining := b.max - b.written
	if remaining <= 0 {
		return originalLen, nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := b.Buffer.Write(p)
	b.written += int64(n)
	// Return original len so callers (TeeReader, ResponseWriter chain) aren't confused by a short write.
	return originalLen, err
}

// isAllowedContentType reports whether contentType starts with any prefix in allowed.
// Empty allowed list means all types are allowed.
func isAllowedContentType(contentType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	for _, a := range allowed {
		if strings.HasPrefix(ct, strings.ToLower(strings.TrimSpace(a))) {
			return true
		}
	}
	return false
}
