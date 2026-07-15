package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// TaskfileTask represents a task in a Taskfile.yaml
type TaskfileTask struct {
	Name        string            `yaml:"-"` // Populated from map key
	Description string            `yaml:"desc"`
	Summary     string            `yaml:"summary"`
	Commands    []string          `yaml:"cmds"`
	Deps        []string          `yaml:"deps"`
	Vars        map[string]string `yaml:"vars"`
	Env         map[string]string `yaml:"env"`
	Dir         string            `yaml:"dir"`
	Silent      bool              `yaml:"silent"`
}

// Taskfile represents the structure of a Taskfile.yaml
type Taskfile struct {
	Version string                  `yaml:"version"`
	Tasks   map[string]TaskfileTask `yaml:"tasks"`
	Vars    map[string]string       `yaml:"vars"`
	Env     map[string]string       `yaml:"env"`
}

// TaskfileRunner discovers and runs Taskfile.yaml tasks
type TaskfileRunner interface {
	// DiscoverTaskfile finds Taskfile.yaml in the given directory or parent directories
	DiscoverTaskfile(startDir string) (string, error)

	// LoadTaskfile loads and parses a Taskfile.yaml
	LoadTaskfile(path string) (*Taskfile, error)

	// ListTasks returns all available tasks in a Taskfile
	ListTasks(taskfile *Taskfile) []TaskfileTask

	// RunTask executes a task using the CLIExecutor
	RunTask(taskfilePath string, taskName string, executor CLIExecutor) error
}

// DefaultTaskfileRunner implements TaskfileRunner
type DefaultTaskfileRunner struct{}

// NewTaskfileRunner creates a new Taskfile runner
func NewTaskfileRunner() TaskfileRunner {
	return &DefaultTaskfileRunner{}
}

// DiscoverTaskfile finds Taskfile.yaml by walking up the directory tree
func (tr *DefaultTaskfileRunner) DiscoverTaskfile(startDir string) (string, error) {
	// Normalize the starting directory
	absStartDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	currentDir := absStartDir

	// Walk up the directory tree
	for {
		// Check for Taskfile.yaml
		taskfilePath := filepath.Join(currentDir, "Taskfile.yaml")
		if _, err := os.Stat(taskfilePath); err == nil {
			return taskfilePath, nil
		}

		// Check for Taskfile.yml (alternative extension)
		taskfilePath = filepath.Join(currentDir, "Taskfile.yml")
		if _, err := os.Stat(taskfilePath); err == nil {
			return taskfilePath, nil
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached the root
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("Taskfile.yaml not found in %s or any parent directory", absStartDir)
}

// LoadTaskfile loads and parses a Taskfile.yaml
func (tr *DefaultTaskfileRunner) LoadTaskfile(path string) (*Taskfile, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read Taskfile: %w", err)
	}

	// Parse YAML
	var taskfile Taskfile
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return nil, fmt.Errorf("failed to parse Taskfile YAML: %w", err)
	}

	// Populate task names from map keys
	for name, task := range taskfile.Tasks {
		task.Name = name
		taskfile.Tasks[name] = task
	}

	return &taskfile, nil
}

// ListTasks returns all available tasks in a Taskfile
func (tr *DefaultTaskfileRunner) ListTasks(taskfile *Taskfile) []TaskfileTask {
	tasks := make([]TaskfileTask, 0, len(taskfile.Tasks))
	for _, task := range taskfile.Tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// RunTask executes a task using the CLIExecutor
func (tr *DefaultTaskfileRunner) RunTask(taskfilePath string, taskName string, executor CLIExecutor) error {
	// Load the Taskfile
	taskfile, err := tr.LoadTaskfile(taskfilePath)
	if err != nil {
		return err
	}

	// Find the task
	task, exists := taskfile.Tasks[taskName]
	if !exists {
		return fmt.Errorf("task '%s' not found in Taskfile", taskName)
	}

	// Get the directory of the Taskfile
	taskfileDir := filepath.Dir(taskfilePath)

	// Determine working directory
	workDir := taskfileDir
	if task.Dir != "" {
		// Task has a custom directory
		if filepath.IsAbs(task.Dir) {
			workDir = task.Dir
		} else {
			workDir = filepath.Join(taskfileDir, task.Dir)
		}
	}

	// Run dependencies first
	for _, dep := range task.Deps {
		if err := tr.RunTask(taskfilePath, dep, executor); err != nil {
			return fmt.Errorf("dependency '%s' failed: %w", dep, err)
		}
	}

	// Execute commands
	for i, cmd := range task.Commands {
		// Skip empty commands
		if strings.TrimSpace(cmd) == "" {
			continue
		}

		// Expand variables in the command
		expandedCmd := tr.expandVars(cmd, taskfile.Vars, task.Vars)

		// Execute the command
		stdout, stderr, err := executor.Execute(expandedCmd, workDir)

		// Print output unless silent
		if !task.Silent {
			if stdout != "" {
				fmt.Print(stdout)
			}
			if stderr != "" {
				fmt.Fprint(os.Stderr, stderr)
			}
		}

		// Check for errors
		if err != nil {
			return fmt.Errorf("command %d failed: %w", i+1, err)
		}
	}

	return nil
}

// dangerousVarValue detects shell injection in variable values
var dangerousVarValue = regexp.MustCompile(`[;` + "`" + `$\(\)]`)

// expandVars expands variables in a command string
// Variables are referenced as {{.VAR_NAME}} or $VAR_NAME
// Variable values are validated to prevent shell injection.
func (tr *DefaultTaskfileRunner) expandVars(cmd string, globalVars, taskVars map[string]string) string {
	result := cmd

	// Merge vars (task vars override global vars)
	allVars := make(map[string]string)
	for k, v := range globalVars {
		allVars[k] = v
	}
	for k, v := range taskVars {
		allVars[k] = v
	}

	// Replace {{.VAR}} style variables (with validation)
	for key, value := range allVars {
		if dangerousVarValue.MatchString(value) {
			// Skip injection — replace with sanitized placeholder
			fmt.Fprintf(os.Stderr, "Warning: skipping variable %q — value contains shell metacharacters\n", key)
			continue
		}
		placeholder := fmt.Sprintf("{{.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Replace $VAR style variables (with validation)
	for key, value := range allVars {
		if dangerousVarValue.MatchString(value) {
			continue
		}
		placeholder := fmt.Sprintf("$%s", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}
