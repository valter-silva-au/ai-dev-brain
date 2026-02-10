package integration

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CLIAlias maps a short alias name to a full CLI command with optional default arguments.
type CLIAlias struct {
	Name        string   `yaml:"name"`
	Command     string   `yaml:"command"`
	DefaultArgs []string `yaml:"default_args,omitempty"`
}

// CLIExecConfig holds all parameters needed to execute an external CLI tool.
type CLIExecConfig struct {
	CLI     string
	Args    []string
	TaskCtx *TaskEnvContext // nil if no active task
	Aliases []CLIAlias
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// TaskEnvContext carries task-specific information to inject as environment variables.
type TaskEnvContext struct {
	TaskID       string
	Branch       string
	WorktreePath string
	TicketPath   string
}

// CLIExecResult captures the outcome of an external CLI invocation.
type CLIExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CLIExecutor defines the interface for invoking external CLI tools with
// alias resolution and task context injection.
type CLIExecutor interface {
	// Exec invokes an external CLI, resolving aliases and injecting task env vars.
	Exec(config CLIExecConfig) (*CLIExecResult, error)
	// ResolveAlias returns the expanded command and args for an alias, or the original if not aliased.
	ResolveAlias(name string, aliases []CLIAlias) (command string, defaultArgs []string, found bool)
	// BuildEnv constructs the subprocess environment with task context variables injected.
	BuildEnv(base []string, taskCtx *TaskEnvContext) []string
	// ListAliases returns all configured CLI aliases as formatted strings.
	ListAliases(aliases []CLIAlias) []string
	// LogFailure records a CLI failure in the task's context if a task is active.
	LogFailure(taskCtx *TaskEnvContext, cli string, args []string, result *CLIExecResult) error
}

// cliExecutor implements CLIExecutor.
type cliExecutor struct{}

// NewCLIExecutor creates a new CLIExecutor.
func NewCLIExecutor() CLIExecutor {
	return &cliExecutor{}
}

// ResolveAlias scans the aliases list for a matching name. If found, it returns
// the expanded command and default args. If not found, it returns the original
// name with nil default args.
func (e *cliExecutor) ResolveAlias(name string, aliases []CLIAlias) (string, []string, bool) {
	for _, a := range aliases {
		if a.Name == name {
			return a.Command, a.DefaultArgs, true
		}
	}
	return name, nil, false
}

// BuildEnv appends ADB_* environment variables to the base environment when
// a task context is provided. When taskCtx is nil, the base is returned unchanged.
func (e *cliExecutor) BuildEnv(base []string, taskCtx *TaskEnvContext) []string {
	if taskCtx == nil {
		return base
	}
	env := make([]string, len(base), len(base)+4)
	copy(env, base)
	env = append(env,
		"ADB_TASK_ID="+taskCtx.TaskID,
		"ADB_BRANCH="+taskCtx.Branch,
		"ADB_WORKTREE_PATH="+taskCtx.WorktreePath,
		"ADB_TICKET_PATH="+taskCtx.TicketPath,
	)
	return env
}

// containsPipe returns true if any argument is the pipe character "|".
func containsPipe(args []string) bool {
	for _, a := range args {
		if a == "|" {
			return true
		}
	}
	return false
}

// Exec resolves aliases, builds the environment, and runs the external CLI.
// If the arguments contain a pipe character, the full command is delegated to
// the system shell (sh -c on Linux/Mac, cmd /c on Windows).
func (e *cliExecutor) Exec(config CLIExecConfig) (*CLIExecResult, error) {
	command, defaultArgs, _ := e.ResolveAlias(config.CLI, config.Aliases)

	// Build the full argument list: default_args + user args.
	fullArgs := make([]string, 0, len(defaultArgs)+len(config.Args))
	fullArgs = append(fullArgs, defaultArgs...)
	fullArgs = append(fullArgs, config.Args...)

	var cmd *exec.Cmd

	if containsPipe(fullArgs) {
		// Delegate to system shell for pipe support.
		parts := append([]string{command}, fullArgs...)
		cmdLine := strings.Join(parts, " ")
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/c", cmdLine)
		} else {
			cmd = exec.Command("sh", "-c", cmdLine)
		}
	} else {
		cmd = exec.Command(command, fullArgs...)
	}

	// Build environment with task context injection.
	cmd.Env = e.BuildEnv(os.Environ(), config.TaskCtx)

	// Set up I/O. We always capture stdout/stderr for the result,
	// but also tee to the provided writers if set.
	var stdoutBuf, stderrBuf bytes.Buffer

	if config.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdoutBuf, config.Stdout)
	} else {
		cmd.Stdout = &stdoutBuf
	}

	if config.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderrBuf, config.Stderr)
	} else {
		cmd.Stderr = &stderrBuf
	}

	if config.Stdin != nil {
		cmd.Stdin = config.Stdin
	}

	err := cmd.Run()

	result := &CLIExecResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			// Command could not be started (e.g., not found).
			return result, fmt.Errorf("executing %s: %w", command, err)
		}
	}

	// Log failure if exit code is non-zero and a task context is active.
	if result.ExitCode != 0 && config.TaskCtx != nil {
		if logErr := e.LogFailure(config.TaskCtx, command, fullArgs, result); logErr != nil {
			// Log warning but do not fail the CLI execution itself.
			fmt.Fprintf(os.Stderr, "warning: failed to log CLI failure to context: %v\n", logErr)
		}
	}

	return result, nil
}

// ListAliases returns formatted strings describing each configured alias.
func (e *cliExecutor) ListAliases(aliases []CLIAlias) []string {
	result := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if len(a.DefaultArgs) > 0 {
			result = append(result, fmt.Sprintf("%s -> %s [%s]", a.Name, a.Command, strings.Join(a.DefaultArgs, " ")))
		} else {
			result = append(result, fmt.Sprintf("%s -> %s", a.Name, a.Command))
		}
	}
	return result
}

// LogFailure appends a failure entry to the task's context.md file.
func (e *cliExecutor) LogFailure(taskCtx *TaskEnvContext, cli string, args []string, result *CLIExecResult) error {
	if taskCtx == nil {
		return nil
	}

	contextPath := filepath.Join(taskCtx.TicketPath, "context.md")

	entry := fmt.Sprintf("\n\n## CLI Failure\n\n- **Command:** `%s %s`\n- **Exit Code:** %d\n- **Stderr:**\n```\n%s\n```\n",
		cli, strings.Join(args, " "), result.ExitCode, result.Stderr)

	f, err := os.OpenFile(contextPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening context file %s: %w", contextPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("writing to context file %s: %w", contextPath, err)
	}

	return nil
}
