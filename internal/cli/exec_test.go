package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

type execMock struct {
	execFn        func(config integration.CLIExecConfig) (*integration.CLIExecResult, error)
	listAliasesFn func(aliases []integration.CLIAlias) []string
}

func (m *execMock) Exec(config integration.CLIExecConfig) (*integration.CLIExecResult, error) {
	if m.execFn != nil {
		return m.execFn(config)
	}
	return &integration.CLIExecResult{}, nil
}

func (m *execMock) ResolveAlias(name string, aliases []integration.CLIAlias) (string, []string, bool) {
	return name, nil, false
}

func (m *execMock) BuildEnv(base []string, taskCtx *integration.TaskEnvContext) []string {
	return base
}

func (m *execMock) ListAliases(aliases []integration.CLIAlias) []string {
	if m.listAliasesFn != nil {
		return m.listAliasesFn(aliases)
	}
	return nil
}

func (m *execMock) LogFailure(taskCtx *integration.TaskEnvContext, cli string, args []string, result *integration.CLIExecResult) error {
	return nil
}

func TestExecCmd_NilExecutor(t *testing.T) {
	origExec := Executor
	defer func() { Executor = origExec }()
	Executor = nil

	err := execCmd.RunE(execCmd, []string{"git", "status"})
	if err == nil {
		t.Fatal("expected error when Executor is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecCmd_NoArgs_NoAliases(t *testing.T) {
	origExec := Executor
	defer func() { Executor = origExec }()

	Executor = &execMock{
		listAliasesFn: func(aliases []integration.CLIAlias) []string {
			return nil
		},
	}

	err := execCmd.RunE(execCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecCmd_NoArgs_WithAliases(t *testing.T) {
	origExec := Executor
	origAliases := ExecAliases
	defer func() {
		Executor = origExec
		ExecAliases = origAliases
	}()

	ExecAliases = []integration.CLIAlias{
		{Name: "cc", Command: "claude"},
	}
	Executor = &execMock{
		listAliasesFn: func(aliases []integration.CLIAlias) []string {
			return []string{"cc -> claude"}
		},
	}

	err := execCmd.RunE(execCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecCmd_ExecSuccess(t *testing.T) {
	origExec := Executor
	origExit := osExit
	defer func() {
		Executor = origExec
		osExit = origExit
	}()

	Executor = &execMock{
		execFn: func(config integration.CLIExecConfig) (*integration.CLIExecResult, error) {
			if config.CLI != "git" {
				t.Errorf("expected CLI 'git', got %q", config.CLI)
			}
			return &integration.CLIExecResult{ExitCode: 0}, nil
		},
	}

	err := execCmd.RunE(execCmd, []string{"git", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecCmd_ExecError(t *testing.T) {
	origExec := Executor
	defer func() { Executor = origExec }()

	Executor = &execMock{
		execFn: func(config integration.CLIExecConfig) (*integration.CLIExecResult, error) {
			return nil, fmt.Errorf("command not found")
		},
	}

	err := execCmd.RunE(execCmd, []string{"bad"})
	if err == nil {
		t.Fatal("expected error from Exec")
	}
	if !strings.Contains(err.Error(), "exec bad") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecCmd_NonZeroExitCode(t *testing.T) {
	origExec := Executor
	origExit := osExit
	defer func() {
		Executor = origExec
		osExit = origExit
	}()

	var exitCode int
	osExit = func(code int) { exitCode = code }

	Executor = &execMock{
		execFn: func(config integration.CLIExecConfig) (*integration.CLIExecResult, error) {
			return &integration.CLIExecResult{ExitCode: 1}, nil
		},
	}

	_ = execCmd.RunE(execCmd, []string{"git", "bad"})
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestExecCmd_HelpFlag(t *testing.T) {
	// --help should not error (Cobra handles it).
	err := execCmd.RunE(execCmd, []string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
