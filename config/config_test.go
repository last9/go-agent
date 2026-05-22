package config

import (
	"os"
	"reflect"
	"testing"
)

func TestParseSampleRate(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{"empty returns -1 (unset)", "", -1},
		{"zero is valid", "0.0", 0.0},
		{"half", "0.5", 0.5},
		{"full", "1.0", 1.0},
		{"precise value", "0.123", 0.123},
		{"negative is invalid", "-0.1", -1},
		{"greater than 1 is invalid", "1.5", -1},
		{"non-numeric is invalid", "abc", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSampleRate(tt.raw)
			if got != tt.want {
				t.Errorf("parseSampleRate(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLoad_SampleRate(t *testing.T) {
	// Clean env
	os.Unsetenv("LAST9_TRACE_SAMPLE_RATE")

	t.Run("unset env returns -1", func(t *testing.T) {
		os.Unsetenv("LAST9_TRACE_SAMPLE_RATE")
		cfg := Load()
		if cfg.SampleRate != -1 {
			t.Errorf("expected SampleRate=-1 when unset, got %v", cfg.SampleRate)
		}
	})

	t.Run("set to 0.5", func(t *testing.T) {
		os.Setenv("LAST9_TRACE_SAMPLE_RATE", "0.5")
		defer os.Unsetenv("LAST9_TRACE_SAMPLE_RATE")
		cfg := Load()
		if cfg.SampleRate != 0.5 {
			t.Errorf("expected SampleRate=0.5, got %v", cfg.SampleRate)
		}
	})

	t.Run("set to 0 (sample nothing)", func(t *testing.T) {
		os.Setenv("LAST9_TRACE_SAMPLE_RATE", "0")
		defer os.Unsetenv("LAST9_TRACE_SAMPLE_RATE")
		cfg := Load()
		if cfg.SampleRate != 0 {
			t.Errorf("expected SampleRate=0, got %v", cfg.SampleRate)
		}
	})

	t.Run("invalid value returns -1", func(t *testing.T) {
		os.Setenv("LAST9_TRACE_SAMPLE_RATE", "not-a-number")
		defer os.Unsetenv("LAST9_TRACE_SAMPLE_RATE")
		cfg := Load()
		if cfg.SampleRate != -1 {
			t.Errorf("expected SampleRate=-1 for invalid, got %v", cfg.SampleRate)
		}
	})
}

func TestParseCommaSeparatedWithDefault(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		envVal     *string // nil = not set
		defaultVal string
		want       []string
	}{
		{
			name:       "env not set uses default",
			envKey:     "TEST_PARSE_CSV_1",
			envVal:     nil,
			defaultVal: "/health,/metrics",
			want:       []string{"/health", "/metrics"},
		},
		{
			name:       "env set to empty opts out",
			envKey:     "TEST_PARSE_CSV_2",
			envVal:     strPtr(""),
			defaultVal: "/health,/metrics",
			want:       nil,
		},
		{
			name:       "env set to custom value",
			envKey:     "TEST_PARSE_CSV_3",
			envVal:     strPtr("/custom,/paths"),
			defaultVal: "/health,/metrics",
			want:       []string{"/custom", "/paths"},
		},
		{
			name:       "whitespace trimmed",
			envKey:     "TEST_PARSE_CSV_4",
			envVal:     strPtr(" /health , /metrics "),
			defaultVal: "",
			want:       []string{"/health", "/metrics"},
		},
		{
			name:       "empty default with env not set",
			envKey:     "TEST_PARSE_CSV_5",
			envVal:     nil,
			defaultVal: "",
			want:       nil,
		},
		{
			name:       "trailing comma ignored",
			envKey:     "TEST_PARSE_CSV_6",
			envVal:     strPtr("/health,,/metrics,"),
			defaultVal: "",
			want:       []string{"/health", "/metrics"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(tt.envKey)
			if tt.envVal != nil {
				os.Setenv(tt.envKey, *tt.envVal)
				defer os.Unsetenv(tt.envKey)
			}

			got := parseCommaSeparatedWithDefault(tt.envKey, tt.defaultVal)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCommaSeparatedWithDefault(%q) = %v, want %v", tt.envKey, got, tt.want)
			}
		})
	}
}

func TestLoadExcludedPathsDefaults(t *testing.T) {
	// Ensure env vars are not set so defaults apply
	os.Unsetenv("LAST9_EXCLUDED_PATHS")
	os.Unsetenv("LAST9_EXCLUDED_PATH_PREFIXES")
	os.Unsetenv("LAST9_EXCLUDED_PATH_PATTERNS")

	cfg := Load()

	expectedPaths := []string{"/health", "/healthz", "/metrics", "/ready", "/live", "/ping"}
	if !reflect.DeepEqual(cfg.ExcludedPaths, expectedPaths) {
		t.Errorf("ExcludedPaths = %v, want %v", cfg.ExcludedPaths, expectedPaths)
	}

	if cfg.ExcludedPathPrefixes != nil {
		t.Errorf("ExcludedPathPrefixes = %v, want nil", cfg.ExcludedPathPrefixes)
	}

	expectedPatterns := []string{"/*/health", "/*/healthz", "/*/metrics", "/*/ready", "/*/live", "/*/ping"}
	if !reflect.DeepEqual(cfg.ExcludedPathPatterns, expectedPatterns) {
		t.Errorf("ExcludedPathPatterns = %v, want %v", cfg.ExcludedPathPatterns, expectedPatterns)
	}
}

func TestLoadExcludedPathsOptOut(t *testing.T) {
	// Set env vars to empty to opt out
	os.Setenv("LAST9_EXCLUDED_PATHS", "")
	os.Setenv("LAST9_EXCLUDED_PATH_PATTERNS", "")
	defer os.Unsetenv("LAST9_EXCLUDED_PATHS")
	defer os.Unsetenv("LAST9_EXCLUDED_PATH_PATTERNS")

	cfg := Load()

	if cfg.ExcludedPaths != nil {
		t.Errorf("ExcludedPaths = %v, want nil (opted out)", cfg.ExcludedPaths)
	}
	if cfg.ExcludedPathPatterns != nil {
		t.Errorf("ExcludedPathPatterns = %v, want nil (opted out)", cfg.ExcludedPathPatterns)
	}
}

func TestLoadExcludedPathsCustom(t *testing.T) {
	os.Setenv("LAST9_EXCLUDED_PATHS", "/custom-health")
	os.Setenv("LAST9_EXCLUDED_PATH_PREFIXES", "/internal/")
	os.Setenv("LAST9_EXCLUDED_PATH_PATTERNS", "/v*/status")
	defer os.Unsetenv("LAST9_EXCLUDED_PATHS")
	defer os.Unsetenv("LAST9_EXCLUDED_PATH_PREFIXES")
	defer os.Unsetenv("LAST9_EXCLUDED_PATH_PATTERNS")

	cfg := Load()

	if !reflect.DeepEqual(cfg.ExcludedPaths, []string{"/custom-health"}) {
		t.Errorf("ExcludedPaths = %v, want [/custom-health]", cfg.ExcludedPaths)
	}
	if !reflect.DeepEqual(cfg.ExcludedPathPrefixes, []string{"/internal/"}) {
		t.Errorf("ExcludedPathPrefixes = %v, want [/internal/]", cfg.ExcludedPathPrefixes)
	}
	if !reflect.DeepEqual(cfg.ExcludedPathPatterns, []string{"/v*/status"}) {
		t.Errorf("ExcludedPathPatterns = %v, want [/v*/status]", cfg.ExcludedPathPatterns)
	}
}

func TestParseBoolEnv(t *testing.T) {
	const key = "TEST_PARSE_BOOL_ENV"
	tests := []struct {
		val        *string
		name       string
		want       bool
		defaultVal bool
	}{
		{nil, "not set uses default true", true, true},
		{nil, "not set uses default false", false, false},
		{strPtr("true"), "true", true, false},
		{strPtr("1"), "1", true, false},
		{strPtr("TRUE"), "TRUE", true, false},
		{strPtr("false"), "false", false, true},
		{strPtr("0"), "0", false, true},
		{strPtr("yep"), "invalid falls back to default", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(key)
			if tt.val != nil {
				os.Setenv(key, *tt.val)
				defer os.Unsetenv(key)
			}
			got := parseBoolEnv(key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("parseBoolEnv(%q)=%v (val=%v), want %v", key, got, tt.val, tt.want)
			}
		})
	}
}

func TestParseInt64Env(t *testing.T) {
	const key = "TEST_PARSE_INT64_ENV"
	tests := []struct {
		val        *string
		name       string
		want       int64
		defaultVal int64
	}{
		{nil, "not set uses default", 8192, 8192},
		{strPtr("1024"), "valid value", 1024, 8192},
		{strPtr("0"), "zero", 0, 8192},
		{strPtr("-1"), "negative falls back to default", 8192, 8192},
		{strPtr("abc"), "non-numeric falls back to default", 8192, 8192},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(key)
			if tt.val != nil {
				os.Setenv(key, *tt.val)
				defer os.Unsetenv(key)
			}
			got := parseInt64Env(key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("parseInt64Env(%q)=%v (val=%v), want %v", key, got, tt.val, tt.want)
			}
		})
	}
}

func TestLoad_BodyCapture_Defaults(t *testing.T) {
	os.Unsetenv("LAST9_BODY_CAPTURE_ENABLED")
	os.Unsetenv("LAST9_BODY_CAPTURE_MAX_BYTES")
	os.Unsetenv("LAST9_BODY_CAPTURE_ON_ERROR_ONLY")
	os.Unsetenv("LAST9_BODY_CAPTURE_CONTENT_TYPES")

	cfg := Load()

	if cfg.BodyCaptureEnabled {
		t.Error("BodyCaptureEnabled should default to false")
	}
	if cfg.BodyCaptureMaxBytes != 8192 {
		t.Errorf("BodyCaptureMaxBytes = %d, want 8192", cfg.BodyCaptureMaxBytes)
	}
	if cfg.BodyCaptureOnErrorOnly {
		t.Error("BodyCaptureOnErrorOnly should default to false")
	}
	want := []string{"application/json", "application/xml", "text/plain"}
	if !reflect.DeepEqual(cfg.BodyCaptureContentTypes, want) {
		t.Errorf("BodyCaptureContentTypes = %v, want %v", cfg.BodyCaptureContentTypes, want)
	}
}

func TestLoad_BodyCapture_EnvVars(t *testing.T) {
	os.Setenv("LAST9_BODY_CAPTURE_ENABLED", "true")
	os.Setenv("LAST9_BODY_CAPTURE_MAX_BYTES", "4096")
	os.Setenv("LAST9_BODY_CAPTURE_ON_ERROR_ONLY", "true")
	os.Setenv("LAST9_BODY_CAPTURE_CONTENT_TYPES", "application/json,text/xml")
	defer func() {
		os.Unsetenv("LAST9_BODY_CAPTURE_ENABLED")
		os.Unsetenv("LAST9_BODY_CAPTURE_MAX_BYTES")
		os.Unsetenv("LAST9_BODY_CAPTURE_ON_ERROR_ONLY")
		os.Unsetenv("LAST9_BODY_CAPTURE_CONTENT_TYPES")
	}()

	cfg := Load()

	if !cfg.BodyCaptureEnabled {
		t.Error("BodyCaptureEnabled should be true")
	}
	if cfg.BodyCaptureMaxBytes != 4096 {
		t.Errorf("BodyCaptureMaxBytes = %d, want 4096", cfg.BodyCaptureMaxBytes)
	}
	if !cfg.BodyCaptureOnErrorOnly {
		t.Error("BodyCaptureOnErrorOnly should be true")
	}
	want := []string{"application/json", "text/xml"}
	if !reflect.DeepEqual(cfg.BodyCaptureContentTypes, want) {
		t.Errorf("BodyCaptureContentTypes = %v, want %v", cfg.BodyCaptureContentTypes, want)
	}
}

func strPtr(s string) *string { return &s }
