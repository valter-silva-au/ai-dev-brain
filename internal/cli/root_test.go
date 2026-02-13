package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetVersionInfo(t *testing.T) {
	// Save originals.
	origVersion := appVersion
	origCommit := appCommit
	origDate := appDate
	defer func() {
		appVersion = origVersion
		appCommit = origCommit
		appDate = origDate
	}()

	SetVersionInfo("1.2.3", "abc1234", "2026-02-13")

	if appVersion != "1.2.3" {
		t.Errorf("appVersion = %q, want 1.2.3", appVersion)
	}
	if appCommit != "abc1234" {
		t.Errorf("appCommit = %q, want abc1234", appCommit)
	}
	if appDate != "2026-02-13" {
		t.Errorf("appDate = %q, want 2026-02-13", appDate)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"nonexistent-command"})

	err := Execute()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecute_VersionSubcommand(t *testing.T) {
	origVersion := appVersion
	origCommit := appCommit
	origDate := appDate
	defer func() {
		appVersion = origVersion
		appCommit = origCommit
		appDate = origDate
	}()
	appVersion = "test-ver"
	appCommit = "test-commit"
	appDate = "test-date"

	// The version command uses fmt.Printf (writes to os.Stdout, not cmd.Out),
	// so we just verify Execute() succeeds without error.
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetArgs([]string{"version"})

	err := Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVersionCommand_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("version command not registered on root")
	}
}
