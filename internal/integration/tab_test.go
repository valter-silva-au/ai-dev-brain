package integration

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewTabManager(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)
	if manager == nil {
		t.Fatal("NewTabManager() returned nil")
	}

	// Test with nil writer (should use os.Stdout)
	manager = NewTabManager(nil)
	if manager == nil {
		t.Fatal("NewTabManager() with nil writer returned nil")
	}
}

func TestSetTabName(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	err := manager.SetTabName("Test Tab")
	if err != nil {
		t.Fatalf("SetTabName() failed: %v", err)
	}

	output := buf.String()

	// Check for ANSI OSC 0 sequence
	// Format: \033]0;text\007
	if !strings.Contains(output, "\033]0;") {
		t.Error("Output does not contain ANSI OSC 0 sequence start")
	}
	if !strings.Contains(output, "Test Tab") {
		t.Error("Output does not contain tab name")
	}
	if !strings.Contains(output, "\007") {
		t.Error("Output does not contain BEL terminator")
	}

	// Verify exact format
	expected := "\033]0;Test Tab\007"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}
}

func TestSetTabNameWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		tabName string
	}{
		{
			name:    "With spaces",
			tabName: "My Tab Name",
		},
		{
			name:    "With numbers",
			tabName: "Task-123",
		},
		{
			name:    "With symbols",
			tabName: "Project: ABC",
		},
		{
			name:    "With slashes",
			tabName: "feature/branch",
		},
		{
			name:    "With underscores",
			tabName: "my_task_01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			manager := NewTabManager(&buf)

			err := manager.SetTabName(tt.tabName)
			if err != nil {
				t.Fatalf("SetTabName() failed: %v", err)
			}

			output := buf.String()
			expected := "\033]0;" + tt.tabName + "\007"
			if output != expected {
				t.Errorf("Expected %q, got %q", expected, output)
			}
		})
	}
}

func TestSetTabNameEmptyName(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	err := manager.SetTabName("")
	if err == nil {
		t.Error("SetTabName() with empty name should return error")
	}
	if !strings.Contains(err.Error(), "tab name cannot be empty") {
		t.Errorf("Expected error to contain 'tab name cannot be empty', got: %v", err)
	}

	// Buffer should be empty
	if buf.Len() > 0 {
		t.Errorf("Buffer should be empty after error, got: %q", buf.String())
	}
}

func TestSetTabNameWithWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	manager := NewTabManager(&buf1)

	// Set tab name with custom writer
	err := manager.SetTabNameWithWriter("Custom Tab", &buf2)
	if err != nil {
		t.Fatalf("SetTabNameWithWriter() failed: %v", err)
	}

	// Should write to buf2, not buf1
	if buf1.Len() > 0 {
		t.Error("SetTabNameWithWriter() should not write to default writer")
	}

	expected := "\033]0;Custom Tab\007"
	if buf2.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf2.String())
	}
}

func TestSetTabNameWithWriterNilWriter(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	// Should use manager's writer when nil is passed
	err := manager.SetTabNameWithWriter("Fallback Tab", nil)
	if err != nil {
		t.Fatalf("SetTabNameWithWriter() with nil writer failed: %v", err)
	}

	expected := "\033]0;Fallback Tab\007"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestSetTabNameWithWriterEmptyName(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	err := manager.SetTabNameWithWriter("", &buf)
	if err == nil {
		t.Error("SetTabNameWithWriter() with empty name should return error")
	}
}

func TestSetTabNameMultipleCalls(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	// Make multiple calls
	names := []string{"Tab 1", "Tab 2", "Tab 3"}
	for _, name := range names {
		if err := manager.SetTabName(name); err != nil {
			t.Fatalf("SetTabName(%q) failed: %v", name, err)
		}
	}

	output := buf.String()

	// Each name should be in the output with proper sequences
	for _, name := range names {
		if !strings.Contains(output, name) {
			t.Errorf("Output does not contain tab name: %q", name)
		}
	}

	// Should have 3 sequences
	count := strings.Count(output, "\033]0;")
	if count != 3 {
		t.Errorf("Expected 3 sequences, got %d", count)
	}
}

func TestTabManagerANSISequenceFormat(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	err := manager.SetTabName("Format Test")
	if err != nil {
		t.Fatalf("SetTabName() failed: %v", err)
	}

	output := buf.String()

	// Verify the exact sequence format
	// ESC ] 0 ; text BEL
	// \033 = ESC (octal 033)
	// ] = right bracket
	// 0 = OSC command 0 (set icon and title)
	// ; = separator
	// text = the tab name
	// \007 = BEL (octal 007)

	if len(output) < 7 { // Minimum length: \033]0;\007
		t.Fatalf("Output too short: %q", output)
	}

	// Check ESC character
	if output[0] != '\033' {
		t.Errorf("First character should be ESC (\\033), got: %q", output[0])
	}

	// Check sequence start
	if !strings.HasPrefix(output, "\033]0;") {
		t.Errorf("Sequence should start with \\033]0;, got: %q", output[:4])
	}

	// Check BEL terminator
	if output[len(output)-1] != '\007' {
		t.Errorf("Last character should be BEL (\\007), got: %q", output[len(output)-1])
	}
}

func TestTabManagerWithLongName(t *testing.T) {
	var buf bytes.Buffer
	manager := NewTabManager(&buf)

	// Test with a very long name
	longName := strings.Repeat("A", 200)
	err := manager.SetTabName(longName)
	if err != nil {
		t.Fatalf("SetTabName() with long name failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, longName) {
		t.Error("Output does not contain the full long name")
	}
}
