package integration

import (
	"os"
	"strings"
	"testing"
)

func TestNewTabManager_ReturnsNonNil(t *testing.T) {
	mgr := NewTabManager()
	if mgr == nil {
		t.Fatal("expected non-nil TabManager")
	}
}

func TestRenameTab_EmptyTaskID_ReturnsError(t *testing.T) {
	mgr := NewTabManager()
	err := mgr.RenameTab("", "some-branch")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestRenameTab_EmptyBranchName_ReturnsError(t *testing.T) {
	mgr := NewTabManager()
	err := mgr.RenameTab("TASK-00001", "")
	if err == nil {
		t.Fatal("expected error for empty branchName")
	}
}

func TestRenameTab_WritesANSISequence(t *testing.T) {
	mgr := NewTabManager()

	// Redirect stdout to capture the ANSI escape sequence.
	// Save original stdout.
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	renameErr := mgr.RenameTab("TASK-00001", "feat-branch")

	// Close writer and read output.
	_ = w.Close()
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	_ = r.Close()

	if renameErr != nil {
		t.Fatalf("unexpected error: %v", renameErr)
	}

	output := string(buf[:n])
	expected := "\033]0;TASK-00001 (feat-branch)\007"
	if output != expected {
		t.Errorf("output = %q, want %q", output, expected)
	}
}

func TestRestoreTab_WritesANSISequence(t *testing.T) {
	mgr := NewTabManager()

	// Redirect stdout to capture the ANSI escape sequence.
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	restoreErr := mgr.RestoreTab()

	_ = w.Close()
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	_ = r.Close()

	if restoreErr != nil {
		t.Fatalf("unexpected error: %v", restoreErr)
	}

	output := string(buf[:n])
	expected := "\033]0;\007"
	if output != expected {
		t.Errorf("output = %q, want %q", output, expected)
	}
}

func TestDetectEnvironment_Kiro(t *testing.T) {
	mgr := NewTabManager()

	// Save and restore environment.
	origKiro := os.Getenv("KIRO_PID")
	origVSCode := os.Getenv("VSCODE_PID")
	origTerm := os.Getenv("TERM_PROGRAM")
	defer func() {
		os.Setenv("KIRO_PID", origKiro)
		os.Setenv("VSCODE_PID", origVSCode)
		os.Setenv("TERM_PROGRAM", origTerm)
	}()

	os.Setenv("KIRO_PID", "12345")
	os.Unsetenv("VSCODE_PID")
	os.Unsetenv("TERM_PROGRAM")

	env := mgr.DetectEnvironment()
	if env != EnvKiro {
		t.Errorf("expected EnvKiro, got %q", env)
	}
}

func TestDetectEnvironment_VSCode(t *testing.T) {
	mgr := NewTabManager()

	origKiro := os.Getenv("KIRO_PID")
	origVSCode := os.Getenv("VSCODE_PID")
	origTerm := os.Getenv("TERM_PROGRAM")
	defer func() {
		os.Setenv("KIRO_PID", origKiro)
		os.Setenv("VSCODE_PID", origVSCode)
		os.Setenv("TERM_PROGRAM", origTerm)
	}()

	os.Unsetenv("KIRO_PID")
	os.Setenv("VSCODE_PID", "99999")
	os.Unsetenv("TERM_PROGRAM")

	env := mgr.DetectEnvironment()
	if env != EnvVSCode {
		t.Errorf("expected EnvVSCode, got %q", env)
	}
}

func TestDetectEnvironment_Terminal(t *testing.T) {
	mgr := NewTabManager()

	origKiro := os.Getenv("KIRO_PID")
	origVSCode := os.Getenv("VSCODE_PID")
	origTerm := os.Getenv("TERM_PROGRAM")
	defer func() {
		os.Setenv("KIRO_PID", origKiro)
		os.Setenv("VSCODE_PID", origVSCode)
		os.Setenv("TERM_PROGRAM", origTerm)
	}()

	os.Unsetenv("KIRO_PID")
	os.Unsetenv("VSCODE_PID")
	os.Setenv("TERM_PROGRAM", "iTerm2")

	env := mgr.DetectEnvironment()
	if env != EnvTerminal {
		t.Errorf("expected EnvTerminal, got %q", env)
	}
}

func TestDetectEnvironment_Unknown(t *testing.T) {
	mgr := NewTabManager()

	origKiro := os.Getenv("KIRO_PID")
	origVSCode := os.Getenv("VSCODE_PID")
	origTerm := os.Getenv("TERM_PROGRAM")
	defer func() {
		os.Setenv("KIRO_PID", origKiro)
		os.Setenv("VSCODE_PID", origVSCode)
		os.Setenv("TERM_PROGRAM", origTerm)
	}()

	os.Unsetenv("KIRO_PID")
	os.Unsetenv("VSCODE_PID")
	os.Unsetenv("TERM_PROGRAM")

	env := mgr.DetectEnvironment()
	if env != EnvUnknown {
		t.Errorf("expected EnvUnknown, got %q", env)
	}
}

func TestRenameTab_WriteError(t *testing.T) {
	// Test RenameTab when writing to stdout fails.
	mgr := NewTabManager()

	// Save original stdout.
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	// Create a closed pipe to simulate write failure.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	_ = r.Close()
	_ = w.Close() // Close the write end so writing fails
	os.Stdout = w

	err = mgr.RenameTab("TASK-00001", "feat-branch")
	if err == nil {
		t.Fatal("expected error when writing to closed stdout")
	}
	if !strings.Contains(err.Error(), "setting tab title") {
		t.Errorf("error = %q, want to contain 'setting tab title'", err.Error())
	}
}

func TestRestoreTab_WriteError(t *testing.T) {
	// Test RestoreTab when writing to stdout fails.
	mgr := NewTabManager()

	// Save original stdout.
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	// Create a closed pipe to simulate write failure.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	_ = r.Close()
	_ = w.Close()
	os.Stdout = w

	err = mgr.RestoreTab()
	if err == nil {
		t.Fatal("expected error when writing to closed stdout")
	}
	if !strings.Contains(err.Error(), "restoring tab title") {
		t.Errorf("error = %q, want to contain 'restoring tab title'", err.Error())
	}
}
