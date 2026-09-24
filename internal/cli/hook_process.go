package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
)

// This file is the AGENT-AGNOSTIC hook entrypoint (TASK-00031 phase 1, step 5):
// `adb hook process --event <name>` dispatches one lifecycle event onto the
// same HookEngine the Claude-named subcommands use. HookEngine's core is
// generic (typed Process* methods, no agent coupling); only the SUBCOMMAND
// NAMES and the wrapper install paths are Claude-specific, and those move to
// harness adapters in TASK-00032 — the existing `hook pre-tool-use`-style
// commands stay as deprecated aliases so existing worktree wrappers keep
// working.

// hookEventNames are the lifecycle events `adb hook process` dispatches.
var hookEventNames = []string{
	"pre-tool-use", "post-tool-use", "stop", "task-completed", "session-end",
}

// newHookProcessCmd builds `adb hook process --event <name>`.
func newHookProcessCmd() *cobra.Command {
	var event string
	cmd := &cobra.Command{
		Use:   "process",
		Short: "Process one lifecycle event from stdin (agent-agnostic entrypoint)",
		Long: `Process one hook lifecycle event from stdin, dispatching to the shared
HookEngine. Any harness adapter (Claude Code, pi, codex, ...) drives the same
engine through this entrypoint; the Claude-named subcommands (pre-tool-use,
post-tool-use, stop, task-completed, session-end) remain as deprecated
aliases that map onto the same dispatch.

Events:
  pre-tool-use    blocking validation
  post-tool-use   non-blocking actions
  stop            advisory checks
  task-completed  two-phase: blocking quality gates + non-blocking knowledge extraction
  session-end     capture transcript, update context`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if event == "" {
				return fmt.Errorf("--event is required (one of: %s)", strings.Join(hookEventNames, ", "))
			}
			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}
			return dispatchHookEvent(engine, event)
		},
	}
	cmd.Flags().StringVar(&event, "event", "", "event name to process (pre-tool-use, post-tool-use, stop, task-completed, session-end)")
	return cmd
}

// dispatchHookEvent processes one event by name — the single seam every
// harness adapter maps its own lifecycle events onto. Error handling mirrors
// the deprecated Claude-named commands: parse errors are fatal (the harness
// sent a malformed payload); process errors on non-blocking phases warn to
// stderr instead of failing the hook.
func dispatchHookEvent(engine *core.HookEngine, name string) error {
	switch name {
	case "pre-tool-use":
		return processHookEvent(func(e *hooks.PreToolUseEvent) error {
			return engine.ProcessPreToolUse(e)
		})
	case "post-tool-use":
		return processHookEvent(func(e *hooks.PostToolUseEvent) error {
			if err := engine.ProcessPostToolUse(e); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			return nil
		})
	case "stop":
		if err := engine.ProcessStop(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		return nil
	case "task-completed":
		return processHookEvent(func(e *hooks.TaskCompletedEvent) error {
			return engine.ProcessTaskCompleted(e)
		})
	case "session-end":
		return processHookEvent(func(e *hooks.SessionEndEvent) error {
			return engine.ProcessSessionEnd(e)
		})
	default:
		return fmt.Errorf("unknown event %q (one of: %s)", name, strings.Join(hookEventNames, ", "))
	}
}

// processHookEvent parses the event payload from stdin and runs process.
func processHookEvent[T any](process func(*T) error) error {
	event, err := hooks.ParseStdin[T](nil)
	if err != nil {
		return fmt.Errorf("failed to parse event: %w", err)
	}
	return process(event)
}
