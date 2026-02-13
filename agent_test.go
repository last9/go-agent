//go:build test

package agent

import (
	"os"
	"testing"
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
		name         string
		samplerName  string
		wantNil      bool
		description  string
	}{
		{"always_on", "always_on", false, "should create AlwaysSample sampler"},
		{"always_off", "always_off", false, "should create NeverSample sampler"},
		{"empty", "", false, "should default to AlwaysSample"},
		{"traceidratio", "traceidratio", false, "should create TraceIDRatioBased sampler"},
		{"parentbased_always_on", "parentbased_always_on", false, "should create ParentBased(AlwaysSample) sampler"},
		{"unknown", "invalid_sampler", false, "should default to AlwaysSample with warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := createSampler(tt.samplerName)
			if sampler == nil {
				t.Errorf("createSampler(%q) returned nil, expected non-nil sampler", tt.samplerName)
			}
		})
	}
}

func TestGetRouteMatcher(t *testing.T) {
	defer Reset()

	// Before Start, returns nil
	rm := GetRouteMatcher()
	if rm != nil {
		t.Error("GetRouteMatcher() should return nil before Start()")
	}
	// Nil receiver is safe — should not exclude anything
	if rm.ShouldExclude("/health") {
		t.Error("nil RouteMatcher should not exclude /health")
	}

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	// Use defaults (health endpoints excluded)
	os.Unsetenv("LAST9_EXCLUDED_PATHS")
	os.Unsetenv("LAST9_EXCLUDED_PATH_PREFIXES")
	os.Unsetenv("LAST9_EXCLUDED_PATH_PATTERNS")

	if err := Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	rm = GetRouteMatcher()
	if rm == nil {
		t.Fatal("GetRouteMatcher() should return non-nil after Start()")
	}
	if rm.IsEmpty() {
		t.Error("RouteMatcher should not be empty with default exclusion rules")
	}

	// Default exact excludes /health
	if !rm.ShouldExclude("/health") {
		t.Error("default RouteMatcher should exclude /health")
	}
	// Default pattern excludes /v1/health
	if !rm.ShouldExclude("/v1/health") {
		t.Error("default RouteMatcher should exclude /v1/health via glob pattern")
	}
	// Should not exclude normal paths
	if rm.ShouldExclude("/api/users") {
		t.Error("default RouteMatcher should not exclude /api/users")
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
