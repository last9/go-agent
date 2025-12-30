// Package metrics provides convenient helpers for custom application metrics.
// Use this package to add counters, gauges, and histograms to your application.
package metrics

import (
	"context"
	"fmt"

	"github.com/last9/go-agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Counter is a monotonically increasing metric.
// Use for counting events like requests, errors, cache hits, etc.
type Counter struct {
	counter metric.Int64Counter
}

// NewCounter creates a new counter metric.
// Returns an error if the agent cannot be initialized or the counter cannot be created.
//
// Example:
//
//	requestCounter, err := metrics.NewCounter(
//	    "http.requests",
//	    "Total number of HTTP requests",
//	    "{request}",
//	)
//	if err != nil {
//	    log.Printf("Failed to create counter: %v", err)
//	    return err
//	}
//	requestCounter.Add(ctx, 1, attribute.String("method", "GET"))
func NewCounter(name, description, unit string) (*Counter, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Int64Counter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create counter %s: %w", name, err)
	}

	return &Counter{counter: counter}, nil
}

// Add increments the counter by the given value.
func (c *Counter) Add(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if c.counter != nil {
		c.counter.Add(ctx, value, metric.WithAttributes(attrs...))
	}
}

// Inc increments the counter by 1.
func (c *Counter) Inc(ctx context.Context, attrs ...attribute.KeyValue) {
	c.Add(ctx, 1, attrs...)
}

// FloatCounter is a monotonically increasing metric for floating point values.
type FloatCounter struct {
	counter metric.Float64Counter
}

// NewFloatCounter creates a new floating point counter metric.
// Returns an error if the agent cannot be initialized or the counter cannot be created.
func NewFloatCounter(name, description, unit string) (*FloatCounter, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Float64Counter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create float counter %s: %w", name, err)
	}

	return &FloatCounter{counter: counter}, nil
}

// Add increments the counter by the given value.
func (c *FloatCounter) Add(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	if c.counter != nil {
		c.counter.Add(ctx, value, metric.WithAttributes(attrs...))
	}
}

// Histogram records a distribution of values.
// Use for latencies, request sizes, etc.
type Histogram struct {
	histogram metric.Int64Histogram
}

// NewHistogram creates a new histogram metric.
// Returns an error if the agent cannot be initialized or the histogram cannot be created.
//
// Example:
//
//	latencyHistogram, err := metrics.NewHistogram(
//	    "request.duration",
//	    "Request duration in milliseconds",
//	    "ms",
//	)
//	if err != nil {
//	    log.Printf("Failed to create histogram: %v", err)
//	    return err
//	}
//	latencyHistogram.Record(ctx, durationMs, attribute.String("endpoint", "/api/users"))
func NewHistogram(name, description, unit string) (*Histogram, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	histogram, err := meter.Int64Histogram(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create histogram %s: %w", name, err)
	}

	return &Histogram{histogram: histogram}, nil
}

// Record records a value in the histogram.
func (h *Histogram) Record(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if h.histogram != nil {
		h.histogram.Record(ctx, value, metric.WithAttributes(attrs...))
	}
}

// FloatHistogram records a distribution of floating point values.
type FloatHistogram struct {
	histogram metric.Float64Histogram
}

// NewFloatHistogram creates a new floating point histogram metric.
// Returns an error if the agent cannot be initialized or the histogram cannot be created.
//
// Example:
//
//	responseSize, err := metrics.NewFloatHistogram(
//	    "response.size",
//	    "Response size in kilobytes",
//	    "kB",
//	)
//	if err != nil {
//	    log.Printf("Failed to create histogram: %v", err)
//	    return err
//	}
//	responseSize.Record(ctx, sizeKB, attribute.String("content_type", "application/json"))
func NewFloatHistogram(name, description, unit string) (*FloatHistogram, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	histogram, err := meter.Float64Histogram(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create float histogram %s: %w", name, err)
	}

	return &FloatHistogram{histogram: histogram}, nil
}

// Record records a value in the histogram.
func (h *FloatHistogram) Record(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	if h.histogram != nil {
		h.histogram.Record(ctx, value, metric.WithAttributes(attrs...))
	}
}

// Gauge represents a value that can go up and down.
// Use for things like active connections, queue size, memory usage, etc.
type Gauge struct {
	gauge metric.Int64ObservableGauge
}

// NewGauge creates a new gauge metric with a callback function.
// The callback is invoked periodically to read the current value.
// Returns an error if the agent cannot be initialized or the gauge cannot be created.
//
// Example:
//
//	var activeConnections int64
//	connectionGauge, err := metrics.NewGauge(
//	    "active.connections",
//	    "Number of active connections",
//	    "{connection}",
//	    func(ctx context.Context) int64 {
//	        return atomic.LoadInt64(&activeConnections)
//	    },
//	)
//	if err != nil {
//	    log.Printf("Failed to create gauge: %v", err)
//	    return err
//	}
func NewGauge(name, description, unit string, callback func(context.Context) int64) (*Gauge, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	gauge, err := meter.Int64ObservableGauge(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			value := callback(ctx)
			observer.Observe(value)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gauge %s: %w", name, err)
	}

	return &Gauge{gauge: gauge}, nil
}

// FloatGauge represents a floating point value that can go up and down.
type FloatGauge struct {
	gauge metric.Float64ObservableGauge
}

// NewFloatGauge creates a new floating point gauge metric with a callback function.
// Returns an error if the agent cannot be initialized or the gauge cannot be created.
//
// Example:
//
//	var cpuUsage float64
//	cpuGauge, err := metrics.NewFloatGauge(
//	    "cpu.usage",
//	    "CPU usage percentage",
//	    "%",
//	    func(ctx context.Context) float64 {
//	        return getCPUUsage()
//	    },
//	)
//	if err != nil {
//	    log.Printf("Failed to create gauge: %v", err)
//	    return err
//	}
func NewFloatGauge(name, description, unit string, callback func(context.Context) float64) (*FloatGauge, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	gauge, err := meter.Float64ObservableGauge(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
		metric.WithFloat64Callback(func(ctx context.Context, observer metric.Float64Observer) error {
			value := callback(ctx)
			observer.Observe(value)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create float gauge %s: %w", name, err)
	}

	return &FloatGauge{gauge: gauge}, nil
}

// UpDownCounter is a metric that can increase or decrease.
// Unlike Counter (which only increases), this can go down.
type UpDownCounter struct {
	counter metric.Int64UpDownCounter
}

// NewUpDownCounter creates a new up-down counter metric.
// Returns an error if the agent cannot be initialized or the counter cannot be created.
//
// Example:
//
//	queueSize, err := metrics.NewUpDownCounter(
//	    "queue.size",
//	    "Number of items in queue",
//	    "{item}",
//	)
//	if err != nil {
//	    log.Printf("Failed to create counter: %v", err)
//	    return err
//	}
//	queueSize.Add(ctx, 5)  // Add 5 items
//	queueSize.Add(ctx, -3) // Remove 3 items
func NewUpDownCounter(name, description, unit string) (*UpDownCounter, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Int64UpDownCounter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create up-down counter %s: %w", name, err)
	}

	return &UpDownCounter{counter: counter}, nil
}

// Add adds the given delta to the counter (can be negative).
func (c *UpDownCounter) Add(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if c.counter != nil {
		c.counter.Add(ctx, value, metric.WithAttributes(attrs...))
	}
}
