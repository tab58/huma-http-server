package middleware

import (
	"testing"
	"time"
)

func TestApplyWideEventDefaults(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := applyWideEventDefaults(WideEventConfig{})
		// SampleRate has no zero-default: 0 must disable success sampling
		if cfg.SampleRate != 0 {
			t.Errorf("SampleRate = %v, want 0 (disabled)", cfg.SampleRate)
		}
		if cfg.SlowThreshold != DEFAULT_SLOW_THRESHOLD {
			t.Errorf("SlowThreshold = %v, want %v", cfg.SlowThreshold, DEFAULT_SLOW_THRESHOLD)
		}
		if cfg.Logger == nil {
			t.Error("Logger should default to slog.Default(), got nil")
		}
	})

	t.Run("explicit values preserved", func(t *testing.T) {
		cfg := applyWideEventDefaults(WideEventConfig{SampleRate: 0.5, SlowThreshold: 10 * time.Second})
		if cfg.SampleRate != 0.5 || cfg.SlowThreshold != 10*time.Second {
			t.Errorf("explicit values overwritten: %+v", cfg)
		}
	})
}

func TestShouldSample(t *testing.T) {
	fast := 1 * time.Millisecond
	slow := 5 * time.Second
	base := WideEventConfig{SampleRate: 0, SlowThreshold: 2 * time.Second}

	tests := []struct {
		name     string
		cfg      WideEventConfig
		event    *WideEventContext
		expected bool
	}{
		{"errors always kept", base, &WideEventContext{StatusCode: 500, Duration: fast}, true},
		{"slow requests always kept", base, &WideEventContext{StatusCode: 200, Duration: slow}, true},
		{"fast success with rate 0 dropped", base, &WideEventContext{StatusCode: 200, Duration: fast}, false},
		{
			"rate 1 always kept",
			WideEventConfig{SampleRate: 1, SlowThreshold: 2 * time.Second},
			&WideEventContext{StatusCode: 200, Duration: fast},
			true,
		},
		{
			"custom SampleFn kept",
			WideEventConfig{SlowThreshold: 2 * time.Second, SampleFn: func(e *WideEventContext) bool { return e.Path == "/keep" }},
			&WideEventContext{StatusCode: 200, Duration: fast, Path: "/keep"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSample(tt.cfg, tt.event); got != tt.expected {
				t.Errorf("shouldSample = %v, want %v", got, tt.expected)
			}
		})
	}
}
