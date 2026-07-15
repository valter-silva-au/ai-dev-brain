package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileChannel_SendMessage(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	msg := &FileChannelMessage{
		Type:   "command",
		Source: "user",
		Target: "system",
		Body:   "This is a test message",
	}

	err := fc.SendMessage(msg)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify the message file was created
	outboxDir := filepath.Join(tmpDir, "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err != nil {
		t.Fatalf("Failed to read outbox: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 file in outbox, got %d", len(entries))
	}

	// Verify message has ID and timestamp
	if msg.ID == "" {
		t.Error("Expected message to have ID")
	}

	if msg.Timestamp.IsZero() {
		t.Error("Expected message to have timestamp")
	}

	if msg.Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", msg.Status)
	}
}

func TestFileChannel_SendReceive(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Send a message to outbox
	msg := &FileChannelMessage{
		ID:     "test-msg-1",
		Type:   "command",
		Source: "user",
		Target: "system",
		Status: "pending",
		Body:   "Test message body",
	}

	err := fc.SendMessage(msg)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Move the message to inbox for testing receive
	outboxPath := filepath.Join(tmpDir, "outbox", "test-msg-1.md")
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0o755)
	inboxPath := filepath.Join(inboxDir, "test-msg-1.md")
	os.Rename(outboxPath, inboxPath)

	// Receive the message
	receivedMsg, err := fc.ReceiveMessage()
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	if receivedMsg == nil {
		t.Fatal("Expected to receive a message")
	}

	if receivedMsg.ID != msg.ID {
		t.Errorf("Expected ID '%s', got '%s'", msg.ID, receivedMsg.ID)
	}

	if receivedMsg.Type != msg.Type {
		t.Errorf("Expected type '%s', got '%s'", msg.Type, receivedMsg.Type)
	}

	if receivedMsg.Body != msg.Body {
		t.Errorf("Expected body '%s', got '%s'", msg.Body, receivedMsg.Body)
	}
}

func TestFileChannel_ListInbox(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Create inbox directory and add some messages
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0o755)

	messages := []*FileChannelMessage{
		{
			ID:        "msg-1",
			Type:      "command",
			Timestamp: time.Now().UTC(),
			Source:    "user",
			Target:    "system",
			Status:    "pending",
			Body:      "Message 1",
		},
		{
			ID:        "msg-2",
			Type:      "response",
			Timestamp: time.Now().UTC(),
			Source:    "system",
			Target:    "user",
			Status:    "completed",
			Body:      "Message 2",
		},
	}

	// Write messages directly to inbox
	for _, msg := range messages {
		path := filepath.Join(inboxDir, msg.ID+".md")
		fsfc := fc.(*FileSystemFileChannel)
		fsfc.writeMessage(path, msg)
	}

	// List inbox
	listed, err := fc.ListInbox()
	if err != nil {
		t.Fatalf("Failed to list inbox: %v", err)
	}

	if len(listed) != 2 {
		t.Errorf("Expected 2 messages in inbox, got %d", len(listed))
	}

	// Verify message contents
	foundIDs := make(map[string]bool)
	for _, msg := range listed {
		foundIDs[msg.ID] = true
	}

	if !foundIDs["msg-1"] || !foundIDs["msg-2"] {
		t.Error("Expected to find both msg-1 and msg-2")
	}
}

func TestFileChannel_ListOutbox(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Send some messages
	for i := 1; i <= 3; i++ {
		msg := &FileChannelMessage{
			Type:   "command",
			Source: "user",
			Target: "system",
			Body:   "Test message",
		}
		fc.SendMessage(msg)
	}

	// List outbox
	messages, err := fc.ListOutbox()
	if err != nil {
		t.Fatalf("Failed to list outbox: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages in outbox, got %d", len(messages))
	}
}

func TestFileChannel_ArchiveMessage(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Send a message
	msg := &FileChannelMessage{
		ID:     "archive-test",
		Type:   "command",
		Source: "user",
		Target: "system",
		Body:   "Test message",
	}
	fc.SendMessage(msg)

	// Archive the message
	err := fc.ArchiveMessage("archive-test", "outbox")
	if err != nil {
		t.Fatalf("Failed to archive message: %v", err)
	}

	// Verify message is in archive
	archivePath := filepath.Join(tmpDir, "archive", "archive-test.md")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Error("Message was not archived")
	}

	// Verify message is not in outbox
	outboxPath := filepath.Join(tmpDir, "outbox", "archive-test.md")
	if _, err := os.Stat(outboxPath); !os.IsNotExist(err) {
		t.Error("Message still exists in outbox")
	}
}

func TestFileChannel_DeleteMessage(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Send a message
	msg := &FileChannelMessage{
		ID:     "delete-test",
		Type:   "command",
		Source: "user",
		Target: "system",
		Body:   "Test message",
	}
	fc.SendMessage(msg)

	// Delete the message
	err := fc.DeleteMessage("delete-test", "outbox")
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify message is deleted
	outboxPath := filepath.Join(tmpDir, "outbox", "delete-test.md")
	if _, err := os.Stat(outboxPath); !os.IsNotExist(err) {
		t.Error("Message was not deleted")
	}
}

func TestFileChannel_MessageFormat(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	msg := &FileChannelMessage{
		ID:        "format-test",
		Type:      "command",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Source:    "user",
		Target:    "system",
		Status:    "pending",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Body: "This is the message body\nwith multiple lines",
	}

	fc.SendMessage(msg)

	// Read the file directly and verify format
	filePath := filepath.Join(tmpDir, "outbox", "format-test.md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read message file: %v", err)
	}

	contentStr := string(content)

	// Check frontmatter delimiters
	if !strings.HasPrefix(contentStr, "---\n") {
		t.Error("Expected file to start with YAML frontmatter")
	}

	if !strings.Contains(contentStr, "\n---\n") {
		t.Error("Expected frontmatter end delimiter")
	}

	// Check body is present
	if !strings.Contains(contentStr, "This is the message body") {
		t.Error("Expected message body in file")
	}

	// Read back and verify
	fsfc := fc.(*FileSystemFileChannel)
	readMsg, err := fsfc.readMessage(filePath)
	if err != nil {
		t.Fatalf("Failed to read message back: %v", err)
	}

	if readMsg.ID != msg.ID {
		t.Errorf("Expected ID '%s', got '%s'", msg.ID, readMsg.ID)
	}

	if readMsg.Body != msg.Body {
		t.Errorf("Expected body '%s', got '%s'", msg.Body, readMsg.Body)
	}

	if readMsg.Metadata["key1"] != "value1" {
		t.Error("Expected metadata to be preserved")
	}
}

func TestFileChannel_EmptyInbox(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	// Try to receive from empty inbox
	msg, err := fc.ReceiveMessage()
	if err != nil {
		t.Errorf("Expected no error for empty inbox, got: %v", err)
	}

	if msg != nil {
		t.Error("Expected nil message for empty inbox")
	}

	// List empty inbox
	messages, err := fc.ListInbox()
	if err != nil {
		t.Errorf("Expected no error listing empty inbox, got: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}
}

func TestFileChannel_InvalidBox(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)

	err := fc.ArchiveMessage("test", "invalid")
	if err == nil {
		t.Error("Expected error for invalid box name")
	}

	err = fc.DeleteMessage("test", "invalid")
	if err == nil {
		t.Error("Expected error for invalid box name")
	}
}

func TestFileChannel_InvalidMessageFormat(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileChannel(tmpDir)
	fsfc := fc.(*FileSystemFileChannel)

	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0o755)

	// Create invalid message files
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "no-frontmatter.md",
			content: "Just body content without frontmatter",
		},
		{
			name:    "incomplete-frontmatter.md",
			content: "---\nid: test\n\nNo closing delimiter",
		},
	}

	for _, tt := range tests {
		filePath := filepath.Join(inboxDir, tt.name)
		os.WriteFile(filePath, []byte(tt.content), 0o644)

		_, err := fsfc.readMessage(filePath)
		if err == nil {
			t.Errorf("Expected error reading invalid message %s", tt.name)
		}
	}
}
