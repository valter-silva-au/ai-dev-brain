package observability

import (
	"fmt"
	"strconv"
	"time"
)

// ParseDuration parses a duration string with adb's one extension to Go's
// grammar: a trailing "d" means DAYS ("7d" = 168h). time.ParseDuration has no
// day unit, so that spelling only works because of the special case below;
// everything else falls through to the standard parser ("24h", "90m", "1h30m").
//
// This is the canonical implementation of that rule. It lives in observability
// because this is the lowest package that owns the durations users type at adb —
// the event/metrics window (`--since`) and the alert thresholds — and because
// internal/cli already imports this package, so the CLI's own unexported
// parseDuration (internal/cli/metrics.go, shared by `adb metrics --since`,
// `adb events query --since` and `adb task timeline --since`) can become a
// one-line delegate to it. Two spellings of one duration rule is exactly the
// drift this repo has been removing; do not add a third.
func ParseDuration(s string) (time.Duration, error) {
	// Support days (d) format.
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]

		// strconv.Atoi, NOT fmt.Sscanf. Sscanf does not require full consumption:
		// with "%d" it reads the leading integer and stops, reporting n=1 and a nil
		// error, so "1.5d" silently parsed as 1 day (and "1 2d", "1abcd", "1e3d"
		// likewise) — user input truncated with no diagnostic. Atoi rejects any
		// trailing text, which is what makes "1.5d" an error rather than a lie.
		d, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day format: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// Use standard time.ParseDuration for other formats.
	return time.ParseDuration(s)
}
