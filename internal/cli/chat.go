package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewChatCmd builds the `adb chat` subcommand — the Go seam the F4 webview
// chat/steer flow spawns instead of shelling out to `claude` from
// TypeScript. Keeping the LLM invocation on the Go side means:
//
//   - The observability.Chat helper (salvaged in F2) has one caller path.
//   - System-prompt construction (task summary + metrics) stays in Go, so
//     the extension never has to re-implement it.
//   - The reply is streamed back on stdout with a stable UTF-8 shape the
//     extension parses with parseSteerActions.
//
// The command is DELIBERATELY thin: no interactivity, no history, no state.
// One `--message` flag → one `claude -p` shell-out (via observability.Chat)
// → reply on stdout. The webview is what carries the "chat" UX; this is
// only the LLM adapter.
//
// SECURITY POSTURE: this command does NOT act on the reply. It just emits
// the reply text. The steer-action allowlist lives in the extension
// (vscode-extension/src/webview/actions.ts) — Go's job is just to render
// the reply. Anything mutation-shaped in the reply MUST pass through
// actions.toAdbArgv/lower before it can execute.
func NewChatCmd() *cobra.Command {
	var (
		message string
	)
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "One-shot LLM chat with a live workspace context",
		Long: `Run a single ADB-orchestrator turn: builds a system prompt from the
current task list + metrics, appends --message, calls claude -p, and prints
the reply on stdout.

Used by the VS Code webview dashboard's chat panel; --message is the text
the user typed. If claude proposes mutations, they appear inside a fenced
` + "```" + `adb-action` + "```" + ` block — the extension parses them and shows a
per-action modal confirm gate before running anything.

This command NEVER runs the mutations itself. It is a pure LLM adapter.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("--message is required (and non-empty)")
			}
			cctx, err := buildLiveChatContext()
			if err != nil {
				return fmt.Errorf("build chat context: %w", err)
			}

			runner := chatRunnerForTesting
			if runner == nil {
				runner = observability.ExecChatRunner()
			}

			reply, err := observability.Chat(cmd.Context(), runner, cctx, message)
			if err != nil {
				return fmt.Errorf("chat: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), reply)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "User message (required)")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

// chatRunnerForTesting lets tests inject a fake runner so they never
// actually spawn `claude`. Production callers leave this nil, in which
// case NewChatCmd falls back to observability.ExecChatRunner().
//
// The indirection is a package-level var (like `App` above) rather than a
// DI container because the CLI already uses that idiom and this way the
// production code path has zero runtime cost — a single `!= nil` check.
var chatRunnerForTesting observability.ChatRunner

// buildLiveChatContext assembles the ChatContext from the App-scoped
// backlog + metrics. Both are pre-rendered to strings here (not passed as
// live objects into observability.ChatContext) so the observability
// package stays free of dependencies on internal/core / pkg/models.
func buildLiveChatContext() (observability.ChatContext, error) {
	tasks, err := renderTasksSummary()
	if err != nil {
		return observability.ChatContext{}, err
	}
	metrics, err := renderMetricsSummary()
	if err != nil {
		return observability.ChatContext{}, err
	}
	return observability.ChatContext{Tasks: tasks, Metrics: metrics}, nil
}

// renderTasksSummary produces a compact multi-line summary of the current
// backlog. One task per line: "TASK-N [status] title (type/priority)".
// Anything more elaborate belongs in the LLM's own reasoning; we just give
// it the raw shape.
func renderTasksSummary() (string, error) {
	if App == nil || App.BacklogManager == nil {
		return "", fmt.Errorf("backlog manager not initialized")
	}
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		return "", fmt.Errorf("load backlog: %w", err)
	}
	if len(backlog.Tasks) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for i := range backlog.Tasks {
		t := backlog.Tasks[i]
		fmt.Fprintf(&sb, "%s [%s] %s (%s/%s)\n",
			t.ID, t.Status, t.Title, t.Type, t.Priority)
	}
	return sb.String(), nil
}

// renderMetricsSummary produces a compact metrics summary. Pulls from the
// same MetricsCalculator the `adb metrics` CLI uses so the LLM sees the
// same view a human running `adb metrics` sees.
func renderMetricsSummary() (string, error) {
	if App == nil || App.MetricsCalculator == nil {
		return "", fmt.Errorf("metrics calculator not initialized")
	}
	m, err := App.MetricsCalculator.ComputeMetrics()
	if err != nil {
		return "", fmt.Errorf("compute metrics: %w", err)
	}
	return renderMetrics(m), nil
}

// renderMetrics is split out so tests pin the string shape without needing
// a live App/EventLog. Mirrors printMetrics in metrics.go but writes to a
// string builder — keeps the two renderers in sync-by-inspection.
func renderMetrics(m *observability.Metrics) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Tasks Created: %d\n", m.TasksCreated)
	fmt.Fprintf(&sb, "Tasks Completed: %d\n", m.TasksCompleted)
	fmt.Fprintf(&sb, "Agent Sessions: %d\n", m.AgentSessions)
	fmt.Fprintf(&sb, "Worktrees Created: %d, Removed: %d\n",
		m.WorktreesCreated, m.WorktreesRemoved)
	if len(m.TasksByStatus) > 0 {
		sb.WriteString("By status:")
		for status, n := range m.TasksByStatus {
			fmt.Fprintf(&sb, " %s=%d", status, n)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// _ ensures models is used even when the CLI wiring above changes; the
// import lets tests reach through to the backlog-task shape without
// re-typing the struct.
var _ = models.Task{}
