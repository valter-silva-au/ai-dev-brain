package integration

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCLIExecutor(t *testing.T) {
	aliases := map[string]string{
		"ll": "ls -la",
	}
	taskEnv := TaskEnv{
		TaskID:       "TASK-001",
		Branch:       "task/001",
		WorktreePath: "/path/to/worktree",
		TicketPath:   "/path/to/ticket",
	}
	contextFile := "/path/to/context.md"

	executor := NewCLIExecutor(aliases, taskEnv, contextFile)
	if executor == nil {
		t.Fatal("NewCLIExecutor() returned nil")
	}

	// Test with nil aliases
	executor = NewCLIExecutor(nil, taskEnv, contextFile)
	if executor == nil {
		t.Fatal("NewCLIExecutor() with nil aliases returned nil")
	}
}

func TestExecuteSimpleCommand(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	// Execute simple echo command
	stdout, stderr, err := executor.Execute("echo hello", "")
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	if !strings.Contains(stdout, "hello") {
		t.Errorf("Expected stdout to contain 'hello', got: %s", stdout)
	}
	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestExecuteWithPipe(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	// Execute command with pipe
	stdout, stderr, err := executor.Execute("echo hello | grep hello", "")
	if err != nil {
		t.Fatalf("Execute() with pipe failed: %v", err)
	}

	if !strings.Contains(stdout, "hello") {
		t.Errorf("Expected stdout to contain 'hello', got: %s", stdout)
	}
	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestExecuteWithWorkDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file in temp directory
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	// Execute ls command in temp directory
	stdout, _, err := executor.Execute("ls", tempDir)
	if err != nil {
		t.Fatalf("Execute() with workDir failed: %v", err)
	}

	if !strings.Contains(stdout, "test.txt") {
		t.Errorf("Expected stdout to contain 'test.txt', got: %s", stdout)
	}
}

func TestExecuteWithAliasResolution(t *testing.T) {
	aliases := map[string]string{
		"ll":    "ls -la",
		"greet": "echo Hello",
	}
	executor := NewCLIExecutor(aliases, TaskEnv{}, "")

	// Execute command with alias
	stdout, _, err := executor.Execute("greet World", "")
	if err != nil {
		t.Fatalf("Execute() with alias failed: %v", err)
	}

	if !strings.Contains(stdout, "Hello World") {
		t.Errorf("Expected stdout to contain 'Hello World', got: %s", stdout)
	}
}

func TestExecuteWithEnvInjection(t *testing.T) {
	taskEnv := TaskEnv{
		TaskID:       "TASK-123",
		Branch:       "task/123",
		WorktreePath: "/work/task-123",
		TicketPath:   "/tickets/TASK-123",
	}
	executor := NewCLIExecutor(nil, taskEnv, "")

	// Use 'env' command (no shell metacharacters) to verify env vars are injected
	stdout, _, err := executor.Execute("env", "")
	if err != nil {
		t.Fatalf("Execute() with env injection failed: %v", err)
	}

	expected := []string{"ADB_TASK_ID=TASK-123", "ADB_BRANCH=task/123", "ADB_WORKTREE_PATH=/work/task-123", "ADB_TICKET_PATH=/tickets/TASK-123"}
	for _, exp := range expected {
		if !strings.Contains(stdout, exp) {
			t.Errorf("Expected stdout to contain '%s', got: %s", exp, stdout)
		}
	}
}

func TestExecuteRejectsInjection(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	tests := []struct {
		name    string
		command string
	}{
		{"semicolon injection", "echo hello; rm -rf /"},
		{"backtick injection", "echo `whoami`"},
		{"subshell injection", "echo $(whoami)"},
		{"dollar injection in shell mode", "sh -c 'echo $HOME'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executor.Execute(tt.command, "")
			if err == nil {
				t.Errorf("Execute(%q) should have been rejected", tt.command)
			}
		})
	}
}

func TestValidateCommandArgs(t *testing.T) {
	// Safe args
	if err := ValidateCommandArgs([]string{"hello", "world", "--flag", "-v", "path/to/file"}); err != nil {
		t.Errorf("ValidateCommandArgs() rejected safe args: %v", err)
	}

	// Dangerous args
	dangerous := []string{"hello;world", "$(whoami)", "`id`", "foo()", "a;b"}
	for _, arg := range dangerous {
		if err := ValidateCommandArgs([]string{arg}); err == nil {
			t.Errorf("ValidateCommandArgs(%q) should have been rejected", arg)
		}
	}
}

func TestExecuteWithWriter(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	var buf bytes.Buffer
	stdout, stderr, err := executor.ExecuteWithWriter("echo test", "", &buf)
	if err != nil {
		t.Fatalf("ExecuteWithWriter() failed: %v", err)
	}

	if !strings.Contains(stdout, "test") {
		t.Errorf("Expected stdout to contain 'test', got: %s", stdout)
	}
	if !strings.Contains(buf.String(), "test") {
		t.Errorf("Expected writer to contain 'test', got: %s", buf.String())
	}
	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestExecuteEmptyCommand(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	_, _, err := executor.Execute("", "")
	if err == nil {
		t.Error("Execute() with empty command should return error")
	}
	if !strings.Contains(err.Error(), "command cannot be empty") {
		t.Errorf("Expected error to contain 'command cannot be empty', got: %v", err)
	}
}

func TestExecuteFailureLogging(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, "context.md")

	executor := NewCLIExecutor(nil, TaskEnv{}, contextFile)

	// Execute command that will fail
	_, _, err := executor.Execute("sh -c 'exit 1'", "")
	if err == nil {
		t.Error("Execute() should have failed")
	}

	// Check if failure was logged to context.md
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		t.Error("context.md was not created")
		return
	}

	content, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatalf("Failed to read context.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Command Failure") {
		t.Error("context.md does not contain 'Command Failure'")
	}
	if !strings.Contains(contentStr, "sh -c") {
		t.Error("context.md does not contain the failed command")
	}
}

func TestExecuteWithMultiWriter(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	var buf bytes.Buffer
	stdout, _, err := executor.ExecuteWithWriter("echo multi", "", &buf)
	if err != nil {
		t.Fatalf("ExecuteWithWriter() failed: %v", err)
	}

	// Both stdout and writer should have the output
	if !strings.Contains(stdout, "multi") {
		t.Errorf("stdout should contain 'multi', got: %s", stdout)
	}
	if !strings.Contains(buf.String(), "multi") {
		t.Errorf("writer should contain 'multi', got: %s", buf.String())
	}
}

func TestResolveAlias(t *testing.T) {
	aliases := map[string]string{
		"ll":   "ls -la",
		"gst":  "git status",
		"test": "go test",
	}
	executor := &DefaultCLIExecutor{aliases: aliases}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple alias",
			input:    "ll",
			expected: "ls -la",
		},
		{
			name:     "Alias with arguments",
			input:    "ll /tmp",
			expected: "ls -la /tmp",
		},
		{
			name:     "No alias",
			input:    "echo hello",
			expected: "echo hello",
		},
		{
			name:     "Empty command",
			input:    "",
			expected: "",
		},
		{
			name:     "Alias at start",
			input:    "gst",
			expected: "git status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.resolveAlias(tt.input)
			if result != tt.expected {
				t.Errorf("resolveAlias(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInjectEnv(t *testing.T) {
	taskEnv := TaskEnv{
		TaskID:       "TASK-456",
		Branch:       "task/456",
		WorktreePath: "/work/456",
		TicketPath:   "/tickets/456",
	}
	executor := &DefaultCLIExecutor{taskEnv: taskEnv}

	baseEnv := []string{"PATH=/usr/bin", "HOME=/home/user"}
	result := executor.injectEnv(baseEnv)

	// Check that base env is preserved
	found := false
	for _, env := range result {
		if env == "PATH=/usr/bin" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Base environment was not preserved")
	}

	// Check that task env was injected
	expectedVars := []string{
		"ADB_TASK_ID=TASK-456",
		"ADB_BRANCH=task/456",
		"ADB_WORKTREE_PATH=/work/456",
		"ADB_TICKET_PATH=/tickets/456",
	}

	for _, expected := range expectedVars {
		found := false
		for _, env := range result {
			if env == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected environment variable not found: %s", expected)
		}
	}
}

func TestInjectEnvPartial(t *testing.T) {
	// Test with partial task env
	taskEnv := TaskEnv{
		TaskID: "TASK-789",
		// Other fields empty
	}
	executor := &DefaultCLIExecutor{taskEnv: taskEnv}

	baseEnv := []string{"PATH=/usr/bin"}
	result := executor.injectEnv(baseEnv)

	// Should only have ADB_TASK_ID
	found := false
	for _, env := range result {
		if env == "ADB_TASK_ID=TASK-789" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ADB_TASK_ID was not injected")
	}

	// Should not have empty vars
	for _, env := range result {
		if strings.HasPrefix(env, "ADB_BRANCH=") && env == "ADB_BRANCH=" {
			t.Error("Empty ADB_BRANCH should not be injected")
		}
	}
}

func TestExecuteWithWriterNilWriter(t *testing.T) {
	executor := NewCLIExecutor(nil, TaskEnv{}, "")

	stdout, _, err := executor.ExecuteWithWriter("echo test", "", nil)
	if err != nil {
		t.Fatalf("ExecuteWithWriter() with nil writer failed: %v", err)
	}

	if !strings.Contains(stdout, "test") {
		t.Errorf("Expected stdout to contain 'test', got: %s", stdout)
	}
}

func TestLogFailure(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, "subdir", "context.md")

	executor := &DefaultCLIExecutor{contextFile: contextFile}

	// Test logging failure
	executor.logFailure("test command", "stdout content", "stderr content", io.EOF)

	// Check file was created
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		t.Fatal("context.md was not created")
	}

	// Check content
	content, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatalf("Failed to read context.md: %v", err)
	}

	contentStr := string(content)
	expectedStrings := []string{
		"Command Failure",
		"test command",
		"stdout content",
		"stderr content",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected context.md to contain '%s', got: %s", expected, contentStr)
		}
	}
}

func TestLogFailureEmptyContextFile(t *testing.T) {
	executor := &DefaultCLIExecutor{contextFile: ""}

	// Should not panic or error
	executor.logFailure("test", "out", "err", io.EOF)
}
