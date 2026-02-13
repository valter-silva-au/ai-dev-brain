package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- ResolveAlias tests ---

func TestResolveAlias_Found(t *testing.T) {
	executor := NewCLIExecutor()
	aliases := []CLIAlias{
		{Name: "k", Command: "kubectl", DefaultArgs: []string{"--context", "dev"}},
		{Name: "gh", Command: "gh"},
	}

	cmd, args, found := executor.ResolveAlias("k", aliases)
	if !found {
		t.Fatal("expected alias 'k' to be found")
	}
	if cmd != "kubectl" {
		t.Errorf("command = %q, want %q", cmd, "kubectl")
	}
	if len(args) != 2 || args[0] != "--context" || args[1] != "dev" {
		t.Errorf("defaultArgs = %v, want [--context dev]", args)
	}
}

func TestResolveAlias_NotFound(t *testing.T) {
	executor := NewCLIExecutor()
	aliases := []CLIAlias{
		{Name: "k", Command: "kubectl"},
	}

	cmd, args, found := executor.ResolveAlias("git", aliases)
	if found {
		t.Fatal("expected alias 'git' to NOT be found")
	}
	if cmd != "git" {
		t.Errorf("command = %q, want %q (original name)", cmd, "git")
	}
	if args != nil {
		t.Errorf("defaultArgs = %v, want nil", args)
	}
}

func TestResolveAlias_EmptyAliases(t *testing.T) {
	executor := NewCLIExecutor()

	cmd, args, found := executor.ResolveAlias("docker", nil)
	if found {
		t.Fatal("expected no match in empty aliases")
	}
	if cmd != "docker" {
		t.Errorf("command = %q, want %q", cmd, "docker")
	}
	if args != nil {
		t.Errorf("defaultArgs = %v, want nil", args)
	}
}

func TestResolveAlias_NoDefaultArgs(t *testing.T) {
	executor := NewCLIExecutor()
	aliases := []CLIAlias{
		{Name: "gh", Command: "gh"},
	}

	cmd, args, found := executor.ResolveAlias("gh", aliases)
	if !found {
		t.Fatal("expected alias 'gh' to be found")
	}
	if cmd != "gh" {
		t.Errorf("command = %q, want %q", cmd, "gh")
	}
	if len(args) != 0 {
		t.Errorf("defaultArgs = %v, want empty/nil", args)
	}
}

// --- BuildEnv tests ---

func TestBuildEnv_WithTaskContext(t *testing.T) {
	executor := NewCLIExecutor()
	base := []string{"HOME=/home/user", "PATH=/usr/bin"}
	ctx := &TaskEnvContext{
		TaskID:       "TASK-00042",
		Branch:       "feat/oauth-flow",
		WorktreePath: "/repos/github.com/org/repo/work/TASK-00042",
		TicketPath:   "/tickets/TASK-00042",
	}

	env := executor.BuildEnv(base, ctx)

	// Base env should be preserved.
	if len(env) != len(base)+4 {
		t.Fatalf("env length = %d, want %d", len(env), len(base)+4)
	}
	if env[0] != "HOME=/home/user" || env[1] != "PATH=/usr/bin" {
		t.Errorf("base env not preserved: %v", env[:2])
	}

	// Check ADB variables.
	expected := map[string]string{
		"ADB_TASK_ID":       "TASK-00042",
		"ADB_BRANCH":        "feat/oauth-flow",
		"ADB_WORKTREE_PATH": "/repos/github.com/org/repo/work/TASK-00042",
		"ADB_TICKET_PATH":   "/tickets/TASK-00042",
	}
	for _, entry := range env[len(base):] {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			t.Errorf("malformed env entry: %q", entry)
			continue
		}
		want, ok := expected[parts[0]]
		if !ok {
			t.Errorf("unexpected env var: %q", parts[0])
			continue
		}
		if parts[1] != want {
			t.Errorf("%s = %q, want %q", parts[0], parts[1], want)
		}
		delete(expected, parts[0])
	}
	for k := range expected {
		t.Errorf("missing env var: %s", k)
	}
}

func TestBuildEnv_NilTaskContext(t *testing.T) {
	executor := NewCLIExecutor()
	base := []string{"HOME=/home/user"}

	env := executor.BuildEnv(base, nil)

	if len(env) != len(base) {
		t.Errorf("env length = %d, want %d (no ADB vars)", len(env), len(base))
	}
}

func TestBuildEnv_NilBase(t *testing.T) {
	executor := NewCLIExecutor()
	ctx := &TaskEnvContext{
		TaskID:       "TASK-00001",
		Branch:       "feat/test",
		WorktreePath: "/worktree",
		TicketPath:   "/ticket",
	}

	env := executor.BuildEnv(nil, ctx)
	if len(env) != 4 {
		t.Fatalf("env length = %d, want 4", len(env))
	}
}

// --- ListAliases tests ---

func TestListAliases_WithDefaults(t *testing.T) {
	executor := NewCLIExecutor()
	aliases := []CLIAlias{
		{Name: "k", Command: "kubectl", DefaultArgs: []string{"--context", "dev"}},
		{Name: "gh", Command: "gh"},
	}

	result := executor.ListAliases(aliases)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "k -> kubectl [--context dev]" {
		t.Errorf("result[0] = %q, want %q", result[0], "k -> kubectl [--context dev]")
	}
	if result[1] != "gh -> gh" {
		t.Errorf("result[1] = %q, want %q", result[1], "gh -> gh")
	}
}

func TestListAliases_Empty(t *testing.T) {
	executor := NewCLIExecutor()

	result := executor.ListAliases(nil)
	if len(result) != 0 {
		t.Errorf("len = %d, want 0", len(result))
	}
}

// --- Exec tests ---

func TestExec_SimpleCommand(t *testing.T) {
	executor := NewCLIExecutor()

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "echo hello"}
	} else {
		cli = "echo"
		args = []string{"hello"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:  cli,
		Args: args,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("stdout = %q, want to contain 'hello'", result.Stdout)
	}
}

func TestExec_FailingCommand(t *testing.T) {
	executor := NewCLIExecutor()

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "exit 1"}
	} else {
		cli = "sh"
		args = []string{"-c", "exit 1"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:  cli,
		Args: args,
	})
	if err != nil {
		t.Fatalf("unexpected error (should return result with exit code): %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
}

func TestExec_NonExistentCommand(t *testing.T) {
	executor := NewCLIExecutor()

	_, err := executor.Exec(CLIExecConfig{
		CLI:  "nonexistent_command_xyz_12345",
		Args: nil,
	})
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestExec_WithAlias(t *testing.T) {
	executor := NewCLIExecutor()
	aliases := []CLIAlias{
		{Name: "myecho", Command: "echo", DefaultArgs: []string{"default-prefix"}},
	}

	if runtime.GOOS == "windows" {
		// echo on Windows works differently; use cmd /c echo
		aliases = []CLIAlias{
			{Name: "myecho", Command: "cmd", DefaultArgs: []string{"/c", "echo"}},
		}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:     "myecho",
		Args:    []string{"user-arg"},
		Aliases: aliases,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestExec_CapturesStdout(t *testing.T) {
	executor := NewCLIExecutor()
	var outBuf bytes.Buffer

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "echo captured"}
	} else {
		cli = "echo"
		args = []string{"captured"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:    cli,
		Args:   args,
		Stdout: &outBuf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both result.Stdout and the writer should have the output.
	if !strings.Contains(result.Stdout, "captured") {
		t.Errorf("result.Stdout = %q, want to contain 'captured'", result.Stdout)
	}
	if !strings.Contains(outBuf.String(), "captured") {
		t.Errorf("writer output = %q, want to contain 'captured'", outBuf.String())
	}
}

func TestExec_FailureLogsToContext(t *testing.T) {
	executor := NewCLIExecutor()
	ticketDir := t.TempDir()

	ctx := &TaskEnvContext{
		TaskID:       "TASK-00001",
		Branch:       "feat/test",
		WorktreePath: "/worktree",
		TicketPath:   ticketDir,
	}

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "echo FAIL>&2 && exit 1"}
	} else {
		cli = "sh"
		args = []string{"-c", "echo FAIL >&2; exit 1"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:     cli,
		Args:    args,
		TaskCtx: ctx,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}

	// Check that context.md was created with failure info.
	contextPath := filepath.Join(ticketDir, "context.md")
	data, readErr := os.ReadFile(contextPath)
	if readErr != nil {
		t.Fatalf("failed to read context.md: %v", readErr)
	}
	content := string(data)
	if !strings.Contains(content, "CLI Failure") {
		t.Errorf("context.md missing 'CLI Failure' heading")
	}
	if !strings.Contains(content, "Exit Code:** 1") {
		t.Errorf("context.md missing exit code")
	}
}

// --- LogFailure tests ---

func TestLogFailure_NilContext(t *testing.T) {
	executor := NewCLIExecutor()

	err := executor.LogFailure(nil, "git", []string{"push"}, &CLIExecResult{ExitCode: 1})
	if err != nil {
		t.Errorf("unexpected error for nil context: %v", err)
	}
}

func TestLogFailure_AppendsToContext(t *testing.T) {
	executor := NewCLIExecutor()
	ticketDir := t.TempDir()

	// Write an initial context.md.
	initialContent := "# Task Context: TASK-00001\n\n## Summary\nInitial content.\n"
	if err := os.WriteFile(filepath.Join(ticketDir, "context.md"), []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial context.md: %v", err)
	}

	ctx := &TaskEnvContext{
		TaskID:     "TASK-00001",
		TicketPath: ticketDir,
	}

	err := executor.LogFailure(ctx, "npm", []string{"test", "--verbose"}, &CLIExecResult{
		ExitCode: 2,
		Stderr:   "test failed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(ticketDir, "context.md"))
	if readErr != nil {
		t.Fatalf("failed to read context.md: %v", readErr)
	}
	content := string(data)

	// Initial content should still be there.
	if !strings.Contains(content, "Initial content.") {
		t.Error("initial content was lost")
	}
	// Failure entry should be appended.
	if !strings.Contains(content, "npm test --verbose") {
		t.Errorf("context.md missing command: %s", content)
	}
	if !strings.Contains(content, "test failed") {
		t.Error("context.md missing stderr")
	}
}

// --- containsPipe tests ---

func TestContainsPipe_NoPipe(t *testing.T) {
	if containsPipe([]string{"echo", "hello"}) {
		t.Error("expected false for args without pipe")
	}
}

func TestContainsPipe_WithPipe(t *testing.T) {
	if !containsPipe([]string{"echo", "hello", "|", "wc", "-l"}) {
		t.Error("expected true for args with pipe")
	}
}

func TestContainsPipe_EmptyArgs(t *testing.T) {
	if containsPipe(nil) {
		t.Error("expected false for nil args")
	}
	if containsPipe([]string{}) {
		t.Error("expected false for empty args")
	}
}

// --- Exec with pipe ---

func TestExec_WithPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pipe test uses sh -c on non-Windows")
	}
	executor := NewCLIExecutor()

	result, err := executor.Exec(CLIExecConfig{
		CLI:  "echo",
		Args: []string{"hello world", "|", "wc", "-w"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestExec_CapturesStderr(t *testing.T) {
	executor := NewCLIExecutor()
	var errBuf bytes.Buffer

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "echo stderr_msg>&2"}
	} else {
		cli = "sh"
		args = []string{"-c", "echo stderr_msg >&2"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:    cli,
		Args:   args,
		Stderr: &errBuf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stderr, "stderr_msg") {
		t.Errorf("result.Stderr = %q, want to contain 'stderr_msg'", result.Stderr)
	}
	if !strings.Contains(errBuf.String(), "stderr_msg") {
		t.Errorf("errBuf = %q, want to contain 'stderr_msg'", errBuf.String())
	}
}

func TestExec_WithStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdin test uses cat on non-Windows")
	}
	executor := NewCLIExecutor()
	input := strings.NewReader("input_data\n")

	result, err := executor.Exec(CLIExecConfig{
		CLI:   "cat",
		Stdin: input,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "input_data") {
		t.Errorf("stdout = %q, want to contain 'input_data'", result.Stdout)
	}
}

// --- LogFailure additional tests ---

func TestLogFailure_InvalidTicketPath_ReturnsError(t *testing.T) {
	executor := NewCLIExecutor()
	ctx := &TaskEnvContext{
		TaskID:     "TASK-00001",
		TicketPath: "/nonexistent/path/that/does/not/exist",
	}

	err := executor.LogFailure(ctx, "git", []string{"push"}, &CLIExecResult{
		ExitCode: 1,
		Stderr:   "error",
	})
	if err == nil {
		t.Fatal("expected error for invalid ticket path")
	}
	if !strings.Contains(err.Error(), "opening context file") {
		t.Errorf("error = %q, want to contain 'opening context file'", err.Error())
	}
}

func TestExec_FailingCommand_WithTaskCtx_LogsFailure(t *testing.T) {
	executor := NewCLIExecutor()
	ticketDir := t.TempDir()

	ctx := &TaskEnvContext{
		TaskID:       "TASK-00001",
		Branch:       "feat/test",
		WorktreePath: "/worktree",
		TicketPath:   ticketDir,
	}

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "exit 42"}
	} else {
		cli = "sh"
		args = []string{"-c", "exit 42"}
	}

	result, err := executor.Exec(CLIExecConfig{
		CLI:     cli,
		Args:    args,
		TaskCtx: ctx,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}

	// Verify failure was logged.
	data, readErr := os.ReadFile(filepath.Join(ticketDir, "context.md"))
	if readErr != nil {
		t.Fatalf("failed to read context.md: %v", readErr)
	}
	if !strings.Contains(string(data), "CLI Failure") {
		t.Error("context.md missing 'CLI Failure'")
	}
}

func TestExec_FailingCommand_LogFailureError_PrintsWarning(t *testing.T) {
	// When a command fails and LogFailure also fails (e.g., bad ticket path),
	// the warning is printed to stderr but the command result is still returned.
	executor := NewCLIExecutor()

	ctx := &TaskEnvContext{
		TaskID:       "TASK-00001",
		Branch:       "feat/test",
		WorktreePath: "/worktree",
		TicketPath:   "/nonexistent/path/for/test",
	}

	var errBuf bytes.Buffer

	var cli string
	var args []string
	if runtime.GOOS == "windows" {
		cli = "cmd"
		args = []string{"/c", "exit 1"}
	} else {
		cli = "sh"
		args = []string{"-c", "exit 1"}
	}

	// Redirect os.Stderr temporarily to capture the warning.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result, err := executor.Exec(CLIExecConfig{
		CLI:     cli,
		Args:    args,
		TaskCtx: ctx,
		Stderr:  &errBuf,
	})

	_ = w.Close()
	os.Stderr = origStderr

	warningBuf := make([]byte, 4096)
	n, _ := r.Read(warningBuf)
	_ = r.Close()
	warning := string(warningBuf[:n])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}

	// The warning about LogFailure should have been printed to stderr.
	if !strings.Contains(warning, "warning: failed to log CLI failure") {
		t.Errorf("expected warning about LogFailure, got: %q", warning)
	}
}

func TestExec_WithPipe_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	executor := NewCLIExecutor()

	// Test pipe handling on Windows (uses cmd /c).
	result, err := executor.Exec(CLIExecConfig{
		CLI:  "echo",
		Args: []string{"hello", "|", "findstr", "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestLogFailure_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not supported on Windows: Unix file permissions not available on Windows")
	}
	// Test LogFailure when WriteString fails due to a closed file.
	executor := NewCLIExecutor()
	ticketDir := t.TempDir()

	// Create an initial context.md file.
	contextPath := filepath.Join(ticketDir, "context.md")
	if err := os.WriteFile(contextPath, []byte("# Context\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &TaskEnvContext{
		TaskID:     "TASK-00001",
		TicketPath: ticketDir,
	}

	// Make the directory read-only so appending to the file fails.
	if err := os.Chmod(ticketDir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(ticketDir, 0o755) }()

	err := executor.LogFailure(ctx, "git", []string{"push"}, &CLIExecResult{
		ExitCode: 1,
		Stderr:   "push failed",
	})
	if err == nil {
		t.Fatal("expected error when writing to context file fails")
	}
	if !strings.Contains(err.Error(), "opening context file") {
		t.Errorf("error = %q, want to contain 'opening context file'", err.Error())
	}
}
