// Package config handles configuration loading from environment variables
package config

import (
	"log"
	"os"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// Config holds the agent configuration
type Config struct {
	// ServiceName is the name of the service (from OTEL_SERVICE_NAME)
	ServiceName string

	// Environment is the deployment environment (from OTEL_RESOURCE_ATTRIBUTES or defaults to "production")
	Environment string

	// Endpoint is the OTLP endpoint (from OTEL_EXPORTER_OTLP_ENDPOINT)
	Endpoint string

	// Headers contains additional headers for OTLP requests (from OTEL_EXPORTER_OTLP_HEADERS)
	Headers map[string]string

	// ResourceAttributes contains additional resource attributes
	ResourceAttributes []attribute.KeyValue

	// Sampler is the trace sampling strategy (from OTEL_TRACES_SAMPLER)
	Sampler string
}

// Load reads configuration from environment variables
func Load() *Config {
	cfg := &Config{
		ServiceName: getEnvOrDefault("OTEL_SERVICE_NAME", "unknown-service"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Headers:     parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		Sampler:     getEnvOrDefault("OTEL_TRACES_SAMPLER", "always_on"),
	}

	// Parse resource attributes
	cfg.ResourceAttributes, cfg.Environment = parseResourceAttributes(
		os.Getenv("OTEL_RESOURCE_ATTRIBUTES"),
	)

	// Validate configuration
	if cfg.Endpoint == "" {
		log.Println("[Last9 Agent] Warning: OTEL_EXPORTER_OTLP_ENDPOINT not set")
	}

	return cfg
}

// parseHeaders parses the OTEL_EXPORTER_OTLP_HEADERS environment variable
// Expected format: "key1=value1,key2=value2"
func parseHeaders(headersStr string) map[string]string {
	headers := make(map[string]string)
	if headersStr == "" {
		return headers
	}

	pairs := strings.Split(headersStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return headers
}

// parseResourceAttributes parses the OTEL_RESOURCE_ATTRIBUTES environment variable
// and extracts the deployment.environment if present
// Expected format: "key1=value1,key2=value2"
func parseResourceAttributes(attrsStr string) ([]attribute.KeyValue, string) {
	var attrs []attribute.KeyValue
	environment := "production" // default

	if attrsStr == "" {
		return attrs, environment
	}

	pairs := strings.Split(attrsStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			// Extract environment
			if key == "deployment.environment" {
				environment = value
			}

			attrs = append(attrs, attribute.String(key, value))
		}
	}

	return attrs, environment
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
