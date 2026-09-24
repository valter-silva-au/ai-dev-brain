package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendContextSection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	contextPath := filepath.Join(tmpDir, "context.md")

	t.Run("Append to new file", func(t *testing.T) {
		err := AppendContextSection(contextPath, "Test Section", "This is test content")
		if err != nil {
			t.Fatalf("AppendContextSection() error = %v", err)
		}

		content, err := os.ReadFile(contextPath)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "## Test Section") {
			t.Errorf("Context file doesn't contain section title")
		}
		if !strings.Contains(contentStr, "This is test content") {
			t.Errorf("Context file doesn't contain section content")
		}
	})

	t.Run("Append to existing file", func(t *testing.T) {
		err := AppendContextSection(contextPath, "Another Section", "More content")
		if err != nil {
			t.Fatalf("AppendContextSection() error = %v", err)
		}

		content, err := os.ReadFile(contextPath)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "## Test Section") {
			t.Errorf("Context file lost previous section")
		}
		if !strings.Contains(contentStr, "## Another Section") {
			t.Errorf("Context file doesn't contain new section")
		}
		if !strings.Contains(contentStr, "More content") {
			t.Errorf("Context file doesn't contain new content")
		}
	})
}

func TestAppendTimestampedEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "log.txt")

	t.Run("Append entry", func(t *testing.T) {
		err := AppendTimestampedEntry(logPath, "First entry")
		if err != nil {
			t.Fatalf("AppendTimestampedEntry() error = %v", err)
		}

		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "First entry") {
			t.Errorf("Log file doesn't contain entry")
		}
		if !strings.Contains(contentStr, "[202") {
			t.Errorf("Log file doesn't contain timestamp")
		}
	})

	t.Run("Append multiple entries", func(t *testing.T) {
		err := AppendTimestampedEntry(logPath, "Second entry")
		if err != nil {
			t.Fatalf("AppendTimestampedEntry() error = %v", err)
		}

		err = AppendTimestampedEntry(logPath, "Third entry")
		if err != nil {
			t.Fatalf("AppendTimestampedEntry() error = %v", err)
		}

		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "First entry") {
			t.Errorf("Log file lost first entry")
		}
		if !strings.Contains(contentStr, "Second entry") {
			t.Errorf("Log file doesn't contain second entry")
		}
		if !strings.Contains(contentStr, "Third entry") {
			t.Errorf("Log file doesn't contain third entry")
		}
	})
}

func TestUpdateContextFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "TASK-001")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	t.Run("Update context", func(t *testing.T) {
		err := UpdateContextFile(taskDir, "Session summary content")
		if err != nil {
			t.Fatalf("UpdateContextFile() error = %v", err)
		}

		contextPath := filepath.Join(taskDir, "context.md")
		content, err := os.ReadFile(contextPath)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "Session Update") {
			t.Errorf("Context file doesn't contain section title")
		}
		if !strings.Contains(contentStr, "Session summary content") {
			t.Errorf("Context file doesn't contain content")
		}
	})
}

func TestCaptureTranscript(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "TASK-001")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	t.Run("Capture transcript", func(t *testing.T) {
		sessionID := "sess-123"
		transcript := "User: Do something\nAssistant: Done!"

		err := CaptureTranscript(taskDir, sessionID, transcript)
		if err != nil {
			t.Fatalf("CaptureTranscript() error = %v", err)
		}

		transcriptPath := filepath.Join(taskDir, "transcript_sess-123.md")
		content, err := os.ReadFile(transcriptPath)
		if err != nil {
			t.Fatalf("Failed to read transcript file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "Session Transcript") {
			t.Errorf("Transcript doesn't contain header")
		}
		if !strings.Contains(contentStr, sessionID) {
			t.Errorf("Transcript doesn't contain session ID")
		}
		if !strings.Contains(contentStr, "User: Do something") {
			t.Errorf("Transcript doesn't contain conversation")
		}
		if !strings.Contains(contentStr, "Assistant: Done!") {
			t.Errorf("Transcript doesn't contain response")
		}
	})
}
