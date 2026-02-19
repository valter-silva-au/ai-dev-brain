package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// fileChannelAdapter implements core.ChannelAdapter using markdown files with
// YAML frontmatter. Incoming items are read from an inbox directory and
// outgoing items are written to an outbox directory.
type fileChannelAdapter struct {
	name      string
	baseDir   string
	inboxDir  string
	outboxDir string
}

// FileChannelConfig holds the paths for the file-based channel adapter.
type FileChannelConfig struct {
	Name    string
	BaseDir string // Parent directory; inbox/ and outbox/ are created beneath it.
}

// NewFileChannelAdapter creates a file-based channel adapter that reads from
// baseDir/inbox/ and writes to baseDir/outbox/.
func NewFileChannelAdapter(cfg FileChannelConfig) (*fileChannelAdapter, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("creating file channel adapter: name is empty")
	}
	if cfg.BaseDir == "" {
		return nil, fmt.Errorf("creating file channel adapter: base dir is empty")
	}

	inboxDir := filepath.Join(cfg.BaseDir, "inbox")
	outboxDir := filepath.Join(cfg.BaseDir, "outbox")

	for _, dir := range []string{inboxDir, outboxDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating file channel directory %s: %w", dir, err)
		}
	}

	return &fileChannelAdapter{
		name:      cfg.Name,
		baseDir:   cfg.BaseDir,
		inboxDir:  inboxDir,
		outboxDir: outboxDir,
	}, nil
}

func (a *fileChannelAdapter) Name() string {
	return a.name
}

func (a *fileChannelAdapter) Type() models.ChannelType {
	return models.ChannelFile
}

// fileFrontmatter is the YAML frontmatter structure for channel files.
type fileFrontmatter struct {
	ID          string            `yaml:"id"`
	From        string            `yaml:"from,omitempty"`
	To          string            `yaml:"to,omitempty"`
	Subject     string            `yaml:"subject"`
	Date        string            `yaml:"date"`
	Priority    string            `yaml:"priority,omitempty"`
	Status      string            `yaml:"status"`
	Tags        []string          `yaml:"tags,omitempty"`
	RelatedTask string            `yaml:"related_task,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
}

// Fetch reads all pending markdown files from the inbox directory and returns
// them as ChannelItems. Only files with status "pending" are returned.
func (a *fileChannelAdapter) Fetch() ([]models.ChannelItem, error) {
	entries, err := os.ReadDir(a.inboxDir)
	if err != nil {
		return nil, fmt.Errorf("reading inbox directory: %w", err)
	}

	var items []models.ChannelItem
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(a.inboxDir, entry.Name())
		item, err := a.parseInboxFile(filePath)
		if err != nil {
			// Skip malformed files rather than failing the entire fetch.
			continue
		}

		if item.Status == models.ChannelStatusPending {
			items = append(items, *item)
		}
	}

	return items, nil
}

// Send writes an OutputItem as a markdown file in the outbox directory.
func (a *fileChannelAdapter) Send(item models.OutputItem) error {
	if item.ID == "" {
		return fmt.Errorf("sending to file channel: item ID is empty")
	}

	fm := fileFrontmatter{
		ID:      item.ID,
		To:      item.Destination,
		Subject: item.Subject,
		Date:    time.Now().UTC().Format("2006-01-02"),
		Status:  "sent",
	}
	if item.InReplyTo != "" {
		if fm.Metadata == nil {
			fm.Metadata = make(map[string]string)
		}
		fm.Metadata["in_reply_to"] = item.InReplyTo
	}
	if item.SourceTask != "" {
		if fm.Metadata == nil {
			fm.Metadata = make(map[string]string)
		}
		fm.Metadata["source_task"] = item.SourceTask
	}
	for k, v := range item.Metadata {
		if fm.Metadata == nil {
			fm.Metadata = make(map[string]string)
		}
		fm.Metadata[k] = v
	}

	content, err := a.renderFile(fm, item.Content)
	if err != nil {
		return fmt.Errorf("rendering outbox file: %w", err)
	}

	filename := item.ID + ".md"
	filePath := filepath.Join(a.outboxDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing outbox file: %w", err)
	}

	return nil
}

// MarkProcessed updates the status in the frontmatter of an inbox file to "processed".
func (a *fileChannelAdapter) MarkProcessed(itemID string) error {
	filePath, err := a.findInboxFile(itemID)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading inbox file: %w", err)
	}

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return fmt.Errorf("parsing frontmatter: %w", err)
	}

	fm.Status = "processed"

	content, err := a.renderFile(fm, body)
	if err != nil {
		return fmt.Errorf("rendering updated file: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing updated inbox file: %w", err)
	}

	return nil
}

// parseInboxFile reads a markdown file with YAML frontmatter and converts it
// to a ChannelItem.
func (a *fileChannelAdapter) parseInboxFile(filePath string) (*models.ChannelItem, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %s: %w", filePath, err)
	}

	priority := models.ChannelPriorityMedium
	switch strings.ToLower(fm.Priority) {
	case "high":
		priority = models.ChannelPriorityHigh
	case "low":
		priority = models.ChannelPriorityLow
	}

	status := models.ChannelStatusPending
	switch strings.ToLower(fm.Status) {
	case "processed":
		status = models.ChannelStatusProcessed
	case "actionable":
		status = models.ChannelStatusActionable
	case "archived":
		status = models.ChannelStatusArchived
	}

	item := &models.ChannelItem{
		ID:          fm.ID,
		Channel:     models.ChannelFile,
		Source:      a.name,
		From:        fm.From,
		Subject:     fm.Subject,
		Content:     body,
		Date:        fm.Date,
		Priority:    priority,
		Status:      status,
		Tags:        fm.Tags,
		Metadata:    fm.Metadata,
		RelatedTask: fm.RelatedTask,
	}

	// Use filename as ID if frontmatter ID is empty.
	if item.ID == "" {
		base := filepath.Base(filePath)
		item.ID = strings.TrimSuffix(base, ".md")
	}

	return item, nil
}

// findInboxFile locates an inbox file by item ID. It checks for ID.md first,
// then scans all files for a matching frontmatter ID.
func (a *fileChannelAdapter) findInboxFile(itemID string) (string, error) {
	// Fast path: check ID.md directly.
	direct := filepath.Join(a.inboxDir, itemID+".md")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}

	// Slow path: scan all files.
	entries, err := os.ReadDir(a.inboxDir)
	if err != nil {
		return "", fmt.Errorf("scanning inbox for item %s: %w", itemID, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		filePath := filepath.Join(a.inboxDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		fm, _, err := parseFrontmatter(string(data))
		if err != nil {
			continue
		}
		if fm.ID == itemID {
			return filePath, nil
		}
	}

	return "", fmt.Errorf("inbox item %q not found", itemID)
}

// renderFile produces a markdown string with YAML frontmatter.
func (a *fileChannelAdapter) renderFile(fm fileFrontmatter, body string) (string, error) {
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshaling frontmatter: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(fmBytes)
	sb.WriteString("---\n\n")
	sb.WriteString(body)

	return sb.String(), nil
}

// parseFrontmatter splits a markdown file into its YAML frontmatter and body.
// The frontmatter is delimited by "---" lines.
func parseFrontmatter(content string) (fileFrontmatter, string, error) {
	var fm fileFrontmatter

	if !strings.HasPrefix(content, "---\n") {
		return fm, content, fmt.Errorf("no frontmatter delimiter found")
	}

	rest := content[4:] // Skip opening "---\n"
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Try end-of-file delimiter.
		if strings.HasSuffix(rest, "\n---") {
			idx = len(rest) - 4
		} else {
			return fm, content, fmt.Errorf("no closing frontmatter delimiter found")
		}
	}

	fmStr := rest[:idx]
	// idx+4 skips past "\n---\n"; trim any leading newlines separating
	// the closing delimiter from the body content.
	body := strings.TrimLeft(rest[idx+4:], "\n")

	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return fm, body, fmt.Errorf("unmarshaling frontmatter: %w", err)
	}

	return fm, body, nil
}
