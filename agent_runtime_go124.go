//go:build go1.24

package agent

import (
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

// startRuntimeInstrumentation starts full runtime metric collection.
// This version includes comprehensive Go runtime metrics:
//   - Memory: heap_alloc, heap_idle, heap_inuse, heap_sys, etc.
//   - GC: gc_count, gc_pause_ns, gc_pause_total_ns
//   - Goroutines: goroutine count
//   - CPU: uptime, cgo_calls
//   - Plus 10+ other runtime metrics
//
// This implementation uses go.opentelemetry.io/contrib/instrumentation/runtime
// which requires Go 1.24+.
func startRuntimeInstrumentation(interval time.Duration) error {
	return runtime.Start(runtime.WithMinimumReadMemStatsInterval(interval))
}
