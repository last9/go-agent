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

	// ExcludedPaths is a list of exact URL paths to exclude from tracing (from LAST9_EXCLUDED_PATHS).
	// Default: /health,/healthz,/metrics,/ready,/live,/ping
	// Set LAST9_EXCLUDED_PATHS="" to disable defaults.
	ExcludedPaths []string

	// ExcludedPathPrefixes is a list of URL path prefixes to exclude from tracing (from LAST9_EXCLUDED_PATH_PREFIXES).
	// Default: none
	ExcludedPathPrefixes []string

	// ExcludedPathPatterns is a list of glob patterns to exclude from tracing (from LAST9_EXCLUDED_PATH_PATTERNS).
	// Uses path.Match semantics (not filepath.Match).
	// Default: /*/health,/*/healthz,/*/metrics,/*/ready,/*/live,/*/ping
	// Set LAST9_EXCLUDED_PATH_PATTERNS="" to disable defaults.
	ExcludedPathPatterns []string

	// HTTP body capture configuration (all four fields controlled by LAST9_BODY_CAPTURE_* env vars)
	BodyCaptureContentTypes []string // LAST9_BODY_CAPTURE_CONTENT_TYPES; default: application/json,application/xml,text/plain

	// BodyCaptureMaxBytes is the maximum bytes captured per body (LAST9_BODY_CAPTURE_MAX_BYTES).
	// Default: 8192. Set to 0 to capture no bytes (middleware overhead still applies;
	// use BodyCaptureEnabled=false to disable entirely).
	BodyCaptureMaxBytes int64

	// BodyCaptureEnabled enables HTTP request/response body capture (LAST9_BODY_CAPTURE_ENABLED).
	// Default: false — opt-in, bodies may contain PII.
	BodyCaptureEnabled bool

	// BodyCaptureOnErrorOnly restricts capture to status >= 400 (LAST9_BODY_CAPTURE_ON_ERROR_ONLY).
	// Default: false.
	BodyCaptureOnErrorOnly bool

	SampleRate float64
	// SamplerRatio is the sampling ratio for traceidratio samplers (0.0-1.0).
	// Only used when Sampler is "traceidratio" or "parentbased_traceidratio".
	// Set via WithSamplingRate() option. Zero value means use OTEL_TRACES_SAMPLER_ARG env var.
	SamplerRatio float64
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

	// Parse body capture configuration
	cfg.BodyCaptureEnabled = parseBoolEnv("LAST9_BODY_CAPTURE_ENABLED", false)
	cfg.BodyCaptureMaxBytes = parseInt64Env("LAST9_BODY_CAPTURE_MAX_BYTES", 8192)
	cfg.BodyCaptureOnErrorOnly = parseBoolEnv("LAST9_BODY_CAPTURE_ON_ERROR_ONLY", false)
	cfg.BodyCaptureContentTypes = parseCommaSeparatedWithDefault(
		"LAST9_BODY_CAPTURE_CONTENT_TYPES",
		"application/json,application/xml,text/plain",
	)

	// Parse route exclusion configuration
	cfg.ExcludedPaths = parseCommaSeparatedWithDefault(
		"LAST9_EXCLUDED_PATHS",
		"/health,/healthz,/metrics,/ready,/live,/ping",
	)
	cfg.ExcludedPathPrefixes = parseCommaSeparatedWithDefault(
		"LAST9_EXCLUDED_PATH_PREFIXES",
		"",
	)
	cfg.ExcludedPathPatterns = parseCommaSeparatedWithDefault(
		"LAST9_EXCLUDED_PATH_PATTERNS",
		"/*/health,/*/healthz,/*/metrics,/*/ready,/*/live,/*/ping",
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

// parseCommaSeparatedWithDefault reads an env var and splits it by comma.
// If the env var is not set, defaultVal is used. If the env var is explicitly
// set to "", an empty slice is returned (opt-out of defaults).
func parseCommaSeparatedWithDefault(envKey, defaultVal string) []string {
	raw, ok := os.LookupEnv(envKey)
	if !ok {
		raw = defaultVal
	}
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			result = append(result, v)
		}
	}
	return result
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseBoolEnv reads an env var as bool. Accepts "true"/"1" (case-insensitive).
func parseBoolEnv(key string, defaultVal bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Invalid bool for %s=%q, using default %v", key, raw, defaultVal)
		return defaultVal
	}
	return v
}

// parseInt64Env reads an env var as int64.
func parseInt64Env(key string, defaultVal int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		log.Printf("[Last9 Agent] Warning: Invalid int64 for %s=%q, using default %d", key, raw, defaultVal)
		return defaultVal
	}
	return v
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
