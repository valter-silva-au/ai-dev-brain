package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

type runMock struct {
	discoverFn  func(dir string) (*integration.Taskfile, error)
	runFn       func(config integration.TaskfileRunConfig) (*integration.CLIExecResult, error)
	listTasksFn func(dir string) ([]integration.TaskfileTask, error)
}

func (m *runMock) Discover(dir string) (*integration.Taskfile, error) {
	if m.discoverFn != nil {
		return m.discoverFn(dir)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *runMock) Run(config integration.TaskfileRunConfig) (*integration.CLIExecResult, error) {
	if m.runFn != nil {
		return m.runFn(config)
	}
	return &integration.CLIExecResult{}, nil
}

func (m *runMock) ListTasks(dir string) ([]integration.TaskfileTask, error) {
	if m.listTasksFn != nil {
		return m.listTasksFn(dir)
	}
	return nil, nil
}

func TestRunCmd_NilRunner(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()
	Runner = nil

	err := runCmd.RunE(runCmd, []string{"test"})
	if err == nil {
		t.Fatal("expected error when Runner is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunCmd_NoArgs(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()
	Runner = &runMock{}

	err := runCmd.RunE(runCmd, []string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "task name required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunCmd_ListEmpty(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()

	Runner = &runMock{
		listTasksFn: func(dir string) ([]integration.TaskfileTask, error) {
			return nil, nil
		},
	}

	err := runCmd.RunE(runCmd, []string{"--list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmd_ListWithTasks(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()

	Runner = &runMock{
		listTasksFn: func(dir string) ([]integration.TaskfileTask, error) {
			return []integration.TaskfileTask{
				{Name: "test", Description: "Run tests"},
				{Name: "build", Description: ""},
			}, nil
		},
	}

	err := runCmd.RunE(runCmd, []string{"--list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmd_ListShorthand(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()

	Runner = &runMock{
		listTasksFn: func(dir string) ([]integration.TaskfileTask, error) {
			return []integration.TaskfileTask{
				{Name: "check", Description: "Run checks"},
			}, nil
		},
	}

	err := runCmd.RunE(runCmd, []string{"-l"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmd_ListError(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()

	Runner = &runMock{
		listTasksFn: func(dir string) ([]integration.TaskfileTask, error) {
			return nil, fmt.Errorf("Taskfile.yaml not found")
		},
	}

	err := runCmd.RunE(runCmd, []string{"--list"})
	if err == nil {
		t.Fatal("expected error from ListTasks")
	}
	if !strings.Contains(err.Error(), "listing tasks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunCmd_RunSuccess(t *testing.T) {
	orig := Runner
	origExit := osExit
	defer func() {
		Runner = orig
		osExit = origExit
	}()

	Runner = &runMock{
		runFn: func(config integration.TaskfileRunConfig) (*integration.CLIExecResult, error) {
			if config.TaskName != "test" {
				t.Errorf("expected task name 'test', got %q", config.TaskName)
			}
			return &integration.CLIExecResult{ExitCode: 0}, nil
		},
	}

	err := runCmd.RunE(runCmd, []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmd_RunError(t *testing.T) {
	orig := Runner
	defer func() { Runner = orig }()

	Runner = &runMock{
		runFn: func(config integration.TaskfileRunConfig) (*integration.CLIExecResult, error) {
			return nil, fmt.Errorf("task not found")
		},
	}

	err := runCmd.RunE(runCmd, []string{"missing"})
	if err == nil {
		t.Fatal("expected error from Run")
	}
	if !strings.Contains(err.Error(), "run missing") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunCmd_NonZeroExitCode(t *testing.T) {
	orig := Runner
	origExit := osExit
	defer func() {
		Runner = orig
		osExit = origExit
	}()

	var exitCode int
	osExit = func(code int) { exitCode = code }

	Runner = &runMock{
		runFn: func(config integration.TaskfileRunConfig) (*integration.CLIExecResult, error) {
			return &integration.CLIExecResult{ExitCode: 2}, nil
		},
	}

	_ = runCmd.RunE(runCmd, []string{"failing-task"})
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
}

func TestRunCmd_HelpFlag(t *testing.T) {
	err := runCmd.RunE(runCmd, []string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
