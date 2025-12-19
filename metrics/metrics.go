// Package metrics provides convenient helpers for custom application metrics.
// Use this package to add counters, gauges, and histograms to your application.
package metrics

import (
	"context"
	"log"

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
//
// Example:
//
//	requestCounter := metrics.NewCounter(
//	    "http.requests",
//	    "Total number of HTTP requests",
//	    "{request}",
//	)
//	requestCounter.Add(ctx, 1, attribute.String("method", "GET"))
func NewCounter(name, description, unit string) *Counter {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &Counter{}
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Int64Counter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create counter %s: %v", name, err)
		return &Counter{}
	}

	return &Counter{counter: counter}
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
func NewFloatCounter(name, description, unit string) *FloatCounter {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &FloatCounter{}
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Float64Counter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create float counter %s: %v", name, err)
		return &FloatCounter{}
	}

	return &FloatCounter{counter: counter}
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
//
// Example:
//
//	latencyHistogram := metrics.NewHistogram(
//	    "request.duration",
//	    "Request duration in milliseconds",
//	    "ms",
//	)
//	latencyHistogram.Record(ctx, durationMs, attribute.String("endpoint", "/api/users"))
func NewHistogram(name, description, unit string) *Histogram {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &Histogram{}
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	histogram, err := meter.Int64Histogram(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create histogram %s: %v", name, err)
		return &Histogram{}
	}

	return &Histogram{histogram: histogram}
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
//
// Example:
//
//	responseSize := metrics.NewFloatHistogram(
//	    "response.size",
//	    "Response size in kilobytes",
//	    "kB",
//	)
//	responseSize.Record(ctx, sizeKB, attribute.String("content_type", "application/json"))
func NewFloatHistogram(name, description, unit string) *FloatHistogram {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &FloatHistogram{}
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	histogram, err := meter.Float64Histogram(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create float histogram %s: %v", name, err)
		return &FloatHistogram{}
	}

	return &FloatHistogram{histogram: histogram}
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
//
// Example:
//
//	var activeConnections int64
//	connectionGauge := metrics.NewGauge(
//	    "active.connections",
//	    "Number of active connections",
//	    "{connection}",
//	    func(ctx context.Context) int64 {
//	        return atomic.LoadInt64(&activeConnections)
//	    },
//	)
func NewGauge(name, description, unit string, callback func(context.Context) int64) *Gauge {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &Gauge{}
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
		log.Printf("[Last9 Agent] Warning: Failed to create gauge %s: %v", name, err)
		return &Gauge{}
	}

	return &Gauge{gauge: gauge}
}

// FloatGauge represents a floating point value that can go up and down.
type FloatGauge struct {
	gauge metric.Float64ObservableGauge
}

// NewFloatGauge creates a new floating point gauge metric with a callback function.
//
// Example:
//
//	var cpuUsage float64
//	cpuGauge := metrics.NewFloatGauge(
//	    "cpu.usage",
//	    "CPU usage percentage",
//	    "%",
//	    func(ctx context.Context) float64 {
//	        return getCPUUsage()
//	    },
//	)
func NewFloatGauge(name, description, unit string, callback func(context.Context) float64) *FloatGauge {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &FloatGauge{}
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
		log.Printf("[Last9 Agent] Warning: Failed to create float gauge %s: %v", name, err)
		return &FloatGauge{}
	}

	return &FloatGauge{gauge: gauge}
}

// UpDownCounter is a metric that can increase or decrease.
// Unlike Counter (which only increases), this can go down.
type UpDownCounter struct {
	counter metric.Int64UpDownCounter
}

// NewUpDownCounter creates a new up-down counter metric.
//
// Example:
//
//	queueSize := metrics.NewUpDownCounter(
//	    "queue.size",
//	    "Number of items in queue",
//	    "{item}",
//	)
//	queueSize.Add(ctx, 5)  // Add 5 items
//	queueSize.Add(ctx, -3) // Remove 3 items
func NewUpDownCounter(name, description, unit string) *UpDownCounter {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v", err)
			return &UpDownCounter{}
		}
	}

	meter := otel.Meter("github.com/last9/go-agent/metrics")
	counter, err := meter.Int64UpDownCounter(
		name,
		metric.WithDescription(description),
		metric.WithUnit(unit),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create up-down counter %s: %v", name, err)
		return &UpDownCounter{}
	}

	return &UpDownCounter{counter: counter}
}

// Add adds the given delta to the counter (can be negative).
func (c *UpDownCounter) Add(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if c.counter != nil {
		c.counter.Add(ctx, value, metric.WithAttributes(attrs...))
	}
}
