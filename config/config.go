// Package config handles configuration loading from environment variables
package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// Config holds the agent configuration
type Config struct {
	Headers            map[string]string
	ServiceName        string
	ServiceVersion     string
	Environment        string
	Endpoint           string
	Sampler            string
	ResourceAttributes []attribute.KeyValue
	SampleRate         float64
}

// Load reads configuration from environment variables.
//
// Note: If OTEL_EXPORTER_OTLP_ENDPOINT is not set, the agent will start but
// telemetry data will not be exported. This is useful for local development
// or when using a custom exporter configuration.
func Load() *Config {
	cfg := &Config{
		ServiceName:    getEnvOrDefault("OTEL_SERVICE_NAME", "unknown-service"),
		ServiceVersion: os.Getenv("OTEL_SERVICE_VERSION"),
		Endpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Headers:        parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		Sampler:        getEnvOrDefault("OTEL_TRACES_SAMPLER", "always_on"),
		SampleRate:     parseSampleRate(os.Getenv("LAST9_TRACE_SAMPLE_RATE")),
	}

	// Parse resource attributes
	cfg.ResourceAttributes, cfg.Environment, cfg.ServiceVersion = parseResourceAttributes(
		os.Getenv("OTEL_RESOURCE_ATTRIBUTES"),
		cfg.ServiceVersion,
	)

	// Validate configuration
	if cfg.Endpoint == "" {
		log.Println("[Last9 Agent] Warning: OTEL_EXPORTER_OTLP_ENDPOINT not set - telemetry will not be exported")
		log.Println("[Last9 Agent] Set this environment variable to export telemetry data")
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
// and extracts special attributes like deployment.environment and service.version
// Expected format: "key1=value1,key2=value2"
func parseResourceAttributes(attrsStr string, serviceVersion string) ([]attribute.KeyValue, string, string) {
	var attrs []attribute.KeyValue
	environment := "production" // default

	if attrsStr == "" {
		return attrs, environment, serviceVersion
	}

	pairs := strings.Split(attrsStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			// Extract special attributes
			switch key {
			case "deployment.environment":
				environment = value
			case "service.version":
				if serviceVersion == "" {
					serviceVersion = value
				}
			}

			attrs = append(attrs, attribute.String(key, value))
		}
	}

	return attrs, environment, serviceVersion
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseSampleRate parses LAST9_TRACE_SAMPLE_RATE into a float64.
// Returns -1 when the env var is empty (unset), so callers can distinguish
// "not configured" from "configured as 0.0" (sample nothing).
func parseSampleRate(raw string) float64 {
	if raw == "" {
		return -1 // unset
	}
	rate, err := strconv.ParseFloat(raw, 64)
	if err != nil || rate < 0 || rate > 1 {
		log.Printf("[Last9 Agent] Warning: Invalid LAST9_TRACE_SAMPLE_RATE %q (must be 0.0–1.0), ignoring", raw)
		return -1
	}
	return rate
}
