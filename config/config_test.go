package config

import (
	"os"
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
