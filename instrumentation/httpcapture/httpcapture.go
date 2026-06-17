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
//	LAST9_HEADER_CAPTURE_REQUEST      comma-separated     (default: empty/disabled)
//	LAST9_HEADER_CAPTURE_RESPONSE     comma-separated     (default: empty/disabled)
//
// Span attributes set:
//
//	http.request.body            — captured request body (truncated to LAST9_BODY_CAPTURE_MAX_BYTES)
//	http.response.body           — captured response body (truncated to LAST9_BODY_CAPTURE_MAX_BYTES)
//	http.request.header.<key>    — allowlisted request header values (StringSlice)
//	http.response.header.<key>   — allowlisted response header values (StringSlice)
//
// Header capture is independent of body capture: set either allowlist and headers
// are captured even when LAST9_BODY_CAPTURE_ENABLED is false, and are not subject
// to LAST9_BODY_CAPTURE_ON_ERROR_ONLY. <key> is the header name normalized exactly
// as the OpenTelemetry Go SDK does: lowercased with '-' replaced by '_'
// (X-Last9-Client -> http.request.header.x_last9_client).
//
// WARNING: header values are captured verbatim onto spans. Do NOT allowlist
// credential-bearing headers (Authorization, Proxy-Authorization, Cookie,
// Set-Cookie, X-Api-Key, etc.) — traces often flow to lower-trust backends.
// Prefer redacting at the collector if such capture is unavoidable.
//
// Note: http.request.body and http.response.body are not in OTel semconv; they follow
// the convention established by last9/dotnet-otel-body-capture.
//
// PII/PHI: body capture is opt-in (disabled by default). For production, prefer
// handling sensitive data redaction at the collector layer using transform processors.
package httpcapture

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/last9/go-agent"
	"github.com/last9/go-agent/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Middleware returns an http.Handler middleware that captures request/response
// bodies (and allowlisted headers) onto the active OTel span.
//
// Config is read once at construction time from agent.GetConfig().
// No-ops when body capture is disabled AND no header allowlist is configured
// (the default), or when no span is recording.
func Middleware(next http.Handler) http.Handler {
	return newMiddleware(next, agent.GetConfig())
}

// newMiddleware is the testable core; Middleware delegates here.
func newMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	if cfg == nil {
		return next
	}

	// Header capture is independent of body capture: the middleware activates if
	// EITHER is configured. Allowlists are normalized to full OTel attribute keys
	// once at construction (no per-request string work).
	reqHeaderKeys := normalizeHeaderKeys("http.request.header.", cfg.CaptureRequestHeaders)
	respHeaderKeys := normalizeHeaderKeys("http.response.header.", cfg.CaptureResponseHeaders)
	headerCaptureOn := len(reqHeaderKeys) > 0 || len(respHeaderKeys) > 0

	if !cfg.BodyCaptureEnabled && !headerCaptureOn {
		return next
	}

	bodyCapture := cfg.BodyCaptureEnabled
	maxBytes := cfg.BodyCaptureMaxBytes
	onErrorOnly := cfg.BodyCaptureOnErrorOnly
	contentTypes := cfg.BodyCaptureContentTypes

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture request body via TeeReader — handler still reads the original stream.
		var reqBodyBuf *limitedBuffer
		if bodyCapture && r.Body != nil && isAllowedContentType(r.Header.Get("Content-Type"), contentTypes) {
			reqBodyBuf = newLimitedBuffer(maxBytes)
			r.Body = io.NopCloser(io.TeeReader(r.Body, reqBodyBuf))
		}

		// Wrap response writer. When onErrorOnly=true, buf is nil until WriteHeader
		// receives a status >= 400 — no allocation on the happy path. Response
		// headers (if allowlisted) are snapshotted at WriteHeader time.
		rw := &captureResponseWriter{
			ResponseWriter: w,
			respHeaderKeys: respHeaderKeys,
			maxBytes:       maxBytes,
			contentTypes:   contentTypes,
			onErrorOnly:    onErrorOnly,
			bodyCapture:    bodyCapture,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rw, r)

		span := trace.SpanFromContext(r.Context())
		if !span.IsRecording() {
			return
		}

		// Header attrs are NOT gated by onErrorOnly (headers are not bodies) and
		// are set regardless of whether body capture is enabled.
		if len(reqHeaderKeys) > 0 {
			span.SetAttributes(headerAttrs(reqHeaderKeys, r.Header)...)
		}
		// Response headers are normally snapshotted in WriteHeader. A handler that
		// sets headers but returns without ever calling Write/WriteHeader still
		// sends them via net/http's implicit 200 — capture those here as a fallback.
		if len(respHeaderKeys) > 0 && !rw.wroteHeader {
			rw.respHeaderAttrs = headerAttrs(respHeaderKeys, rw.Header())
		}
		if len(rw.respHeaderAttrs) > 0 {
			span.SetAttributes(rw.respHeaderAttrs...)
		}

		// Body attrs keep the onErrorOnly gate.
		if onErrorOnly && rw.status < 400 {
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

// normalizeHeaderKeys maps each allowlisted header's canonical (textproto) name
// to its full OTel attribute key. The key normalization mirrors the OpenTelemetry
// Go SDK's semconv header() helper exactly — lowercase, then '-' replaced by '_',
// prefixed — so keys are byte-identical to what otelhttp emits (X-Last9-Client +
// "http.request.header." -> http.request.header.x_last9_client). Baking the full
// key in at construction keeps per-request work to a map lookup. The canonical
// name is the map key for http.Header.Values lookups. Empty names are skipped;
// returns nil for empty input.
func normalizeHeaderKeys(prefix string, names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]string, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		canonical := http.CanonicalHeaderKey(name)
		m[canonical] = prefix + strings.ReplaceAll(strings.ToLower(canonical), "-", "_")
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// headerAttrs builds span attributes for the allowlisted headers present in h.
// HTTP headers are multi-valued, so each is recorded as a StringSlice under its
// full attribute key (e.g. http.request.header.x_last9_client).
func headerAttrs(keys map[string]string, h http.Header) []attribute.KeyValue {
	if len(keys) == 0 {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, len(keys))
	for canonical, attrKey := range keys {
		if vals := h.Values(canonical); len(vals) > 0 {
			attrs = append(attrs, attribute.StringSlice(attrKey, vals))
		}
	}
	return attrs
}

// captureResponseWriter wraps http.ResponseWriter to record status code and body.
//
// It embeds the http.ResponseWriter *interface*, which promotes only Header,
// Write, and WriteHeader — NOT the optional interfaces (http.Hijacker,
// http.Flusher, http.Pusher) that the concrete underlying writer may implement.
// Those are forwarded explicitly below; without Hijack() in particular, wrapping
// this middleware around a handler breaks WebSocket/SSE upgrades (ENG-1278).
// Unwrap() additionally lets http.ResponseController reach the underlying
// writer's deadline/full-duplex methods that are not forwarded explicitly.
//
// When onErrorOnly=true, buf is allocated lazily in WriteHeader only for error responses,
// keeping the successful-request path allocation-free.
//
// Field ordering is optimized for GC pointer scan bytes (pointer fields precede scalars).
type captureResponseWriter struct {
	http.ResponseWriter
	buf             *limitedBuffer       // nil until WriteHeader when onErrorOnly=true
	respHeaderKeys  map[string]string    // allowlist of response headers (canonical->suffix), nil if none
	respHeaderAttrs []attribute.KeyValue // allowlisted response headers snapshotted at WriteHeader time
	respContentType string               // Content-Type snapshotted at WriteHeader time
	contentTypes    []string
	maxBytes        int64
	status          int
	onErrorOnly     bool
	bodyCapture     bool
	wroteHeader     bool
}

func (rw *captureResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.respContentType = rw.Header().Get("Content-Type")
		if rw.bodyCapture && (!rw.onErrorOnly || code >= 400) {
			rw.buf = newLimitedBuffer(rw.maxBytes)
		}
		// Snapshot response headers now: modifying the map after WriteHeader has
		// no effect on the wire, so this captures exactly what was sent.
		if len(rw.respHeaderKeys) > 0 {
			rw.respHeaderAttrs = headerAttrs(rw.respHeaderKeys, rw.Header())
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

// Hijack forwards to the embedded writer's Hijack when supported, letting a
// handler take over the connection (WebSocket upgrades, SSE). Returns
// http.ErrNotSupported when the underlying writer is not an http.Hijacker.
func (rw *captureResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush forwards to the embedded writer's Flush when supported, so streaming
// responses are not buffered by this wrapper. No-op otherwise.
func (rw *captureResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Push forwards HTTP/2 server push to the embedded writer when supported.
// Returns http.ErrNotSupported when the underlying writer is not an http.Pusher.
func (rw *captureResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// Unwrap exposes the wrapped writer so http.ResponseController can walk the
// chain to reach optional methods this wrapper does not forward explicitly —
// notably SetReadDeadline/SetWriteDeadline/EnableFullDuplex, which long-lived
// SSE and WebSocket handlers use for idle timeouts.
func (rw *captureResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
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
