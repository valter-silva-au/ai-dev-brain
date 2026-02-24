package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// --- parseSinceDuration unit tests ---

func TestParseSinceDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"empty defaults to 7d", "", false, ""},
		{"whitespace defaults to 7d", "  ", false, ""},
		{"valid 7d", "7d", false, ""},
		{"valid 30d", "30d", false, ""},
		{"valid 24h", "24h", false, ""},
		{"valid 1h", "1h", false, ""},
		{"invalid suffix", "abc", true, "unsupported duration format"},
		{"invalid day number", "xd", true, "invalid day duration"},
		{"invalid hour number", "yh", true, "invalid hour duration"},
		{"negative day is still valid", "-5d", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSinceDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// --- metricsCmd tests ---

type metricsMock struct {
	calcFn func(since time.Time) (*observability.Metrics, error)
}

func (m *metricsMock) Calculate(since time.Time) (*observability.Metrics, error) {
	return m.calcFn(since)
}

func TestMetricsCmd_NilCalculator(t *testing.T) {
	orig := MetricsCalc
	defer func() { MetricsCalc = orig }()
	MetricsCalc = nil

	err := metricsCmd.RunE(metricsCmd, []string{})
	if err == nil {
		t.Fatal("expected error when MetricsCalc is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMetricsCmd_InvalidSinceFormat(t *testing.T) {
	orig := MetricsCalc
	origSince := metricsSince
	defer func() {
		MetricsCalc = orig
		metricsSince = origSince
	}()

	MetricsCalc = &metricsMock{
		calcFn: func(since time.Time) (*observability.Metrics, error) {
			return &observability.Metrics{}, nil
		},
	}

	tests := []struct {
		name   string
		since  string
		errMsg string
	}{
		{"invalid suffix", "abc", "unsupported duration format"},
		{"invalid day number", "xd", "invalid day duration"},
		{"invalid hour number", "yh", "invalid hour duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricsSince = tt.since
			err := metricsCmd.RunE(metricsCmd, []string{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestMetricsCmd_Success_TableFormat(t *testing.T) {
	orig := MetricsCalc
	origSince := metricsSince
	origJSON := metricsJSON
	defer func() {
		MetricsCalc = orig
		metricsSince = origSince
		metricsJSON = origJSON
	}()

	metricsSince = "7d"
	metricsJSON = false

	MetricsCalc = &metricsMock{
		calcFn: func(since time.Time) (*observability.Metrics, error) {
			return &observability.Metrics{
				TasksCreated:   5,
				TasksCompleted: 3,
				EventCount:     42,
				TasksByType:    map[string]int{"feat": 3, "bug": 2},
				TasksByStatus:  map[string]int{"in_progress": 4},
			}, nil
		},
	}

	err := metricsCmd.RunE(metricsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMetricsCmd_Success_JSONFormat(t *testing.T) {
	orig := MetricsCalc
	origSince := metricsSince
	origJSON := metricsJSON
	defer func() {
		MetricsCalc = orig
		metricsSince = origSince
		metricsJSON = origJSON
	}()

	metricsSince = "7d"
	metricsJSON = true

	MetricsCalc = &metricsMock{
		calcFn: func(since time.Time) (*observability.Metrics, error) {
			return &observability.Metrics{
				TasksCreated: 2,
				EventCount:   10,
			}, nil
		},
	}

	err := metricsCmd.RunE(metricsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMetricsCmd_CalculateError(t *testing.T) {
	orig := MetricsCalc
	origSince := metricsSince
	defer func() {
		MetricsCalc = orig
		metricsSince = origSince
	}()

	metricsSince = "7d"

	MetricsCalc = &metricsMock{
		calcFn: func(since time.Time) (*observability.Metrics, error) {
			return nil, fmt.Errorf("event log corrupted")
		},
	}

	err := metricsCmd.RunE(metricsCmd, []string{})
	if err == nil {
		t.Fatal("expected error from Calculate")
	}
	if !strings.Contains(err.Error(), "calculating metrics") {
		t.Errorf("unexpected error: %v", err)
	}
}
