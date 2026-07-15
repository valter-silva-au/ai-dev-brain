package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// dangerousVarPattern detects shell injection via variable expansion.
// Variable values should not contain shell metacharacters that could
// alter command semantics when substituted into command strings.
var dangerousVarPattern = regexp.MustCompile(`[;` + "`" + `$\(\)]`)

// TaskfileRunner executes tasks defined in Taskfile.yml
type TaskfileRunner interface {
	// Run executes a task from the Taskfile
	Run(taskName string) error

	// ListTasks returns available task names
	ListTasks() ([]string, error)
}

// Taskfile represents a Taskfile.yml structure
type Taskfile struct {
	Version string          `yaml:"version"`
	Tasks   map[string]Task `yaml:"tasks"`
}

// Task represents a task definition in Taskfile
type Task struct {
	Desc string            `yaml:"desc"`
	Cmds []string          `yaml:"cmds"`
	Deps []string          `yaml:"deps,omitempty"`
	Dir  string            `yaml:"dir,omitempty"`
	Env  map[string]string `yaml:"env,omitempty"`
}

// DefaultTaskfileRunner implements TaskfileRunner
type DefaultTaskfileRunner struct {
	taskfilePath string
	workDir      string
}

// NewTaskfileRunner creates a new Taskfile runner
func NewTaskfileRunner(workDir string) TaskfileRunner {
	return &DefaultTaskfileRunner{
		taskfilePath: filepath.Join(workDir, "Taskfile.yml"),
		workDir:      workDir,
	}
}

// Run executes a task from the Taskfile
func (tr *DefaultTaskfileRunner) Run(taskName string) error {
	// Check if Taskfile exists
	if _, err := os.Stat(tr.taskfilePath); os.IsNotExist(err) {
		return fmt.Errorf("Taskfile.yml not found in %s", tr.workDir)
	}

	// Read and parse Taskfile
	data, err := os.ReadFile(tr.taskfilePath)
	if err != nil {
		return fmt.Errorf("failed to read Taskfile: %w", err)
	}

	var taskfile Taskfile
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return fmt.Errorf("failed to parse Taskfile: %w", err)
	}

	// Find the task
	task, exists := taskfile.Tasks[taskName]
	if !exists {
		return fmt.Errorf("task '%s' not found in Taskfile", taskName)
	}

	// Execute dependencies first
	for _, dep := range task.Deps {
		fmt.Printf("Running dependency: %s\n", dep)
		if err := tr.Run(dep); err != nil {
			return fmt.Errorf("dependency '%s' failed: %w", dep, err)
		}
	}

	// Execute commands
	workDir := tr.workDir
	if task.Dir != "" {
		workDir = filepath.Join(tr.workDir, task.Dir)
	}

	// Validate environment variable values before execution
	for k, v := range task.Env {
		if dangerousVarPattern.MatchString(v) {
			return fmt.Errorf("task env variable %q contains dangerous shell metacharacters", k)
		}
	}

	for _, cmdStr := range task.Cmds {
		if strings.TrimSpace(cmdStr) == "" {
			continue
		}

		fmt.Printf("Running: %s\n", cmdStr)

		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = workDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Set environment variables
		cmd.Env = os.Environ()
		for k, v := range task.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}

	return nil
}

// ListTasks returns available task names
func (tr *DefaultTaskfileRunner) ListTasks() ([]string, error) {
	// Check if Taskfile exists
	if _, err := os.Stat(tr.taskfilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Taskfile.yml not found in %s", tr.workDir)
	}

	// Read and parse Taskfile
	data, err := os.ReadFile(tr.taskfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Taskfile: %w", err)
	}

	var taskfile Taskfile
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return nil, fmt.Errorf("failed to parse Taskfile: %w", err)
	}

	// Collect task names
	var tasks []string
	for name := range taskfile.Tasks {
		tasks = append(tasks, name)
	}

	return tasks, nil
}

// ValidateTaskfile validates the Taskfile syntax
func ValidateTaskfile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read Taskfile: %w", err)
	}

	var taskfile Taskfile
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return fmt.Errorf("invalid Taskfile syntax: %w", err)
	}

	// Validate version
	if taskfile.Version == "" {
		return fmt.Errorf("Taskfile version is required")
	}

	// Validate tasks
	if len(taskfile.Tasks) == 0 {
		return fmt.Errorf("no tasks defined in Taskfile")
	}

	// Check for circular dependencies
	visited := make(map[string]bool)
	for taskName := range taskfile.Tasks {
		if err := checkCircularDeps(taskfile.Tasks, taskName, visited, []string{}); err != nil {
			return err
		}
	}

	return nil
}

// checkCircularDeps checks for circular dependencies in tasks
func checkCircularDeps(tasks map[string]Task, taskName string, visited map[string]bool, path []string) error {
	// Check if we've seen this task in the current path
	for _, p := range path {
		if p == taskName {
			return fmt.Errorf("circular dependency detected: %s", strings.Join(append(path, taskName), " -> "))
		}
	}

	// Mark as visited
	if visited[taskName] {
		return nil
	}
	visited[taskName] = true

	// Get task
	task, exists := tasks[taskName]
	if !exists {
		return fmt.Errorf("task '%s' referenced but not defined", taskName)
	}

	// Check dependencies
	newPath := append(path, taskName)
	for _, dep := range task.Deps {
		if err := checkCircularDeps(tasks, dep, visited, newPath); err != nil {
			return err
		}
	}

	return nil
}
