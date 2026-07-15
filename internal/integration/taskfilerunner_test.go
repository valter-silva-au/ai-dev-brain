package integration

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskfileRunner_DiscoverTaskfile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	level1 := filepath.Join(tmpDir, "level1")
	level2 := filepath.Join(level1, "level2")
	os.MkdirAll(level2, 0o755)

	// Create Taskfile in level1
	taskfilePath := filepath.Join(level1, "Taskfile.yaml")
	taskfileContent := `version: '3'
tasks:
  test:
    desc: Test task
    cmds:
      - echo "test"
`
	os.WriteFile(taskfilePath, []byte(taskfileContent), 0o644)

	tr := NewTaskfileRunner()

	t.Run("FindInCurrentDir", func(t *testing.T) {
		found, err := tr.DiscoverTaskfile(level1)
		if err != nil {
			t.Errorf("Failed to discover Taskfile: %v", err)
		}

		if found != taskfilePath {
			t.Errorf("Expected %s, got %s", taskfilePath, found)
		}
	})

	t.Run("FindInParentDir", func(t *testing.T) {
		found, err := tr.DiscoverTaskfile(level2)
		if err != nil {
			t.Errorf("Failed to discover Taskfile in parent: %v", err)
		}

		if found != taskfilePath {
			t.Errorf("Expected %s, got %s", taskfilePath, found)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "empty")
		os.MkdirAll(emptyDir, 0o755)

		_, err := tr.DiscoverTaskfile(emptyDir)
		if err == nil {
			t.Error("Expected error when Taskfile not found")
		}
	})

	t.Run("AlternativeExtension", func(t *testing.T) {
		altDir := filepath.Join(tmpDir, "alt")
		os.MkdirAll(altDir, 0o755)

		altTaskfile := filepath.Join(altDir, "Taskfile.yml")
		os.WriteFile(altTaskfile, []byte(taskfileContent), 0o644)

		found, err := tr.DiscoverTaskfile(altDir)
		if err != nil {
			t.Errorf("Failed to discover Taskfile.yml: %v", err)
		}

		if found != altTaskfile {
			t.Errorf("Expected %s, got %s", altTaskfile, found)
		}
	})
}

func TestTaskfileRunner_LoadTaskfile(t *testing.T) {
	tmpDir := t.TempDir()
	taskfilePath := filepath.Join(tmpDir, "Taskfile.yaml")

	taskfileContent := `version: '3'

vars:
  GREETING: Hello

tasks:
  greet:
    desc: Greet the user
    cmds:
      - echo "{{.GREETING}}"

  build:
    desc: Build the project
    deps:
      - test
    cmds:
      - echo "Building..."

  test:
    desc: Run tests
    cmds:
      - echo "Testing..."
    silent: true
`

	os.WriteFile(taskfilePath, []byte(taskfileContent), 0o644)

	tr := NewTaskfileRunner()

	t.Run("LoadValidTaskfile", func(t *testing.T) {
		taskfile, err := tr.LoadTaskfile(taskfilePath)
		if err != nil {
			t.Fatalf("Failed to load Taskfile: %v", err)
		}

		if taskfile.Version != "3" {
			t.Errorf("Expected version '3', got '%s'", taskfile.Version)
		}

		if len(taskfile.Tasks) != 3 {
			t.Errorf("Expected 3 tasks, got %d", len(taskfile.Tasks))
		}

		greetTask, exists := taskfile.Tasks["greet"]
		if !exists {
			t.Fatal("Expected 'greet' task to exist")
		}

		if greetTask.Name != "greet" {
			t.Errorf("Expected task name 'greet', got '%s'", greetTask.Name)
		}

		if greetTask.Description != "Greet the user" {
			t.Errorf("Expected description 'Greet the user', got '%s'", greetTask.Description)
		}

		buildTask := taskfile.Tasks["build"]
		if len(buildTask.Deps) != 1 || buildTask.Deps[0] != "test" {
			t.Errorf("Expected build task to depend on test")
		}

		testTask := taskfile.Tasks["test"]
		if !testTask.Silent {
			t.Error("Expected test task to be silent")
		}
	})

	t.Run("LoadNonexistentTaskfile", func(t *testing.T) {
		_, err := tr.LoadTaskfile(filepath.Join(tmpDir, "nonexistent.yaml"))
		if err == nil {
			t.Error("Expected error when loading nonexistent Taskfile")
		}
	})

	t.Run("LoadInvalidYAML", func(t *testing.T) {
		invalidPath := filepath.Join(tmpDir, "invalid.yaml")
		os.WriteFile(invalidPath, []byte("invalid: yaml: content: {"), 0o644)

		_, err := tr.LoadTaskfile(invalidPath)
		if err == nil {
			t.Error("Expected error when loading invalid YAML")
		}
	})
}

func TestTaskfileRunner_ListTasks(t *testing.T) {
	tmpDir := t.TempDir()
	taskfilePath := filepath.Join(tmpDir, "Taskfile.yaml")

	taskfileContent := `version: '3'
tasks:
  task1:
    desc: First task
    cmds:
      - echo "1"
  task2:
    desc: Second task
    cmds:
      - echo "2"
`

	os.WriteFile(taskfilePath, []byte(taskfileContent), 0o644)

	tr := NewTaskfileRunner()
	taskfile, _ := tr.LoadTaskfile(taskfilePath)

	tasks := tr.ListTasks(taskfile)

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}

	taskNames := make(map[string]bool)
	for _, task := range tasks {
		taskNames[task.Name] = true
	}

	if !taskNames["task1"] || !taskNames["task2"] {
		t.Error("Expected task1 and task2 in list")
	}
}

func TestTaskfileRunner_ExpandVars(t *testing.T) {
	tr := &DefaultTaskfileRunner{}

	globalVars := map[string]string{
		"NAME":    "World",
		"VERSION": "1.0",
	}

	taskVars := map[string]string{
		"NAME": "Task", // Override global var
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ExpandTemplateVar",
			input:    "Hello {{.NAME}}",
			expected: "Hello Task",
		},
		{
			name:     "ExpandDollarVar",
			input:    "Version $VERSION",
			expected: "Version 1.0",
		},
		{
			name:     "ExpandMultipleVars",
			input:    "{{.NAME}} v$VERSION",
			expected: "Task v1.0",
		},
		{
			name:     "NoVarsToExpand",
			input:    "No variables here",
			expected: "No variables here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tr.expandVars(tt.input, globalVars, taskVars)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestTaskfileRunner_RunTask(t *testing.T) {
	tmpDir := t.TempDir()
	taskfilePath := filepath.Join(tmpDir, "Taskfile.yaml")

	taskfileContent := `version: '3'

vars:
  MSG: "Hello from Taskfile"

tasks:
  simple:
    desc: Simple task
    cmds:
      - echo "simple"

  with-var:
    desc: Task with variable
    cmds:
      - echo "{{.MSG}}"

  with-dep:
    desc: Task with dependency
    deps:
      - simple
    cmds:
      - echo "with-dep"
`

	os.WriteFile(taskfilePath, []byte(taskfileContent), 0o644)

	tr := NewTaskfileRunner()

	// Create a mock executor that captures commands
	type mockExecutor struct {
		commands []string
	}
	executor := &mockExecutor{commands: []string{}}

	// Implement CLIExecutor interface
	executeFunc := func(command string, workDir string) (string, string, error) {
		executor.commands = append(executor.commands, command)
		return "", "", nil
	}

	mockExec := &testCLIExecutor{executeFunc: executeFunc}

	t.Run("RunSimpleTask", func(t *testing.T) {
		executor.commands = []string{}
		err := tr.RunTask(taskfilePath, "simple", mockExec)
		if err != nil {
			t.Errorf("RunTask failed: %v", err)
		}

		if len(executor.commands) != 1 {
			t.Errorf("Expected 1 command, got %d", len(executor.commands))
		}

		if !strings.Contains(executor.commands[0], "echo \"simple\"") {
			t.Errorf("Expected 'echo \"simple\"', got '%s'", executor.commands[0])
		}
	})

	t.Run("RunTaskWithVar", func(t *testing.T) {
		executor.commands = []string{}
		err := tr.RunTask(taskfilePath, "with-var", mockExec)
		if err != nil {
			t.Errorf("RunTask failed: %v", err)
		}

		if len(executor.commands) != 1 {
			t.Errorf("Expected 1 command, got %d", len(executor.commands))
		}

		// Variable should be expanded
		if !strings.Contains(executor.commands[0], "Hello from Taskfile") {
			t.Errorf("Expected expanded variable, got '%s'", executor.commands[0])
		}
	})

	t.Run("RunTaskWithDependency", func(t *testing.T) {
		executor.commands = []string{}
		err := tr.RunTask(taskfilePath, "with-dep", mockExec)
		if err != nil {
			t.Errorf("RunTask failed: %v", err)
		}

		// Should execute dependency first, then the task
		if len(executor.commands) != 2 {
			t.Errorf("Expected 2 commands (dep + task), got %d", len(executor.commands))
		}
	})

	t.Run("RunNonexistentTask", func(t *testing.T) {
		err := tr.RunTask(taskfilePath, "nonexistent", mockExec)
		if err == nil {
			t.Error("Expected error when running nonexistent task")
		}
	})
}

// testCLIExecutor is a test implementation of CLIExecutor
type testCLIExecutor struct {
	executeFunc func(command string, workDir string) (string, string, error)
}

func (e *testCLIExecutor) Execute(command string, workDir string) (string, string, error) {
	return e.executeFunc(command, workDir)
}

func (e *testCLIExecutor) ExecuteWithWriter(command string, workDir string, writer io.Writer) (string, string, error) {
	return e.executeFunc(command, workDir)
}

func TestTaskfileRunner_ExpandVars_RejectsDangerousValues(t *testing.T) {
	tr := &DefaultTaskfileRunner{}

	tests := []struct {
		name     string
		vars     map[string]string
		template string
		want     string // should contain unexpanded placeholder
	}{
		{
			name:     "semicolon in var value",
			vars:     map[string]string{"NAME": "hello; rm -rf /"},
			template: "echo {{.NAME}}",
			want:     "{{.NAME}}", // not expanded
		},
		{
			name:     "backtick in var value",
			vars:     map[string]string{"CMD": "`whoami`"},
			template: "echo $CMD",
			want:     "$CMD", // not expanded
		},
		{
			name:     "subshell in var value",
			vars:     map[string]string{"X": "$(cat /etc/passwd)"},
			template: "echo {{.X}}",
			want:     "{{.X}}", // not expanded
		},
		{
			name:     "safe value is expanded",
			vars:     map[string]string{"NAME": "safe-value"},
			template: "echo {{.NAME}}",
			want:     "echo safe-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tr.expandVars(tt.template, tt.vars, nil)
			if !strings.Contains(result, tt.want) {
				t.Errorf("expandVars() = %q, want to contain %q", result, tt.want)
			}
		})
	}
}
