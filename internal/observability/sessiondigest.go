package observability

import (
	"fmt"
	"sort"
	"time"
)

// DefaultSessionDigestSince is the look-back window used when a caller does
// not specify one. Sessions older than this are not "live" for digest purposes.
const DefaultSessionDigestSince = 8 * time.Hour

// DefaultSessionDigestCap caps the number of lines a digest emits so the
// Tier-0 worktree context (ADR part D) stays token-cheap.
const DefaultSessionDigestCap = 5

// sessionEventTypes are the agent.session_* events the digest is built from.
// A session's most-recent one of these shapes its digest line.
var sessionEventTypes = map[EventType]bool{
	EventAgentSessionStarted: true,
	EventAgentSessionActive:  true,
	EventAgentSessionEnded:   true,
}

// SessionDigestOptions controls BuildSessionDigest.
type SessionDigestOptions struct {
	// Self is the task_id of the current session, excluded from the digest so
	// a session never reports itself. Empty means "exclude nothing".
	Self string
	// Since is the look-back window; events older than now-Since are ignored.
	// Zero uses DefaultSessionDigestSince.
	Since time.Duration
	// Cap is the maximum number of lines returned. Zero uses
	// DefaultSessionDigestCap. Negative means unlimited.
	Cap int
	// Now overrides the reference time (for deterministic tests). Zero uses
	// time.Now().UTC().
	Now time.Time
}

// SessionLine is one other session's compact state.
type SessionLine struct {
	TaskID   string    `json:"task_id"`
	Activity string    `json:"activity"` // human verb, e.g. "editing"; may be empty
	Age      string    `json:"age"`      // pre-rendered, e.g. "12m"
	At       time.Time `json:"at"`       // the underlying event timestamp
}

// Render turns a line into the compact human form used in Tier-0 context, e.g.
// "- TASK-00081 editing (12m ago)". When activity is empty it degrades to
// "- TASK-00081 active (12m ago)".
func (l SessionLine) Render() string {
	activity := l.Activity
	if activity == "" {
		activity = "active"
	}
	return fmt.Sprintf("- %s %s (%s ago)", l.TaskID, activity, l.Age)
}

// SessionDigest is the reduced view of what other same-machine sessions are
// doing, newest first.
type SessionDigest struct {
	Lines []SessionLine `json:"lines"`
}

// Render returns the digest as a newline-joined block of Render()ed lines.
// Empty digest returns "" (callers decide how to phrase "no other sessions").
func (d SessionDigest) Render() string {
	out := ""
	for i, l := range d.Lines {
		if i > 0 {
			out += "\n"
		}
		out += l.Render()
	}
	return out
}

// BuildSessionDigest reads the event log via ReadSince and reduces it to the
// latest agent.session_* event per OTHER task_id, within the since-window,
// capped, newest-first. It does not add any new storage format — it is a pure
// read over the existing JSONL log.
func BuildSessionDigest(log *EventLog, opts SessionDigestOptions) (SessionDigest, error) {
	if log == nil {
		return SessionDigest{}, fmt.Errorf("nil event log")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	since := opts.Since
	if since == 0 {
		since = DefaultSessionDigestSince
	}
	capN := opts.Cap
	if capN == 0 {
		capN = DefaultSessionDigestCap
	}

	cutoff := now.Add(-since)
	events, err := log.ReadSince(cutoff)
	if err != nil {
		return SessionDigest{}, fmt.Errorf("read events since %s: %w", cutoff.Format(time.RFC3339), err)
	}

	// Keep the latest session event per task_id (other than self).
	latest := make(map[string]Event)
	for _, e := range events {
		if !sessionEventTypes[e.Type] {
			continue
		}
		id, _ := e.Data["task_id"].(string)
		if id == "" || id == opts.Self {
			continue
		}
		if prev, ok := latest[id]; !ok || e.Timestamp.After(prev.Timestamp) {
			latest[id] = e
		}
	}

	lines := make([]SessionLine, 0, len(latest))
	for id, e := range latest {
		activity, _ := e.Data["activity"].(string)
		lines = append(lines, SessionLine{
			TaskID:   id,
			Activity: activity,
			Age:      humanizeAge(now.Sub(e.Timestamp)),
			At:       e.Timestamp,
		})
	}

	// Newest first; tie-break by task_id for determinism.
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].At.Equal(lines[j].At) {
			return lines[i].TaskID < lines[j].TaskID
		}
		return lines[i].At.After(lines[j].At)
	})

	if capN >= 0 && len(lines) > capN {
		lines = lines[:capN]
	}

	return SessionDigest{Lines: lines}, nil
}

// humanizeAge renders a duration as a compact age token: "3s", "12m", "4h",
// "2d". Negative/zero durations render as "0s". This is deliberately coarse —
// the digest is a glance, not a stopwatch.
func humanizeAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
