//go:build test

package agent

import (
	"os"
	"testing"

	"github.com/last9/go-agent/config"
)

func TestStart(t *testing.T) {
	// Clean up after test
	defer Reset()

	// Set required environment variables
	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	// Start agent
	err := Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify agent is initialized
	if !IsInitialized() {
		t.Error("Agent should be initialized after Start()")
	}

	// Verify config is accessible
	cfg := GetConfig()
	if cfg == nil {
		t.Error("GetConfig() should return non-nil after Start()")
	}
	if cfg.ServiceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got '%s'", cfg.ServiceName)
	}
}

func TestStartMultipleCalls(t *testing.T) {
	defer Reset()

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	// First call should succeed
	err := Start()
	if err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Second call should be a no-op (no error)
	err = Start()
	if err != nil {
		t.Errorf("Second Start() should not return error, got: %v", err)
	}

	// Agent should still be initialized
	if !IsInitialized() {
		t.Error("Agent should still be initialized after multiple Start() calls")
	}
}

func TestIsInitialized(t *testing.T) {
	defer Reset()

	// Before Start()
	if IsInitialized() {
		t.Error("IsInitialized() should return false before Start()")
	}

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	// After Start()
	_ = Start()
	if !IsInitialized() {
		t.Error("IsInitialized() should return true after Start()")
	}

	// After Reset()
	Reset()
	if IsInitialized() {
		t.Error("IsInitialized() should return false after Reset()")
	}
}

func TestGetConfig(t *testing.T) {
	defer Reset()

	// Before Start()
	if GetConfig() != nil {
		t.Error("GetConfig() should return nil before Start()")
	}

	os.Setenv("OTEL_SERVICE_NAME", "my-service")
	os.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	defer os.Unsetenv("OTEL_TRACES_SAMPLER")

	_ = Start()

	cfg := GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() should return non-nil after Start()")
	}

	if cfg.ServiceName != "my-service" {
		t.Errorf("Expected ServiceName 'my-service', got '%s'", cfg.ServiceName)
	}
	if cfg.Sampler != "always_off" {
		t.Errorf("Expected Sampler 'always_off', got '%s'", cfg.Sampler)
	}
}

func TestShutdown(t *testing.T) {
	defer Reset()

	// Shutdown before Start should not error
	err := Shutdown()
	if err != nil {
		t.Errorf("Shutdown() before Start() should not error, got: %v", err)
	}

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	_ = Start()

	// Shutdown after Start - may return network errors when no collector is running,
	// which is expected in test environments. We just verify shutdown completes.
	_ = Shutdown()
}

func TestCreateSampler(t *testing.T) {
	tests := []struct {
		name        string
		samplerName string
		description string
	}{
		{"always_on", "always_on", "should create AlwaysSample sampler"},
		{"always_off", "always_off", "should create NeverSample sampler"},
		{"empty", "", "should default to AlwaysSample"},
		{"traceidratio", "traceidratio", "should create TraceIDRatioBased sampler"},
		{"parentbased_always_on", "parentbased_always_on", "should create ParentBased(AlwaysSample) sampler"},
		{"unknown", "invalid_sampler", "should default to AlwaysSample with warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Sampler: tt.samplerName}
			sampler := createSampler(cfg)
			if sampler == nil {
				t.Errorf("createSampler(%q) returned nil, expected non-nil sampler", tt.samplerName)
			}
		})
	}
}

func TestStartWithOptions(t *testing.T) {
	defer Reset()

	os.Setenv("OTEL_SERVICE_NAME", "env-service")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://env.example.com")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	err := Start(
		WithServiceName("option-service"),
		WithEnvironment("staging"),
		WithEndpoint("https://option.example.com"),
	)
	if err != nil {
		t.Fatalf("Start() with options failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.ServiceName != "option-service" {
		t.Errorf("Expected ServiceName 'option-service', got '%s'", cfg.ServiceName)
	}
	if cfg.Environment != "staging" {
		t.Errorf("Expected Environment 'staging', got '%s'", cfg.Environment)
	}
	if cfg.Endpoint != "https://option.example.com" {
		t.Errorf("Expected Endpoint 'https://option.example.com', got '%s'", cfg.Endpoint)
	}
}

func TestStartWithSamplingRate(t *testing.T) {
	defer Reset()

	err := Start(
		WithServiceName("test-service"),
		WithSamplingRate(0.25),
	)
	if err != nil {
		t.Fatalf("Start() with sampling rate failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.Sampler != "traceidratio" {
		t.Errorf("Expected Sampler 'traceidratio', got '%s'", cfg.Sampler)
	}
	if cfg.SamplerRatio != 0.25 {
		t.Errorf("Expected SamplerRatio 0.25, got %f", cfg.SamplerRatio)
	}
}

func TestStartWithSamplingRateEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		rate        float64
		wantSampler string
		wantRatio   float64
	}{
		{"zero_is_always_off", 0.0, "always_off", 0},
		{"one_is_always_on", 1.0, "always_on", 0},
		{"fractional", 0.5, "traceidratio", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()
			defer Reset()

			err := Start(WithServiceName("test"), WithSamplingRate(tt.rate))
			if err != nil {
				t.Fatalf("Start() failed: %v", err)
			}

			cfg := GetConfig()
			if cfg.Sampler != tt.wantSampler {
				t.Errorf("Expected Sampler %q, got %q", tt.wantSampler, cfg.Sampler)
			}
			if cfg.SamplerRatio != tt.wantRatio {
				t.Errorf("Expected SamplerRatio %f, got %f", tt.wantRatio, cfg.SamplerRatio)
			}
		})
	}
}

func TestStartWithHeaders(t *testing.T) {
	defer Reset()

	headers := map[string]string{
		"Authorization": "Basic test-token",
		"X-Custom":      "value",
	}

	err := Start(
		WithServiceName("test-service"),
		WithHeaders(headers),
	)
	if err != nil {
		t.Fatalf("Start() with headers failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.Headers["Authorization"] != "Basic test-token" {
		t.Errorf("Expected Authorization header, got %v", cfg.Headers)
	}
	if cfg.Headers["X-Custom"] != "value" {
		t.Errorf("Expected X-Custom header, got %v", cfg.Headers)
	}
}

func TestStartBackwardCompatible(t *testing.T) {
	defer Reset()

	os.Setenv("OTEL_SERVICE_NAME", "env-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	err := Start()
	if err != nil {
		t.Fatalf("Start() without options failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.ServiceName != "env-service" {
		t.Errorf("Expected ServiceName from env 'env-service', got '%s'", cfg.ServiceName)
	}
}

func TestOptionsPreserveEnvDefaults(t *testing.T) {
	defer Reset()

	os.Setenv("OTEL_SERVICE_NAME", "env-service")
	os.Setenv("OTEL_SERVICE_VERSION", "v1.0.0")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://env.example.com")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	defer os.Unsetenv("OTEL_SERVICE_VERSION")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	// Override only service name — other env values should be preserved
	err := Start(WithServiceName("option-service"))
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.ServiceName != "option-service" {
		t.Errorf("Expected ServiceName 'option-service', got '%s'", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "v1.0.0" {
		t.Errorf("Expected ServiceVersion 'v1.0.0' from env, got '%s'", cfg.ServiceVersion)
	}
	if cfg.Endpoint != "https://env.example.com" {
		t.Errorf("Expected Endpoint from env, got '%s'", cfg.Endpoint)
	}
}

func TestParseSamplerRatio(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 1.0},           // empty defaults to 1.0
		{"0.5", 0.5},        // valid ratio
		{"0.0", 0.0},        // min valid
		{"1.0", 1.0},        // max valid
		{"invalid", 1.0},    // invalid defaults to 1.0
		{"-0.5", 1.0},       // negative defaults to 1.0
		{"1.5", 1.0},        // > 1.0 defaults to 1.0
		{"0.123456", 0.123456}, // precise value
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSamplerRatio(tt.input)
			if got != tt.want {
				t.Errorf("parseSamplerRatio(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
