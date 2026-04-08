// Package codeattr provides a SpanProcessor that adds code.function,
// code.filepath, and code.lineno attributes to outbound spans by walking
// the call stack at span creation time.
//
// Only client, producer, and consumer spans are enriched — these correspond
// to DB queries, HTTP client calls, Kafka messages, Redis commands, etc.
// Server and internal spans are skipped to keep overhead minimal.
package codeattr

import (
	"context"
	"runtime"
	"strings"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

// skipPrefixes lists fully-qualified function name prefixes that belong to
// instrumentation libraries or the Go standard library. Frames matching any
// of these are skipped when searching for the application call site.
var skipPrefixes = []string{
	"go.opentelemetry.io/",
	"go.nhat.io/",
	"github.com/last9/go-agent",
	"database/sql.",
	"net/http.",
	"runtime.",
	"testing.",
	"reflect.",
	"github.com/IBM/sarama",
	"github.com/redis/",
	"go.mongodb.org/",
	"github.com/aws/aws-sdk-go-v2",
	"github.com/segmentio/kafka-go",
	"github.com/Shopify/sarama",
}

// Processor is a SpanProcessor that adds code.* attributes to outbound spans.
type Processor struct{}

var _ sdktrace.SpanProcessor = (*Processor)(nil)

type pcsBuffer struct{ pcs [64]uintptr }

// pcPool reuses the uintptr backing array used by runtime.Callers to avoid a
// per-span heap allocation on every client/producer/consumer span start.
var pcPool = sync.Pool{New: func() any { return &pcsBuffer{} }}

// New returns a new Processor.
func New() *Processor { return &Processor{} }

// OnStart adds code.function, code.filepath, and code.lineno to client,
// producer, and consumer spans. All other span kinds are left untouched.
func (p *Processor) OnStart(_ context.Context, s sdktrace.ReadWriteSpan) {
	switch s.SpanKind() {
	case trace.SpanKindClient, trace.SpanKindProducer, trace.SpanKindConsumer:
	default:
		return
	}

	f, ok := callerFrame()
	if !ok {
		return
	}

	s.SetAttributes(
		semconv.CodeFunctionKey.String(shortFuncName(f.Function)),
		semconv.CodeFilepathKey.String(f.File),
		semconv.CodeLineNumberKey.Int(f.Line),
	)
}

func (p *Processor) OnEnd(sdktrace.ReadOnlySpan)      {}
func (p *Processor) Shutdown(context.Context) error   { return nil }
func (p *Processor) ForceFlush(context.Context) error { return nil }

// callerFrame walks the call stack and returns the first frame that is not
// part of a known instrumentation or standard library package.
func callerFrame() (runtime.Frame, bool) {
	buf, ok := pcPool.Get().(*pcsBuffer)
	if !ok || buf == nil {
		buf = &pcsBuffer{}
	}
	defer pcPool.Put(buf)

	n := runtime.Callers(2, buf.pcs[:])
	if n == 0 {
		return runtime.Frame{}, false
	}

	frames := runtime.CallersFrames(buf.pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !isSkipped(f.Function) {
			return f, true
		}
		if !more {
			break
		}
	}
	return runtime.Frame{}, false
}

// isSkipped returns true if fn starts with any of the known library prefixes.
func isSkipped(fn string) bool {
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(fn, prefix) {
			return true
		}
	}
	return false
}

// shortFuncName trims the module path from a fully-qualified function name.
// "github.com/myorg/app/handlers.(*Server).Handle" → "handlers.(*Server).Handle"
func shortFuncName(fn string) string {
	if idx := strings.LastIndex(fn, "/"); idx >= 0 {
		return fn[idx+1:]
	}
	return fn
}
