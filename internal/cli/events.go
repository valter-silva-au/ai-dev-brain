package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// NewEventsCmd builds the `adb events` subcommand tree.
//
// Two verbs, mirrored to the JSONL pipeline the F3 webview reads from:
//
//	adb events query [--type=T] [--task=ID] [--since=DUR] [--json]
//	adb events tail  [--follow] [--json]
//
// Both read from <ADB_HOME>/.adb/events.jsonl (see internal/app.go). Filters are
// applied in Go, so the CLI is a thin renderer over EventLog.ReadAll /
// ReadSince — the file format itself is stable JSONL and the extension can
// spawn `adb events tail --follow --json` and pipe stdout directly into the
// webview.
func NewEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Inspect the structured event log (.adb/events.jsonl)",
		Long: `Query or tail the append-only JSONL event stream adb writes to
<ADB_HOME>/.adb/events.jsonl (task lifecycle, worktree ops, agent sessions,
issue-sync decisions).

Every event is one of the types documented in internal/observability/schema.go
(KnownEventTypes / IsKnownEventType). Consumers — metrics, alerts, the VS Code
webview — should use that set as the allowlist rather than hard-coding strings.`,
	}
	cmd.AddCommand(newEventsQueryCmd(), newEventsTailCmd(), newEventsDigestCmd())
	return cmd
}

// newEventsDigestCmd builds `adb events digest` — the same-machine live digest
// (ADR part D / T4). It reduces the agent.session_* events to one line per
// OTHER active task_id, e.g. "- TASK-00081 editing (12m ago)", capped so the
// Tier-0 worktree context stays token-cheap. This is what the KB
// ticket-bootstrap skill (T6) shells out to.
func newEventsDigestCmd() *cobra.Command {
	var (
		since   string
		self    string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Compact live digest of what other same-machine sessions are doing",
		Long: `Reduce the agent.session_* event stream to one line per OTHER active
task_id (excluding --self), within the --since window, newest first, capped to
keep it token-cheap. Human output is lines like:

  - TASK-00081 editing (12m ago)

--json emits the structured lines (task_id, activity, age, at) for the KB
ticket-bootstrap skill to compose Tier-0 worktree context.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if App == nil || App.EventLog == nil {
				return fmt.Errorf("app not initialized")
			}
			opts := observability.SessionDigestOptions{Self: self}
			if since != "" {
				d, err := parseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				opts.Since = d
			}
			digest, err := observability.BuildSessionDigest(App.EventLog, opts)
			if err != nil {
				return fmt.Errorf("build session digest: %w", err)
			}
			return writeDigest(cmd.OutOrStdout(), digest, jsonOut)
		},
	}
	cmd.Flags().StringVar(&since, "since", "8h", "Look-back window (e.g. 8h, 30m, 2d)")
	cmd.Flags().StringVar(&self, "self", "", "task_id of the current session, excluded from the digest")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit the structured digest lines as a JSON array")
	return cmd
}

// writeDigest renders a SessionDigest: --json emits the structured lines as a
// JSON array (normalized to [] when empty); human mode emits one Render()ed
// line per entry, and a friendly note when there are no other sessions.
func writeDigest(w interface{ Write([]byte) (int, error) }, d observability.SessionDigest, jsonOut bool) error {
	if jsonOut {
		lines := d.Lines
		if lines == nil {
			lines = []observability.SessionLine{}
		}
		b, err := json.MarshalIndent(lines, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal digest: %w", err)
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
		return nil
	}
	if len(d.Lines) == 0 {
		_, err := w.Write([]byte("(no other active sessions)\n"))
		return err
	}
	for _, l := range d.Lines {
		if _, err := w.Write([]byte(l.Render() + "\n")); err != nil {
			return err
		}
	}
	return nil
}

func newEventsQueryCmd() *cobra.Command {
	var (
		typeFilter string
		taskFilter string
		since      string
		jsonOut    bool
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query past events with filters",
		Long: `Filter the event log by type, task_id, and/or a since-window.
Human output is one line per event; --json emits a JSON array (parseable
by any consumer, including the webview overview reload path).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if App == nil || App.EventLog == nil {
				return fmt.Errorf("app not initialized")
			}
			events, err := App.EventLog.ReadAll()
			if err != nil {
				return fmt.Errorf("read event log: %w", err)
			}

			var cutoff time.Time
			if since != "" {
				d, err := parseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				cutoff = time.Now().UTC().Add(-d)
			}

			filtered := filterEvents(events, typeFilter, taskFilter, cutoff)

			if jsonOut {
				return writeJSONArray(cmd.OutOrStdout(), filtered)
			}
			return writeHumanEvents(cmd.OutOrStdout(), filtered)
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by event type (e.g. task.created)")
	cmd.Flags().StringVar(&taskFilter, "task", "", "Filter by data.task_id")
	cmd.Flags().StringVar(&since, "since", "", "Only events within this window (e.g. 24h, 7d)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as a JSON array")
	return cmd
}

func newEventsTailCmd() *cobra.Command {
	var (
		follow  bool
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream events (newest last). --follow keeps streaming new lines.",
		Long: `Print the current tail of the event log, then optionally block and
stream any new events as they are appended. --json emits one JSON object
per line (JSONL) — the exact shape the F3 webview parses.

Without --follow, this is a one-shot dump (like ReadAll). With --follow, the
process polls the log every ~500ms and writes only newly-appended events.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if App == nil || App.EventLog == nil {
				return fmt.Errorf("app not initialized")
			}
			return runEventsTail(cmd, App.EventLog, follow, jsonOut)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Keep streaming as new events arrive")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit one JSON object per line (JSONL) for the extension")
	return cmd
}

// filterEvents applies (type, task_id, cutoff) filters. All filters are
// AND-combined; empty filters are no-ops. Kept as a pure function so it is
// unit-testable without cobra plumbing.
func filterEvents(events []observability.Event, typeFilter, taskFilter string, cutoff time.Time) []observability.Event {
	var out []observability.Event
	for _, e := range events {
		if typeFilter != "" && string(e.Type) != typeFilter {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		if taskFilter != "" {
			id, _ := e.Data["task_id"].(string)
			if id != taskFilter {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// writeJSONArray serializes events as a JSON array (for `--json` on query).
// A nil slice becomes `null` under encoding/json — normalize to `[]` so
// callers always get an array.
func writeJSONArray(w interface{ Write([]byte) (int, error) }, events []observability.Event) error {
	if events == nil {
		events = []observability.Event{}
	}
	b, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// writeHumanEvents renders one event per line: RFC3339 timestamp, type, and
// the raw data map. Compact enough to pipe into `less` or a grep.
func writeHumanEvents(w interface{ Write([]byte) (int, error) }, events []observability.Event) error {
	for _, e := range events {
		line := fmt.Sprintf("%s  %-24s  %v\n",
			e.Timestamp.Format(time.RFC3339), e.Type, e.Data)
		if _, err := w.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

// writeEventJSONL writes one event as a single JSONL line. Used by tail
// (both once and follow modes) to produce the extension-friendly shape.
func writeEventJSONL(w interface{ Write([]byte) (int, error) }, e observability.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// runEventsTail is the actual tail loop. Non-follow: one dump of the current
// contents in the caller's chosen shape. Follow: prints the current tail then
// polls ReadAll, emitting only events past the last-seen count. Poll cadence
// is 500ms — short enough to feel live in a webview, long enough not to burn
// CPU on a mostly-quiet workspace.
func runEventsTail(cmd *cobra.Command, log *observability.EventLog, follow, jsonOut bool) error {
	events, err := log.ReadAll()
	if err != nil {
		return fmt.Errorf("read event log: %w", err)
	}
	if err := renderTail(cmd, events, jsonOut); err != nil {
		return err
	}
	if !follow {
		return nil
	}

	seen := len(events)
	// Signal we're following on stderr — makes the CLI's intent obvious to
	// a human piping through `head`, without polluting the JSONL stdout.
	fmt.Fprintln(os.Stderr, "streaming events (Ctrl-C to stop)...")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	ctx := cmd.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			all, err := log.ReadAll()
			if err != nil {
				return fmt.Errorf("read event log: %w", err)
			}
			if len(all) > seen {
				if err := renderTail(cmd, all[seen:], jsonOut); err != nil {
					return err
				}
				seen = len(all)
			}
		}
	}
}

// renderTail writes events either as JSONL (for --json) or human-readable
// (default). Split out so the follow loop reuses the same formatter.
func renderTail(cmd *cobra.Command, events []observability.Event, jsonOut bool) error {
	if jsonOut {
		for _, e := range events {
			if err := writeEventJSONL(cmd.OutOrStdout(), e); err != nil {
				return err
			}
		}
		return nil
	}
	return writeHumanEvents(cmd.OutOrStdout(), events)
}
