//go:build !go1.24

package agent

import (
	"context"
	"log"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// startRuntimeInstrumentation provides basic runtime metrics without
// the contrib/instrumentation/runtime dependency.
//
// This legacy implementation collects essential Go runtime metrics using
// only the standard library. It provides:
//   - runtime.go.goroutines: Number of goroutines
//   - runtime.go.mem.heap_alloc: Bytes of allocated heap objects
//   - runtime.go.gc.count: Number of completed GC cycles
//
// This version is used for Go 1.22 and 1.23, which cannot use the full
// contrib/instrumentation/runtime package.
func startRuntimeInstrumentation(interval time.Duration) error {
	log.Printf("[Last9 Agent] Using legacy runtime instrumentation (Go 1.22/1.23) - basic metrics only")

	meter := otel.Meter("github.com/last9/go-agent/runtime-legacy")

	// Register basic goroutine counter
	_, err := meter.Int64ObservableGauge(
		"runtime.go.goroutines",
		metric.WithDescription("Number of goroutines that currently exist"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(runtime.NumGoroutine()))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	// Register basic memory stats
	// Note: We use a closure to capture memStats to avoid repeated allocations
	var memStats runtime.MemStats
	_, err = meter.Int64ObservableGauge(
		"runtime.go.mem.heap_alloc",
		metric.WithDescription("Bytes of allocated heap objects"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			runtime.ReadMemStats(&memStats)
			o.Observe(int64(memStats.HeapAlloc))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	// Register GC count
	_, err = meter.Int64ObservableCounter(
		"runtime.go.gc.count",
		metric.WithDescription("Number of completed GC cycles"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			runtime.ReadMemStats(&memStats)
			o.Observe(int64(memStats.NumGC))
			return nil
		}),
	)

	return err
}
