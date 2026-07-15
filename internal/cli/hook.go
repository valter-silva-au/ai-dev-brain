package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
)

// NewHookCmd creates the hook command with all subcommands
func NewHookCmd() *cobra.Command {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Claude Code hook integration",
		Long:  `Commands for managing and processing Claude Code hooks`,
	}

	// Add subcommands
	hookCmd.AddCommand(
		newHookInstallCmd(),
		newHookStatusCmd(),
		newHookPreToolUseCmd(),
		newHookPostToolUseCmd(),
		newHookStopCmd(),
		newHookTaskCompletedCmd(),
		newHookSessionEndCmd(),
	)

	return hookCmd
}

// newHookInstallCmd creates the 'hook install' command
func newHookInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Claude Code hook wrappers",
		Long:  `Deploy hook wrapper scripts to .claude/hooks/ directory`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			hooksDir := filepath.Join(App.BasePath, ".claude", "hooks")
			if err := os.MkdirAll(hooksDir, 0o755); err != nil {
				return fmt.Errorf("failed to create hooks directory: %w", err)
			}

			// Install each hook wrapper
			hooks := map[string]string{
				"adb-hook-pre-tool-use.sh":   preToolUseScript,
				"adb-hook-post-tool-use.sh":  postToolUseScript,
				"adb-hook-stop.sh":           stopScript,
				"adb-hook-task-completed.sh": taskCompletedScript,
				"adb-hook-session-end.sh":    sessionEndScript,
			}

			for filename, content := range hooks {
				hookPath := filepath.Join(hooksDir, filename)
				if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
					return fmt.Errorf("failed to write %s: %w", filename, err)
				}
				fmt.Printf("✓ Installed %s\n", filename)
			}

			fmt.Println("\nHook wrappers installed successfully!")
			fmt.Println("\nTo activate hooks in Claude Code, add to your .claude/settings.json:")
			fmt.Println(`{
  "hooks": {
    "PreToolUse": [
      { "matcher": "*", "hooks": [{ "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/adb-hook-pre-tool-use.sh" }] }
    ],
    "PostToolUse": [
      { "matcher": "*", "hooks": [{ "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/adb-hook-post-tool-use.sh" }] }
    ],
    "Stop": [
      { "hooks": [{ "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/adb-hook-stop.sh" }] }
    ],
    "SessionEnd": [
      { "hooks": [{ "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/adb-hook-session-end.sh" }] }
    ]
  }
}`)
			fmt.Println("\nNote: Claude Code's hook schema is event-keyed (PreToolUse / PostToolUse /")
			fmt.Println("Stop / SessionEnd) with matcher + hooks[] arrays — not a flat name->path map.")
			fmt.Println("There is no Claude Code event for the task-completed wrapper (adb fires")
			fmt.Println("task.completed internally), so it is intentionally omitted above.")
			return nil
		},
	}

	return cmd
}

// newHookStatusCmd creates the 'hook status' command
func newHookStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check hook installation status",
		Long:  `Check which hooks are installed and configured`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			hooksDir := filepath.Join(App.BasePath, ".claude", "hooks")

			hooks := []string{
				"adb-hook-pre-tool-use.sh",
				"adb-hook-post-tool-use.sh",
				"adb-hook-stop.sh",
				"adb-hook-task-completed.sh",
				"adb-hook-session-end.sh",
			}

			fmt.Println("Hook Installation Status:")
			fmt.Println()

			allInstalled := true
			for _, hook := range hooks {
				hookPath := filepath.Join(hooksDir, hook)
				if _, err := os.Stat(hookPath); err == nil {
					fmt.Printf("✓ %s\n", hook)
				} else {
					fmt.Printf("✗ %s (not installed)\n", hook)
					allInstalled = false
				}
			}

			fmt.Println()
			if allInstalled {
				fmt.Println("All hooks are installed!")
			} else {
				fmt.Println("Some hooks are missing. Run 'adb hook install' to install them.")
			}

			return nil
		},
	}

	return cmd
}

// newHookPreToolUseCmd creates the 'hook pre-tool-use' command
func newHookPreToolUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pre-tool-use",
		Short: "Process PreToolUse hook event",
		Long:  `Process PreToolUse hook event from stdin (blocking validation)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}

			event, err := hooks.ParseStdin[hooks.PreToolUseEvent](nil)
			if err != nil {
				return fmt.Errorf("failed to parse event: %w", err)
			}

			if err := engine.ProcessPreToolUse(event); err != nil {
				return err
			}

			return nil
		},
	}

	return cmd
}

// newHookPostToolUseCmd creates the 'hook post-tool-use' command
func newHookPostToolUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post-tool-use",
		Short: "Process PostToolUse hook event",
		Long:  `Process PostToolUse hook event from stdin (non-blocking actions)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}

			event, err := hooks.ParseStdin[hooks.PostToolUseEvent](nil)
			if err != nil {
				return fmt.Errorf("failed to parse event: %w", err)
			}

			if err := engine.ProcessPostToolUse(event); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			return nil
		},
	}

	return cmd
}

// newHookStopCmd creates the 'hook stop' command
func newHookStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Process Stop hook event",
		Long:  `Process Stop hook event (advisory checks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}

			if err := engine.ProcessStop(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			return nil
		},
	}

	return cmd
}

// newHookTaskCompletedCmd creates the 'hook task-completed' command
func newHookTaskCompletedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task-completed",
		Short: "Process TaskCompleted hook event",
		Long:  `Process TaskCompleted hook event (two-phase: blocking quality gates + non-blocking knowledge extraction)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}

			event, err := hooks.ParseStdin[hooks.TaskCompletedEvent](nil)
			if err != nil {
				return fmt.Errorf("failed to parse event: %w", err)
			}

			if err := engine.ProcessTaskCompleted(event); err != nil {
				return err
			}

			return nil
		},
	}

	return cmd
}

// newHookSessionEndCmd creates the 'hook session-end' command
func newHookSessionEndCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-end",
		Short: "Process SessionEnd hook event",
		Long:  `Process SessionEnd hook event (capture transcript, update context)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			engine := core.NewHookEngineWithOptions(App.BasePath, hookOptionsFromConfig())
			if engine.PreventRecursion() {
				return nil
			}

			event, err := hooks.ParseStdin[hooks.SessionEndEvent](nil)
			if err != nil {
				return fmt.Errorf("failed to parse event: %w", err)
			}

			if err := engine.ProcessSessionEnd(event); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			return nil
		},
	}

	return cmd
}

// Shell wrapper scripts
const preToolUseScript = `#!/bin/bash
# Pre-tool-use hook wrapper for ADB
# This wrapper prevents recursion and pipes stdin to 'adb hook pre-tool-use'

if [ "$ADB_HOOK_ACTIVE" = "1" ]; then
  exit 0
fi

export ADB_HOOK_ACTIVE=1
cat | adb hook pre-tool-use
`

const postToolUseScript = `#!/bin/bash
# Post-tool-use hook wrapper for ADB
# This wrapper prevents recursion and pipes stdin to 'adb hook post-tool-use'

if [ "$ADB_HOOK_ACTIVE" = "1" ]; then
  exit 0
fi

export ADB_HOOK_ACTIVE=1
cat | adb hook post-tool-use
`

const stopScript = `#!/bin/bash
# Stop hook wrapper for ADB
# This wrapper prevents recursion and calls 'adb hook stop'

if [ "$ADB_HOOK_ACTIVE" = "1" ]; then
  exit 0
fi

export ADB_HOOK_ACTIVE=1
adb hook stop
`

const taskCompletedScript = `#!/bin/bash
# Task-completed hook wrapper for ADB
# This wrapper prevents recursion and pipes stdin to 'adb hook task-completed'

if [ "$ADB_HOOK_ACTIVE" = "1" ]; then
  exit 0
fi

export ADB_HOOK_ACTIVE=1
cat | adb hook task-completed
`

const sessionEndScript = `#!/bin/bash
# Session-end hook wrapper for ADB
# This wrapper prevents recursion and pipes stdin to 'adb hook session-end'

if [ "$ADB_HOOK_ACTIVE" = "1" ]; then
  exit 0
fi

export ADB_HOOK_ACTIVE=1
cat | adb hook session-end
`
