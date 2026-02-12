package integration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TaskfileTask represents a single task definition within a Taskfile.
type TaskfileTask struct {
	Name        string   `yaml:"-"`
	Description string   `yaml:"desc,omitempty"`
	Commands    []string `yaml:"cmds"`
	Deps        []string `yaml:"deps,omitempty"`
}

// Taskfile represents a parsed Taskfile.yaml.
type Taskfile struct {
	Version string                  `yaml:"version"`
	Tasks   map[string]TaskfileTask `yaml:"tasks"`
}

// TaskfileRunConfig holds all parameters needed to execute a Taskfile task.
type TaskfileRunConfig struct {
	TaskName string
	Args     []string
	TaskCtx  *TaskEnvContext // nil if no active task
	Dir      string          // directory containing Taskfile.yaml
	Stdout   io.Writer
	Stderr   io.Writer
}

// TaskfileRunner defines the interface for discovering and executing
// tasks from Taskfile.yaml.
type TaskfileRunner interface {
	// Discover parses Taskfile.yaml from the given directory and returns available tasks.
	Discover(dir string) (*Taskfile, error)
	// Run executes a named task from the Taskfile, injecting task context env vars.
	Run(config TaskfileRunConfig) (*CLIExecResult, error)
	// ListTasks returns the names and descriptions of all tasks in the Taskfile.
	ListTasks(dir string) ([]TaskfileTask, error)
}

// taskfileRunner implements TaskfileRunner using a CLIExecutor for command execution.
type taskfileRunner struct {
	executor CLIExecutor
}

// NewTaskfileRunner creates a new TaskfileRunner backed by the given CLIExecutor.
func NewTaskfileRunner(executor CLIExecutor) TaskfileRunner {
	return &taskfileRunner{executor: executor}
}

// Discover reads and parses Taskfile.yaml from the given directory.
func (r *taskfileRunner) Discover(dir string) (*Taskfile, error) {
	path := filepath.Join(dir, "Taskfile.yaml")

	data, err := os.ReadFile(path) //nolint:gosec // G304: reading Taskfile.yaml from user-specified directory
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("taskfile.yaml not found in %s: suggest creating one", dir)
		}
		return nil, fmt.Errorf("reading Taskfile.yaml: %w", err)
	}

	var tf Taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing Taskfile.yaml in %s: %w", dir, err)
	}

	// Set the Name field from the map key for each task.
	for name, task := range tf.Tasks {
		task.Name = name
		tf.Tasks[name] = task
	}

	return &tf, nil
}

// ListTasks discovers the Taskfile and returns all tasks with names populated.
func (r *taskfileRunner) ListTasks(dir string) ([]TaskfileTask, error) {
	tf, err := r.Discover(dir)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskfileTask, 0, len(tf.Tasks))
	for _, task := range tf.Tasks {
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// Run discovers the Taskfile, finds the named task, and executes its commands
// sequentially using the CLIExecutor.
func (r *taskfileRunner) Run(config TaskfileRunConfig) (*CLIExecResult, error) {
	tf, err := r.Discover(config.Dir)
	if err != nil {
		return nil, err
	}

	task, ok := tf.Tasks[config.TaskName]
	if !ok {
		available := make([]string, 0, len(tf.Tasks))
		for name := range tf.Tasks {
			available = append(available, name)
		}
		return nil, fmt.Errorf("task %q not found in Taskfile.yaml, available tasks: %v", config.TaskName, available)
	}

	if len(task.Commands) == 0 {
		return &CLIExecResult{ExitCode: 0}, nil
	}

	// Execute each command in the task sequentially.
	var lastResult *CLIExecResult
	for _, cmdStr := range task.Commands {
		result, execErr := r.executor.Exec(CLIExecConfig{
			CLI:     cmdStr,
			Args:    config.Args,
			TaskCtx: config.TaskCtx,
			Stdout:  config.Stdout,
			Stderr:  config.Stderr,
		})
		if execErr != nil {
			return result, fmt.Errorf("running task %q command %q: %w", config.TaskName, cmdStr, execErr)
		}
		lastResult = result
		if result.ExitCode != 0 {
			return result, nil
		}
	}

	return lastResult, nil
}
