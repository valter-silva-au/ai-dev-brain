package integration

import (
	"fmt"
	"os"
	"path/filepath"
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
