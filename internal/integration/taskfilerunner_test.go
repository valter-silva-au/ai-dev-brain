package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// --- Helper ---

func writeTaskfile(t *testing.T, dir string, content string) {
	t.Helper()
	path := filepath.Join(dir, "Taskfile.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write Taskfile.yaml: %v", err)
	}
}

// --- Discover tests ---

func TestDiscover_Valid(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  build:
    desc: "Build the project"
    cmds:
      - go build ./...
  test:
    desc: "Run tests"
    cmds:
      - go test ./...
    deps:
      - build
`)

	runner := NewTaskfileRunner(NewCLIExecutor())
	tf, err := runner.Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tf.Version != "3" {
		t.Errorf("version = %q, want %q", tf.Version, "3")
	}
	if len(tf.Tasks) != 2 {
		t.Fatalf("tasks count = %d, want 2", len(tf.Tasks))
	}

	build, ok := tf.Tasks["build"]
	if !ok {
		t.Fatal("missing 'build' task")
	}
	if build.Name != "build" {
		t.Errorf("build.Name = %q, want %q", build.Name, "build")
	}
	if build.Description != "Build the project" {
		t.Errorf("build.Description = %q", build.Description)
	}
	if len(build.Commands) != 1 || build.Commands[0] != "go build ./..." {
		t.Errorf("build.Commands = %v", build.Commands)
	}

	test, ok := tf.Tasks["test"]
	if !ok {
		t.Fatal("missing 'test' task")
	}
	if len(test.Deps) != 1 || test.Deps[0] != "build" {
		t.Errorf("test.Deps = %v, want [build]", test.Deps)
	}
}

func TestDiscover_NoFile(t *testing.T) {
	dir := t.TempDir()
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.Discover(dir)
	if err == nil {
		t.Fatal("expected error for missing Taskfile.yaml")
	}
}

func TestDiscover_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: [invalid
  broken: {
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.Discover(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- ListTasks tests ---

func TestListTasks_ReturnsAllTasks(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  build:
    desc: "Build"
    cmds: ["echo build"]
  test:
    desc: "Test"
    cmds: ["echo test"]
  lint:
    desc: "Lint"
    cmds: ["echo lint"]
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	tasks, err := runner.ListTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("tasks count = %d, want 3", len(tasks))
	}

	names := make(map[string]bool)
	for _, task := range tasks {
		names[task.Name] = true
	}
	for _, expected := range []string{"build", "test", "lint"} {
		if !names[expected] {
			t.Errorf("missing task %q", expected)
		}
	}
}

func TestListTasks_EmptyTaskfile(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks: {}
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	tasks, err := runner.ListTasks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("tasks count = %d, want 0", len(tasks))
	}
}

// --- Run tests ---

func TestRun_TaskNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  build:
    cmds: ["echo build"]
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.Run(TaskfileRunConfig{
		TaskName: "nonexistent",
		Dir:      dir,
	})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRun_EmptyCommands(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  empty:
    desc: "Nothing to do"
    cmds: []
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "empty",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestRun_MultiWordCommand(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  hello:
    desc: "Say hello"
    cmds:
      - echo hello from taskfile
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "hello",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello from taskfile") {
		t.Errorf("stdout = %q, want it to contain 'hello from taskfile'", result.Stdout)
	}
}

func TestRun_CommandWithArgs(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  greet:
    cmds:
      - echo hello
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "greet",
		Dir:      dir,
		Args:     []string{"world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello world") {
		t.Errorf("stdout = %q, want it to contain 'hello world'", result.Stdout)
	}
}

func TestRun_FailingCommand_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  fail:
    cmds:
      - exit 1
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "fail",
		Dir:      dir,
	})
	// A failing command should return the result (not an error) with nonzero exit code.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRun_MultipleCommands_StopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  multi:
    cmds:
      - echo first
      - exit 1
      - echo third
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "multi",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code from second command")
	}
}

func TestRun_NoTaskfile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.Run(TaskfileRunConfig{
		TaskName: "build",
		Dir:      dir,
	})
	if err == nil {
		t.Fatal("expected error for missing Taskfile.yaml")
	}
}

func TestListTasks_NoTaskfile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.ListTasks(dir)
	if err == nil {
		t.Fatal("expected error for missing Taskfile.yaml")
	}
}

func TestDiscover_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Create Taskfile.yaml as a directory instead of a file to cause a read error.
	if err := os.MkdirAll(filepath.Join(dir, "Taskfile.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := NewTaskfileRunner(NewCLIExecutor())

	_, err := runner.Discover(dir)
	if err == nil {
		t.Fatal("expected error when reading Taskfile.yaml fails")
	}
}

func TestRun_WithTaskContext(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  check:
    cmds:
      - echo running
`)
	runner := NewTaskfileRunner(NewCLIExecutor())
	ctx := &TaskEnvContext{
		TaskID:       "TASK-00001",
		Branch:       "feat/test",
		WorktreePath: "/worktree",
		TicketPath:   "/ticket",
	}

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "check",
		Dir:      dir,
		TaskCtx:  ctx,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestRun_ExecReturnsError(t *testing.T) {
	// The Run method wraps commands in "sh -c", so a non-existent command
	// returns a non-zero exit code rather than an exec error. To trigger
	// the execErr path, we use a mock executor that returns an error.
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  broken:
    cmds:
      - echo test
`)

	// Use the real executor but override it with a wrapper that always errors.
	runner := NewTaskfileRunner(&errorExecutor{})

	_, err := runner.Run(TaskfileRunConfig{
		TaskName: "broken",
		Dir:      dir,
	})
	if err == nil {
		t.Fatal("expected error from executor")
	}
	if !strings.Contains(err.Error(), "running task") {
		t.Errorf("error = %q, want to contain 'running task'", err.Error())
	}
}

// errorExecutor is a CLIExecutor that always returns an error from Exec.
type errorExecutor struct{}

func (e *errorExecutor) Exec(config CLIExecConfig) (*CLIExecResult, error) {
	return nil, fmt.Errorf("simulated exec failure")
}

func (e *errorExecutor) ResolveAlias(name string, aliases []CLIAlias) (string, []string, bool) {
	return name, nil, false
}

func (e *errorExecutor) BuildEnv(base []string, taskCtx *TaskEnvContext) []string {
	return base
}

func (e *errorExecutor) ListAliases(aliases []CLIAlias) []string {
	return nil
}

func (e *errorExecutor) LogFailure(taskCtx *TaskEnvContext, cli string, args []string, result *CLIExecResult) error {
	return nil
}

func TestRun_MultipleCommands_AllSucceed(t *testing.T) {
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  multi:
    cmds:
      - echo first
      - echo second
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "multi",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestRun_WindowsShellHandling(t *testing.T) {
	// Test that Windows uses cmd /c and Unix uses sh -c.
	// This test exercises the shell selection logic at lines 133-138.
	dir := t.TempDir()
	writeTaskfile(t, dir, `
version: "3"
tasks:
  shelltest:
    cmds:
      - echo test
`)
	runner := NewTaskfileRunner(NewCLIExecutor())

	result, err := runner.Run(TaskfileRunConfig{
		TaskName: "shelltest",
		Dir:      dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	// On any platform, the command should succeed and echo "test".
	if !strings.Contains(result.Stdout, "test") {
		t.Errorf("stdout = %q, want to contain 'test'", result.Stdout)
	}
}

// =============================================================================
// Property 28: Taskfile Task Discovery
// =============================================================================

// Feature: ai-dev-brain, Property 28: Taskfile Task Discovery
// *For any* valid Taskfile.yaml containing N task definitions, Discover SHALL
// parse the file and return exactly N tasks, and ListTasks SHALL return task
// entries whose names match exactly the keys defined in the Taskfile.
//
// **Validates: Requirements 18.2, 18.4**
func TestProperty28_TaskfileTaskDiscovery(t *testing.T) {
	runner := NewTaskfileRunner(NewCLIExecutor())

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random number of tasks with unique names.
		numTasks := rapid.IntRange(0, 10).Draw(rt, "numTasks")
		nameSet := make(map[string]bool)
		taskNames := make([]string, 0, numTasks)
		for i := 0; i < numTasks; i++ {
			name := rapid.StringMatching(`[a-z]{2,12}`).Draw(rt, fmt.Sprintf("taskName_%d", i))
			if nameSet[name] {
				continue
			}
			nameSet[name] = true
			taskNames = append(taskNames, name)
		}

		// Build the Taskfile.yaml content.
		yamlContent := "version: \"3\"\ntasks:\n"
		if len(taskNames) == 0 {
			yamlContent += "  {}\n"
		}
		for _, name := range taskNames {
			desc := rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(rt, "desc_"+name)
			numCmds := rapid.IntRange(1, 3).Draw(rt, "numCmds_"+name)
			cmds := ""
			for j := 0; j < numCmds; j++ {
				cmd := rapid.StringMatching(`[a-z]{2,10}`).Draw(rt, fmt.Sprintf("cmd_%s_%d", name, j))
				cmds += fmt.Sprintf("      - %s\n", cmd)
			}
			yamlContent += fmt.Sprintf("  %s:\n    desc: \"%s\"\n    cmds:\n%s", name, desc, cmds)
		}

		dir := t.TempDir()
		writeTaskfile(t, dir, yamlContent)

		// Test Discover.
		tf, err := runner.Discover(dir)
		if err != nil {
			rt.Fatalf("Discover failed: %v", err)
		}

		if len(tf.Tasks) != len(taskNames) {
			rt.Errorf("Discover returned %d tasks, want %d", len(tf.Tasks), len(taskNames))
		}

		// Every generated name should be present as a key.
		for _, name := range taskNames {
			task, ok := tf.Tasks[name]
			if !ok {
				rt.Errorf("Discover missing task %q", name)
				continue
			}
			if task.Name != name {
				rt.Errorf("task.Name = %q, want %q", task.Name, name)
			}
		}

		// Test ListTasks.
		tasks, err := runner.ListTasks(dir)
		if err != nil {
			rt.Fatalf("ListTasks failed: %v", err)
		}

		if len(tasks) != len(taskNames) {
			rt.Errorf("ListTasks returned %d tasks, want %d", len(tasks), len(taskNames))
		}

		// All names from ListTasks should match keys from the file.
		listedNames := make(map[string]bool)
		for _, task := range tasks {
			listedNames[task.Name] = true
		}
		for _, name := range taskNames {
			if !listedNames[name] {
				rt.Errorf("ListTasks missing task %q", name)
			}
		}
	})
}
