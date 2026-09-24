package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// CommunicationManager defines the interface for managing stakeholder communications
type CommunicationManager interface {
	SaveCommunication(comm *models.Communication) error
	GetCommunication(taskID, filename string) (*models.Communication, error)
	ListCommunications(taskID string) ([]string, error)
	GetAllCommunications(taskID string) ([]*models.Communication, error)
}

// TicketDirResolver maps a task id to its on-disk ticket directory. It lets the
// communication store honour the nested correlation layout
// (tickets/<platform>/<org>/<repo>/TASK-id-slug/) instead of assuming a flat
// tickets/<id>/ path. ok=false means "unresolved" — the store then falls back to
// the flat layout so callers with no resolver (and the store's own tests) keep
// working unchanged.
type TicketDirResolver func(taskID string) (dir string, ok bool)

// FileCommunicationManager implements CommunicationManager with file-based storage.
// It stores communications as dated markdown files under the ticket's
// communications/ directory. Without a resolver it uses the legacy flat path
// tickets/<id>/communications/; with one (wired in app.go) it places them inside
// the resolved nested ticket dir alongside context.md / notes.md.
type FileCommunicationManager struct {
	baseDir string
	resolve TicketDirResolver
	mu      sync.RWMutex // Provides concurrent safety
}

// NewFileCommunicationManager creates a new file-based communication manager.
// baseDir is the root directory for all task directories (e.g., "tickets").
func NewFileCommunicationManager(baseDir string) *FileCommunicationManager {
	return &FileCommunicationManager{
		baseDir: baseDir,
	}
}

// SetTicketDirResolver wires a resolver so communications land inside a ticket's
// real (possibly nested) directory. Idempotent; a nil resolver keeps the flat
// fallback.
func (fcm *FileCommunicationManager) SetTicketDirResolver(r TicketDirResolver) {
	fcm.resolve = r
}

// getCommunicationsDir returns the communications directory for a task, honouring
// the nested correlation layout via the resolver when one is wired.
func (fcm *FileCommunicationManager) getCommunicationsDir(taskID string) string {
	if fcm.resolve != nil {
		if dir, ok := fcm.resolve(taskID); ok && dir != "" {
			return filepath.Join(dir, "communications")
		}
	}
	return filepath.Join(fcm.baseDir, taskID, "communications")
}

// ensureCommunicationsDir creates the communications directory if it doesn't exist
func (fcm *FileCommunicationManager) ensureCommunicationsDir(taskID string) error {
	commDir := fcm.getCommunicationsDir(taskID)
	if err := os.MkdirAll(commDir, 0o755); err != nil {
		return fmt.Errorf("failed to create communications directory: %w", err)
	}
	return nil
}

// generateFilename generates a filename for a communication based on date and subject
func (fcm *FileCommunicationManager) generateFilename(comm *models.Communication) string {
	date := comm.Date.Format("2006-01-02")

	// Sanitize subject for filename (if no subject, use ID)
	baseName := comm.ID
	if comm.Subject != "" {
		// Replace spaces and special characters with underscores
		baseName = strings.ToLower(comm.Subject)
		baseName = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return '_'
		}, baseName)
		// Limit length
		if len(baseName) > 50 {
			baseName = baseName[:50]
		}
		// Trim trailing underscores
		baseName = strings.TrimRight(baseName, "_")
	}

	return fmt.Sprintf("%s_%s.md", date, baseName)
}

// SaveCommunication saves a communication as a dated markdown file
func (fcm *FileCommunicationManager) SaveCommunication(comm *models.Communication) error {
	if comm == nil {
		return fmt.Errorf("communication cannot be nil")
	}
	if comm.TaskID == "" {
		return fmt.Errorf("communication must have a task ID")
	}

	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	// Ensure communications directory exists
	if err := fcm.ensureCommunicationsDir(comm.TaskID); err != nil {
		return err
	}

	// Generate filename
	filename := fcm.generateFilename(comm)
	filePath := filepath.Join(fcm.getCommunicationsDir(comm.TaskID), filename)

	// Create markdown content with YAML frontmatter
	var content strings.Builder
	content.WriteString("---\n")

	// Marshal communication metadata to YAML
	yamlData, err := yaml.Marshal(comm)
	if err != nil {
		return fmt.Errorf("failed to marshal communication metadata: %w", err)
	}
	content.Write(yamlData)
	content.WriteString("---\n\n")

	// Add the content
	content.WriteString(comm.Content)
	content.WriteString("\n")

	// Write to file
	if err := os.WriteFile(filePath, []byte(content.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write communication file: %w", err)
	}

	return nil
}

// GetCommunication retrieves a communication by filename
func (fcm *FileCommunicationManager) GetCommunication(taskID, filename string) (*models.Communication, error) {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	filePath := filepath.Join(fcm.getCommunicationsDir(taskID), filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("communication file not found: %s", filename)
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read communication file: %w", err)
	}

	// Parse frontmatter
	content := string(data)
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid communication file format: missing frontmatter")
	}

	// Parse YAML frontmatter
	var comm models.Communication
	if err := yaml.Unmarshal([]byte(parts[1]), &comm); err != nil {
		return nil, fmt.Errorf("failed to parse communication metadata: %w", err)
	}

	return &comm, nil
}

// ListCommunications returns a list of communication filenames for a task
func (fcm *FileCommunicationManager) ListCommunications(taskID string) ([]string, error) {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	commDir := fcm.getCommunicationsDir(taskID)

	// Check if directory exists
	if _, err := os.Stat(commDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	// Read directory
	entries, err := os.ReadDir(commDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read communications directory: %w", err)
	}

	// Collect markdown files
	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			filenames = append(filenames, entry.Name())
		}
	}

	// Sort by filename (which includes date prefix)
	sort.Strings(filenames)

	return filenames, nil
}

// GetAllCommunications retrieves all communications for a task
func (fcm *FileCommunicationManager) GetAllCommunications(taskID string) ([]*models.Communication, error) {
	filenames, err := fcm.ListCommunications(taskID)
	if err != nil {
		return nil, err
	}

	communications := make([]*models.Communication, 0, len(filenames))
	for _, filename := range filenames {
		comm, err := fcm.GetCommunication(taskID, filename)
		if err != nil {
			// Log error but continue with other files
			continue
		}
		communications = append(communications, comm)
	}

	// Sort by date (newest first)
	sort.Slice(communications, func(i, j int) bool {
		return communications[i].Date.After(communications[j].Date)
	})

	return communications, nil
}
