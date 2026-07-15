package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// CLIExecutor executes CLI commands with alias resolution and environment injection
type CLIExecutor interface {
	// Execute runs a command with alias resolution and env injection
	// Returns stdout, stderr, and error
	Execute(command string, workDir string) (string, string, error)

	// ExecuteWithWriter runs a command and captures output to both stdout/stderr and a writer
	ExecuteWithWriter(command string, workDir string, writer io.Writer) (string, string, error)
}

// TaskEnv holds environment variables for a task
type TaskEnv struct {
	TaskID       string
	Branch       string
	WorktreePath string
	TicketPath   string
}

// DefaultCLIExecutor implements CLIExecutor
type DefaultCLIExecutor struct {
	aliases     map[string]string // alias -> command mapping
	taskEnv     TaskEnv
	contextFile string // path to context.md for failure logging
}

// NewCLIExecutor creates a new CLI executor
// aliases: map of command aliases
// taskEnv: task environment variables to inject
// contextFile: path to context.md for logging failures
func NewCLIExecutor(aliases map[string]string, taskEnv TaskEnv, contextFile string) CLIExecutor {
	if aliases == nil {
		aliases = make(map[string]string)
	}
	return &DefaultCLIExecutor{
		aliases:     aliases,
		taskEnv:     taskEnv,
		contextFile: contextFile,
	}
}

// Execute runs a command with alias resolution and env injection
func (e *DefaultCLIExecutor) Execute(command string, workDir string) (string, string, error) {
	return e.ExecuteWithWriter(command, workDir, nil)
}

// dangerousPatterns detects command injection attempts in individual arguments.
// These patterns indicate shell metacharacters that should not appear in
// simple command arguments (they are fine in explicitly shell-delegated commands).
var dangerousPatterns = regexp.MustCompile(`[;` + "`" + `$\(\)]`)

// ValidateCommandArgs checks that individual command arguments don't contain
// shell injection metacharacters. Returns an error if dangerous patterns are found.
func ValidateCommandArgs(args []string) error {
	for _, arg := range args {
		if dangerousPatterns.MatchString(arg) {
			return fmt.Errorf("argument contains potentially dangerous shell characters: %q", arg)
		}
	}
	return nil
}

// ExecuteWithWriter runs a command and captures output to both stdout/stderr and a writer
func (e *DefaultCLIExecutor) ExecuteWithWriter(command string, workDir string, writer io.Writer) (string, string, error) {
	if command == "" {
		return "", "", fmt.Errorf("command cannot be empty")
	}

	// Resolve alias
	resolvedCmd := e.resolveAlias(command)

	// Check if command needs shell execution
	// Delegate to shell if command contains:
	// - pipes (|)
	// - shell quotes (' or ")
	// - redirects (> or <)
	// - background (&)
	// - or starts with sh/bash -c
	needsShell := strings.Contains(resolvedCmd, "|") ||
		strings.Contains(resolvedCmd, "'") ||
		strings.Contains(resolvedCmd, "\"") ||
		strings.Contains(resolvedCmd, ">") ||
		strings.Contains(resolvedCmd, "<") ||
		strings.Contains(resolvedCmd, "&") ||
		strings.HasPrefix(resolvedCmd, "sh -c") ||
		strings.HasPrefix(resolvedCmd, "bash -c")

	var cmd *exec.Cmd
	if needsShell {
		// When delegating to shell, validate that the command doesn't contain
		// injection attempts via semicolons, backticks, or subshell operators
		// that weren't part of the original alias/command structure.
		// Note: pipes, quotes, and redirects are expected in shell commands.
		if dangerousPatterns.MatchString(resolvedCmd) {
			return "", "", fmt.Errorf("command contains potentially dangerous shell metacharacters (;, `, $(), etc.): rejected for safety")
		}
		cmd = exec.Command("sh", "-c", resolvedCmd)
	} else {
		// Split command into parts
		parts := strings.Fields(resolvedCmd)
		if len(parts) == 0 {
			return "", "", fmt.Errorf("empty command after parsing")
		}
		// Validate arguments don't contain injection characters
		if err := ValidateCommandArgs(parts[1:]); err != nil {
			return "", "", fmt.Errorf("command argument validation failed: %w", err)
		}
		cmd = exec.Command(parts[0], parts[1:]...)
	}

	// Set working directory
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Inject task environment variables
	cmd.Env = e.injectEnv(os.Environ())

	// Setup output capture
	var stdoutBuf, stderrBuf strings.Builder

	if writer != nil {
		// Use MultiWriter to write to both buffer and provided writer
		cmd.Stdout = io.MultiWriter(&stdoutBuf, writer)
		cmd.Stderr = io.MultiWriter(&stderrBuf, writer)
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	// Execute command
	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	// Log failure to context.md if error occurred
	if err != nil {
		e.logFailure(command, stdout, stderr, err)
	}

	return stdout, stderr, err
}

// resolveAlias resolves command aliases
func (e *DefaultCLIExecutor) resolveAlias(command string) string {
	// Split command to get the first word
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return command
	}

	// Check if first word is an alias
	if aliasCmd, exists := e.aliases[parts[0]]; exists {
		// Replace alias with actual command
		parts[0] = aliasCmd
		return strings.Join(parts, " ")
	}

	return command
}

// injectEnv injects task environment variables into the environment
func (e *DefaultCLIExecutor) injectEnv(baseEnv []string) []string {
	env := make([]string, len(baseEnv))
	copy(env, baseEnv)

	// Add task environment variables
	if e.taskEnv.TaskID != "" {
		env = append(env, fmt.Sprintf("ADB_TASK_ID=%s", e.taskEnv.TaskID))
	}
	if e.taskEnv.Branch != "" {
		env = append(env, fmt.Sprintf("ADB_BRANCH=%s", e.taskEnv.Branch))
	}
	if e.taskEnv.WorktreePath != "" {
		env = append(env, fmt.Sprintf("ADB_WORKTREE_PATH=%s", e.taskEnv.WorktreePath))
	}
	if e.taskEnv.TicketPath != "" {
		env = append(env, fmt.Sprintf("ADB_TICKET_PATH=%s", e.taskEnv.TicketPath))
	}

	return env
}

// logFailure logs command failures to context.md
func (e *DefaultCLIExecutor) logFailure(command, stdout, stderr string, err error) {
	if e.contextFile == "" {
		return
	}

	// Create log entry
	logEntry := fmt.Sprintf("\n\n## Command Failure\n\n**Command:** `%s`\n\n**Error:** %s\n\n", command, err.Error())
	if stdout != "" {
		logEntry += fmt.Sprintf("**Stdout:**\n```\n%s\n```\n\n", stdout)
	}
	if stderr != "" {
		logEntry += fmt.Sprintf("**Stderr:**\n```\n%s\n```\n\n", stderr)
	}

	// Append to context.md
	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(e.contextFile)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return
	}

	// Append to file (create if doesn't exist)
	f, err := os.OpenFile(e.contextFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(logEntry)
}
