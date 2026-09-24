package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	// Save original version info
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date

	// Set test version info
	Version = "test-version"
	Commit = "test-commit"
	Date = "test-date"

	defer func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	}()

	// Create version command
	cmd := NewVersionCmd()

	// Capture output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Execute command
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify output
	output := buf.String()
	if !strings.Contains(output, "test-version") {
		t.Errorf("output does not contain version: %s", output)
	}
	if !strings.Contains(output, "test-commit") {
		t.Errorf("output does not contain commit: %s", output)
	}
	if !strings.Contains(output, "test-date") {
		t.Errorf("output does not contain date: %s", output)
	}
}
