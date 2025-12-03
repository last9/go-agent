// Package agent provides a drop-in OpenTelemetry agent for Last9
// that minimizes code changes required for instrumentation.
//
// Basic usage:
//
//	import "github.com/last9/go-agent"
//
//	func main() {
//	    agent.Start()
//	    defer agent.Shutdown()
//	    // Your application code
//	}
package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/last9/go-agent/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	globalAgent *Agent
	once        sync.Once
)

// Agent represents the Last9 telemetry agent
type Agent struct {
	config         *config.Config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	shutdown       func(context.Context) error
}

// Start initializes the Last9 agent with configuration from environment variables.
// It sets up OpenTelemetry tracing and metrics exporters configured for Last9.
//
// Environment variables:
//   - OTEL_EXPORTER_OTLP_ENDPOINT: Last9 OTLP endpoint (required)
//   - OTEL_EXPORTER_OTLP_HEADERS: Authorization header (required)
//   - OTEL_SERVICE_NAME: Service name (default: "unknown-service")
//   - OTEL_RESOURCE_ATTRIBUTES: Additional resource attributes as key=value pairs
//   - OTEL_TRACES_SAMPLER: Trace sampling strategy (default: "always_on")
//
// Example:
//
//	export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp.last9.io"
//	export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <your-token>"
//	export OTEL_SERVICE_NAME="my-service"
//
// Start must be called before any instrumentation is used.
// It's safe to call multiple times - only the first call has effect.
func Start() error {
	var err error
	once.Do(func() {
		cfg := config.Load()

		// Create resource
		res, resErr := createResource(cfg)
		if resErr != nil {
			err = fmt.Errorf("failed to create resource: %w", resErr)
			return
		}

		// Initialize tracer provider
		tp, tpErr := initTracerProvider(res)
		if tpErr != nil {
			err = fmt.Errorf("failed to initialize tracer provider: %w", tpErr)
			return
		}

		// Initialize meter provider
		mp, mpErr := initMeterProvider(res)
		if mpErr != nil {
			err = fmt.Errorf("failed to initialize meter provider: %w", mpErr)
			return
		}

		// Set global providers
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)

		globalAgent = &Agent{
			config:         cfg,
			tracerProvider: tp,
			meterProvider:  mp,
			shutdown: func(ctx context.Context) error {
				var errs []error
				if err := tp.Shutdown(ctx); err != nil {
					errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
				}
				if err := mp.Shutdown(ctx); err != nil {
					errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
				}
				if len(errs) > 0 {
					return fmt.Errorf("shutdown errors: %v", errs)
				}
				return nil
			},
		}

		log.Printf("[Last9 Agent] Started successfully for service: %s", cfg.ServiceName)
	})
	return err
}

// Shutdown gracefully shuts down the agent, flushing any pending telemetry.
// It should be called before application exit, typically with defer.
//
// Example:
//
//	agent.Start()
//	defer agent.Shutdown()
func Shutdown() error {
	if globalAgent == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := globalAgent.shutdown(ctx); err != nil {
		return fmt.Errorf("agent shutdown failed: %w", err)
	}

	log.Println("[Last9 Agent] Shutdown complete")
	return nil
}

// IsInitialized returns true if the agent has been started
func IsInitialized() bool {
	return globalAgent != nil
}

// GetConfig returns the agent configuration (or nil if not initialized)
func GetConfig() *config.Config {
	if globalAgent == nil {
		return nil
	}
	return globalAgent.config
}

// createResource creates an OpenTelemetry resource with service information
func createResource(cfg *config.Config) (*resource.Resource, error) {
	// Build base attributes
	baseAttrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(cfg.ServiceName),
		semconv.DeploymentEnvironmentKey.String(cfg.Environment),
	}

	// Add service version if available
	if cfg.ServiceVersion != "" {
		baseAttrs = append(baseAttrs, semconv.ServiceVersionKey.String(cfg.ServiceVersion))
	}

	attrs := []resource.Option{
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithAttributes(baseAttrs...),
	}

	// Add custom attributes from config
	if len(cfg.ResourceAttributes) > 0 {
		attrs = append(attrs, resource.WithAttributes(cfg.ResourceAttributes...))
	}

	return resource.New(context.Background(), attrs...)
}

// initTracerProvider creates and configures the trace provider
func initTracerProvider(res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	return tp, nil
}

// initMeterProvider creates and configures the meter provider
func initMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(
			metric.NewPeriodicReader(exporter, metric.WithInterval(1*time.Minute)),
		),
	)

	return mp, nil
}
