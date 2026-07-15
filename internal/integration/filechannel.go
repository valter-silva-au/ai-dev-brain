package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileChannelMessage represents a message in the file channel
type FileChannelMessage struct {
	ID        string            `yaml:"id"`
	Type      string            `yaml:"type"` // e.g., "command", "response", "notification"
	Timestamp time.Time         `yaml:"timestamp"`
	Source    string            `yaml:"source"`
	Target    string            `yaml:"target"`
	Status    string            `yaml:"status"` // "pending", "processing", "completed", "failed"
	Metadata  map[string]string `yaml:"metadata,omitempty"`
	Body      string            `yaml:"-"` // Body content (after frontmatter)
}

// FileChannel manages file-based inbox/outbox communication
type FileChannel interface {
	// SendMessage writes a message to the outbox
	SendMessage(msg *FileChannelMessage) error

	// ReceiveMessage reads and processes a message from the inbox
	// Returns nil if no messages available
	ReceiveMessage() (*FileChannelMessage, error)

	// ListInbox returns all messages in the inbox
	ListInbox() ([]*FileChannelMessage, error)

	// ListOutbox returns all messages in the outbox
	ListOutbox() ([]*FileChannelMessage, error)

	// ArchiveMessage moves a message to the archive
	ArchiveMessage(messageID string, fromBox string) error

	// DeleteMessage removes a message
	DeleteMessage(messageID string, fromBox string) error
}

// FileSystemFileChannel implements FileChannel using filesystem
type FileSystemFileChannel struct {
	inboxDir   string
	outboxDir  string
	archiveDir string
}

// NewFileChannel creates a new file-based channel
func NewFileChannel(baseDir string) FileChannel {
	return &FileSystemFileChannel{
		inboxDir:   filepath.Join(baseDir, "inbox"),
		outboxDir:  filepath.Join(baseDir, "outbox"),
		archiveDir: filepath.Join(baseDir, "archive"),
	}
}

// ensureDirs creates the necessary directories
func (fc *FileSystemFileChannel) ensureDirs() error {
	dirs := []string{fc.inboxDir, fc.outboxDir, fc.archiveDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// SendMessage writes a message to the outbox
func (fc *FileSystemFileChannel) SendMessage(msg *FileChannelMessage) error {
	if err := fc.ensureDirs(); err != nil {
		return err
	}

	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	if msg.Status == "" {
		msg.Status = "pending"
	}

	// Generate filename
	filename := fmt.Sprintf("%s.md", msg.ID)
	filepath := filepath.Join(fc.outboxDir, filename)

	// Write message with YAML frontmatter
	return fc.writeMessage(filepath, msg)
}

// ReceiveMessage reads the next message from the inbox
func (fc *FileSystemFileChannel) ReceiveMessage() (*FileChannelMessage, error) {
	messages, err := fc.ListInbox()
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, nil // No messages
	}

	// Return the oldest message
	return messages[0], nil
}

// ListInbox returns all messages in the inbox
func (fc *FileSystemFileChannel) ListInbox() ([]*FileChannelMessage, error) {
	return fc.listMessages(fc.inboxDir)
}

// ListOutbox returns all messages in the outbox
func (fc *FileSystemFileChannel) ListOutbox() ([]*FileChannelMessage, error) {
	return fc.listMessages(fc.outboxDir)
}

// listMessages reads all messages from a directory
func (fc *FileSystemFileChannel) listMessages(dir string) ([]*FileChannelMessage, error) {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []*FileChannelMessage{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var messages []*FileChannelMessage
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .md files
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filepath := filepath.Join(dir, entry.Name())
		msg, err := fc.readMessage(filepath)
		if err != nil {
			// Skip invalid messages
			continue
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// ArchiveMessage moves a message to the archive
func (fc *FileSystemFileChannel) ArchiveMessage(messageID string, fromBox string) error {
	if err := fc.ensureDirs(); err != nil {
		return err
	}

	var sourceDir string
	switch fromBox {
	case "inbox":
		sourceDir = fc.inboxDir
	case "outbox":
		sourceDir = fc.outboxDir
	default:
		return fmt.Errorf("invalid box: %s", fromBox)
	}

	filename := fmt.Sprintf("%s.md", messageID)
	sourcePath := filepath.Join(sourceDir, filename)
	archivePath := filepath.Join(fc.archiveDir, filename)

	return os.Rename(sourcePath, archivePath)
}

// DeleteMessage removes a message
func (fc *FileSystemFileChannel) DeleteMessage(messageID string, fromBox string) error {
	var sourceDir string
	switch fromBox {
	case "inbox":
		sourceDir = fc.inboxDir
	case "outbox":
		sourceDir = fc.outboxDir
	case "archive":
		sourceDir = fc.archiveDir
	default:
		return fmt.Errorf("invalid box: %s", fromBox)
	}

	filename := fmt.Sprintf("%s.md", messageID)
	filepath := filepath.Join(sourceDir, filename)

	return os.Remove(filepath)
}

// writeMessage writes a message to a file with YAML frontmatter
func (fc *FileSystemFileChannel) writeMessage(filepath string, msg *FileChannelMessage) error {
	// Marshal frontmatter
	frontmatter, err := yaml.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Construct file content
	content := fmt.Sprintf("---\n%s---\n\n%s", string(frontmatter), msg.Body)

	// Write to file
	if err := os.WriteFile(filepath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write message file: %w", err)
	}

	return nil
}

// readMessage reads a message from a file with YAML frontmatter
func (fc *FileSystemFileChannel) readMessage(filepath string) (*FileChannelMessage, error) {
	// Read file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read message file: %w", err)
	}

	content := string(data)

	// Check for frontmatter delimiter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid message format: missing frontmatter")
	}

	// Find end of frontmatter
	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid message format: malformed frontmatter")
	}

	frontmatterYAML := parts[0]
	body := strings.TrimSpace(parts[1])

	// Parse frontmatter
	var msg FileChannelMessage
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &msg); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	msg.Body = body

	return &msg, nil
}
