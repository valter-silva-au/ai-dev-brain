package observability

import (
	"testing"
	"time"
)

// TestParseDuration pins adb's ONE extension to Go's duration grammar: a
// trailing "d" means days. time.ParseDuration has no day unit, so "7d" only
// works because of that special case — and the whole point of this function
// living here is that there is exactly one implementation of the rule.
//
// The cases below are deliberately the same shapes internal/cli's --since
// accepts, because internal/cli/metrics.go's parseDuration is intended to
// become a one-line delegate to this function.
func TestParseDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "24h", want: 24 * time.Hour},
		{in: "1d", want: 24 * time.Hour},
		{in: "3d", want: 3 * 24 * time.Hour},
		{in: "5d", want: 5 * 24 * time.Hour},
		{in: "7d", want: 168 * time.Hour},
		{in: "30m", want: 30 * time.Minute},
		{in: "90s", want: 90 * time.Second},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "0d", want: 0},
		{in: "-2d", want: -2 * 24 * time.Hour},
		// A bare "d" is NOT a day count: the special case requires len > 1, so it
		// falls through to time.ParseDuration, which rejects it.
		{in: "d", wantErr: true},
		{in: "", wantErr: true},
		{in: "3 weeks", wantErr: true},
		{in: "xd", wantErr: true},
		{in: "1w", wantErr: true},
		// The day count must be consumed WHOLE. These five all used to return
		// 24h and a nil error, because fmt.Sscanf("%d") reads the leading integer
		// and stops without requiring the rest — so `adb metrics --since 1.5d`
		// silently reported one day. strconv.Atoi rejects the trailing text.
		{in: "1.5d", wantErr: true},
		{in: "1 2d", wantErr: true},
		{in: "1abcd", wantErr: true},
		{in: "1e3d", wantErr: true},
		{in: "1_000d", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDuration(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDuration(%q) = %v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDuration(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseDuration(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
