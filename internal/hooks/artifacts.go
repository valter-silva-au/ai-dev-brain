package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AppendContextSection appends a section to a context file
func AppendContextSection(contextPath, sectionTitle, content string) error {
	f, err := os.OpenFile(contextPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open context file: %w", err)
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	section := fmt.Sprintf("\n## %s (Updated: %s)\n\n%s\n", sectionTitle, timestamp, content)

	if _, err := f.WriteString(section); err != nil {
		return fmt.Errorf("failed to append context section: %w", err)
	}

	return nil
}

// AppendTimestampedEntry appends a timestamped entry to a file
func AppendTimestampedEntry(filePath, entry string) error {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("[%s] %s\n", timestamp, entry)

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to append entry: %w", err)
	}

	return nil
}

// UpdateContextFile updates the context.md file for a task
func UpdateContextFile(taskDir, content string) error {
	contextPath := filepath.Join(taskDir, "context.md")
	return AppendContextSection(contextPath, "Session Update", content)
}

// CaptureTranscript saves the session transcript
func CaptureTranscript(taskDir, sessionID, transcript string) error {
	transcriptPath := filepath.Join(taskDir, fmt.Sprintf("transcript_%s.md", sessionID))

	f, err := os.Create(transcriptPath)
	if err != nil {
		return fmt.Errorf("failed to create transcript file: %w", err)
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	header := fmt.Sprintf("# Session Transcript\n\nSession ID: %s\nCaptured: %s\n\n---\n\n", sessionID, timestamp)

	if _, err := f.WriteString(header + transcript); err != nil {
		return fmt.Errorf("failed to write transcript: %w", err)
	}

	return nil
}
