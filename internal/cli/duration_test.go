package cli

import (
	"testing"
	"time"
)

// TestParseDuration_HonoursTheDayRule pins the CLI side of adb's one extension to
// Go's duration grammar: a trailing "d" means DAYS. time.ParseDuration has no day
// unit, so "7d" only parses because of the special case — and three commands take
// a user-typed --since through this function (`adb metrics`, `adb events query`,
// `adb task timeline`).
//
// It had NO test on the CLI side before TASK-00039 Batch 4, which is how the rule
// came to exist twice: an inline copy here and, once alert thresholds became
// configurable, a second one in internal/observability. This function is now a
// one-line delegate to observability.ParseDuration, and this test is what makes
// that delegation load-bearing rather than incidental — it fails if anyone
// re-inlines a divergent copy.
func TestParseDuration_HonoursTheDayRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		// The extension. These are the spellings the docs advertise.
		{"one day", "1d", 24 * time.Hour},
		{"a week, the L200 example", "7d", 168 * time.Hour},
		{"thirty days", "30d", 720 * time.Hour},

		// Standard Go units must keep falling through untouched.
		{"hours", "24h", 24 * time.Hour},
		{"minutes", "90m", 90 * time.Minute},
		{"seconds", "45s", 45 * time.Second},
		{"compound", "1h30m", 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDuration(tt.input)
			if err != nil {
				t.Fatalf("parseDuration(%q) returned an unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseDuration_RejectsAMalformedDay guards the error path. A bare "d" is
// deliberately NOT a day: the day branch requires at least one leading character,
// so "d" falls through to time.ParseDuration and is rejected there instead.
func TestParseDuration_RejectsAMalformedDay(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"xd", "1.5d", "d", "", "7 days", "7D"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if _, err := parseDuration(input); err == nil {
				t.Errorf("parseDuration(%q) = nil error, want a rejection", input)
			}
		})
	}
}
