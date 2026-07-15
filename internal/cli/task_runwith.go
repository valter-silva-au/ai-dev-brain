package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// RunContext carries everything a Commander needs for dispatch. A
// concrete struct (not a context.Context) keeps the exec surface
// explicit and easy to mock.
type RunContext struct {
	Cwd    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Commander is the minimal exec surface the run-with-ruflo command
// needs. Factored behind an interface so tests can inject a fake
// without touching actual processes.
type Commander interface {
	Run(ctx RunContext, name string, args []string) error
}

// execCommander is the production implementation: uses os/exec.
type execCommander struct{}

func (execCommander) Run(ctx RunContext, name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = ctx.Cwd
	cmd.Env = ctx.Env
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// taskRunWithRufloCommander is injectable by tests.
var taskRunWithRufloCommander Commander = execCommander{}

// newTaskRunWithRufloCmd builds `adb task run-with-ruflo`.
//
// The subcommand dispatches an adb-scoped task to ruflo (the Node.js
// agent orchestration layer — see .wiki/decisions/0002-ruflo-dispatch-
// and-vector-memory-in-adb.md on the consumer monorepo). adb remains
// the task boss (scope, gates, knowledge extraction); ruflo is the
// execution muscle. The dispatched process inherits ADB_TASK_ID /
// ADB_TASK_WORKTREE env vars so ruflo-aware agents can call back into
// adb for context.
//
// The ruflo binary name defaults to "claude-flow" (ruflo's real npm
// binary). Users can override with `--ruflo-bin` or by setting
// ADB_RUFLO_BIN in their environment.
func newTaskRunWithRufloCmd() *cobra.Command {
	var rufloBin string

	cmd := &cobra.Command{
		Use:                "run-with-ruflo <task-id> [-- <ruflo-args>]",
		Short:              "Dispatch an adb task to ruflo (the Node.js agent orchestration layer)",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: false,
		Long: `Dispatch an adb task to ruflo. Resolves the task's worktree, sets
ADB_TASK_ID and ADB_TASK_WORKTREE env vars, and execs the ruflo binary
there. Everything after ` + "`--`" + ` is forwarded as-is to ruflo.

Example:
  adb task run-with-ruflo TASK-00042 -- swarm init --topology mesh

Prerequisites (not checked — failure surfaces from exec):
  - ruflo installed and reachable as the configured binary (default
    "claude-flow"); try ` + "`npm install -g claude-flow`" + `.
  - The task's worktree must exist (run ` + "`adb task create --repo=...`" + `
    first).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialised")
			}
			taskID := args[0]
			rufloArgs := args[1:]
			return runTaskWithRuflo(cmd, taskID, rufloBin, rufloArgs)
		},
	}

	cmd.Flags().StringVar(&rufloBin, "ruflo-bin", "", "override the ruflo binary (default: claude-flow; also reads $ADB_RUFLO_BIN)")
	return cmd
}

// runTaskWithRuflo is separated from the Cobra closure so tests can
// call it directly with a controlled Commander.
func runTaskWithRuflo(cmd *cobra.Command, taskID, rufloBin string, rufloArgs []string) error {
	if taskID == "" {
		return fmt.Errorf("task-id is required")
	}

	// Binary resolution: flag > env > default.
	if rufloBin == "" {
		rufloBin = os.Getenv("ADB_RUFLO_BIN")
	}
	if rufloBin == "" {
		rufloBin = "claude-flow"
	}

	// Resolve the worktree. The task must have one — dispatch without
	// a worktree would run ruflo in an undefined cwd which is much
	// worse than failing loudly.
	worktree, err := resolveTaskWorktree(taskID)
	if err != nil {
		return fmt.Errorf("resolve worktree for %s: %w", taskID, err)
	}

	// Build env: pass through parent's env, then layer adb-specific
	// context on top. Downstream ruflo-aware agents can call back into
	// adb using these hints.
	env := append(os.Environ(),
		"ADB_TASK_ID="+taskID,
		"ADB_TASK_WORKTREE="+worktree,
	)

	emitDispatchEvent("agent.session_started", taskID, worktree, rufloBin, rufloArgs, nil)

	runCtx := RunContext{
		Cwd:    worktree,
		Env:    env,
		Stdin:  cmd.InOrStdin(),
		Stdout: cmd.OutOrStdout(),
		Stderr: cmd.ErrOrStderr(),
	}
	runErr := taskRunWithRufloCommander.Run(runCtx, rufloBin, rufloArgs)

	emitDispatchEvent("agent.session_ended", taskID, worktree, rufloBin, rufloArgs, runErr)

	return runErr
}

// resolveTaskWorktree looks up the worktree path for a task id via the
// worktree manager. Returns a usable error if none exists.
func resolveTaskWorktree(taskID string) (string, error) {
	if App == nil || App.GitWorktreeManager == nil {
		return "", fmt.Errorf("worktree manager not available")
	}
	path, exists, err := App.GitWorktreeManager.GetWorktreeForTask(taskID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no worktree found for task %s — run `adb task create --repo=...` first", taskID)
	}
	return path, nil
}

// emitDispatchEvent logs an observability event to the adb event log
// (.events.jsonl). Failures are swallowed because observability must
// never break the user-facing operation.
func emitDispatchEvent(evtType, taskID, worktree, bin string, args []string, runErr error) {
	if App == nil || App.EventLog == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":  taskID,
		"worktree": worktree,
		"bin":      bin,
		"args":     args,
	}
	if runErr != nil {
		data["error"] = runErr.Error()
	}
	App.EventLog.Log(observability.EventType(evtType), data)
}
