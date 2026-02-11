package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// CommunicationManager manages task-related communications.
type CommunicationManager interface {
	AddCommunication(taskID string, comm models.Communication) error
	SearchCommunications(taskID string, query string) ([]models.Communication, error)
	GetAllCommunications(taskID string) ([]models.Communication, error)
}

type fileCommunicationManager struct {
	basePath string
}

// NewCommunicationManager creates a new CommunicationManager that stores
// communications as markdown files in tickets/{taskID}/communications/.
func NewCommunicationManager(basePath string) CommunicationManager {
	return &fileCommunicationManager{basePath: basePath}
}

func (m *fileCommunicationManager) commsDir(taskID string) string {
	return filepath.Join(m.basePath, "tickets", taskID, "communications")
}

// sanitizeForFilename replaces non-alphanumeric characters with hyphens
// and collapses multiple hyphens.
func sanitizeForFilename(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	result := re.ReplaceAllString(s, "-")
	result = strings.Trim(result, "-")
	return strings.ToLower(result)
}

// CommunicationFilename generates a filename from communication metadata.
// Format: YYYY-MM-DD-source-contact-topic.md
func CommunicationFilename(comm models.Communication) string {
	date := comm.Date.Format("2006-01-02")
	source := sanitizeForFilename(comm.Source)
	contact := sanitizeForFilename(comm.Contact)
	topic := sanitizeForFilename(comm.Topic)

	// Truncate topic to keep filenames reasonable.
	if len(topic) > 50 {
		topic = topic[:50]
	}

	return fmt.Sprintf("%s-%s-%s-%s.md", date, source, contact, topic)
}

func formatCommunication(comm models.Communication) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", CommunicationFilename(comm)))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", comm.Date.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Source:** %s\n", comm.Source))
	sb.WriteString(fmt.Sprintf("**Contact:** %s\n", comm.Contact))
	sb.WriteString(fmt.Sprintf("**Topic:** %s\n", comm.Topic))
	sb.WriteString("\n## Content\n\n")
	sb.WriteString(comm.Content)
	sb.WriteString("\n\n## Tags\n")
	for _, tag := range comm.Tags {
		sb.WriteString(fmt.Sprintf("- %s\n", string(tag)))
	}

	return sb.String()
}

func (m *fileCommunicationManager) AddCommunication(taskID string, comm models.Communication) error {
	dir := m.commsDir(taskID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("adding communication for %s: creating dir: %w", taskID, err)
	}

	filename := CommunicationFilename(comm)
	path := filepath.Join(dir, filename)

	// If file already exists, add a numeric suffix.
	if _, err := os.Stat(path); err == nil {
		for i := 2; ; i++ {
			ext := filepath.Ext(filename)
			base := strings.TrimSuffix(filename, ext)
			candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				path = candidate
				break
			}
		}
	}

	content := formatCommunication(comm)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("adding communication for %s: writing file: %w", taskID, err)
	}

	return nil
}

func (m *fileCommunicationManager) SearchCommunications(taskID string, query string) ([]models.Communication, error) {
	all, err := m.GetAllCommunications(taskID)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []models.Communication
	for _, comm := range all {
		if matchesCommunication(comm, query) {
			results = append(results, comm)
		}
	}
	return results, nil
}

func matchesCommunication(comm models.Communication, query string) bool {
	if strings.Contains(strings.ToLower(comm.Content), query) {
		return true
	}
	if strings.Contains(strings.ToLower(comm.Source), query) {
		return true
	}
	if strings.Contains(strings.ToLower(comm.Contact), query) {
		return true
	}
	if strings.Contains(strings.ToLower(comm.Topic), query) {
		return true
	}
	if strings.Contains(comm.Date.Format("2006-01-02"), query) {
		return true
	}
	return false
}

func (m *fileCommunicationManager) GetAllCommunications(taskID string) ([]models.Communication, error) {
	dir := m.commsDir(taskID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading communications for %s: %w", taskID, err)
	}

	var comms []models.Communication
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // G304: reading communication files from managed directory
		if err != nil {
			continue
		}
		comm := parseCommunicationMarkdown(string(data))
		comms = append(comms, comm)
	}
	return comms, nil
}

// parseCommunicationMarkdown parses a communication markdown file into a Communication struct.
func parseCommunicationMarkdown(content string) models.Communication {
	comm := models.Communication{}
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "**Date:**") {
			dateStr := strings.TrimSpace(strings.TrimPrefix(line, "**Date:**"))
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				comm.Date = t
			}
		} else if strings.HasPrefix(line, "**Source:**") {
			comm.Source = strings.TrimSpace(strings.TrimPrefix(line, "**Source:**"))
		} else if strings.HasPrefix(line, "**Contact:**") {
			comm.Contact = strings.TrimSpace(strings.TrimPrefix(line, "**Contact:**"))
		} else if strings.HasPrefix(line, "**Topic:**") {
			comm.Topic = strings.TrimSpace(strings.TrimPrefix(line, "**Topic:**"))
		}
	}

	// Extract content between ## Content and ## Tags.
	if idx := strings.Index(content, "## Content"); idx >= 0 {
		rest := content[idx+len("## Content"):]
		if endIdx := strings.Index(rest, "## Tags"); endIdx >= 0 {
			comm.Content = strings.TrimSpace(rest[:endIdx])
		} else {
			comm.Content = strings.TrimSpace(rest)
		}
	}

	// Extract tags.
	if idx := strings.Index(content, "## Tags"); idx >= 0 {
		rest := content[idx+len("## Tags"):]
		if endIdx := strings.Index(rest, "## "); endIdx >= 0 {
			rest = rest[:endIdx]
		}
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				tag := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if tag != "" {
					comm.Tags = append(comm.Tags, models.CommunicationTag(tag))
				}
			}
		}
	}

	return comm
}
