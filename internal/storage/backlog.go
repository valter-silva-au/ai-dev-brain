package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// normalizeTaskID converts backslashes to forward slashes and strips trailing
// slashes, ensuring consistent lookup on Windows.
// NOTE: intentionally duplicated from core.NormalizeTaskID to avoid an import
// cycle (core already imports storage).
func normalizeTaskID(taskID string) string {
	normalized := filepath.ToSlash(taskID)
	normalized = strings.TrimRight(normalized, "/")
	return normalized
}

// BacklogEntry represents a single task entry in the backlog.
type BacklogEntry struct {
	ID        string            `yaml:"id"`
	Title     string            `yaml:"title"`
	Source    string            `yaml:"source,omitempty"`
	Status    models.TaskStatus `yaml:"status"`
	Priority  models.Priority   `yaml:"priority"`
	Owner     string            `yaml:"owner"`
	Repo      string            `yaml:"repo"`
	Branch    string            `yaml:"branch"`
	Created   string            `yaml:"created"`
	Tags      []string          `yaml:"tags"`
	BlockedBy []string          `yaml:"blocked_by"`
	Related   []string          `yaml:"related"`
}

// BacklogFilter specifies criteria for filtering backlog entries.
// All specified fields use AND logic: a task must match every criterion.
type BacklogFilter struct {
	Status   []models.TaskStatus
	Priority []models.Priority
	Owner    string
	Repo     string
	Tags     []string
}

// BacklogFile represents the top-level structure of backlog.yaml.
type BacklogFile struct {
	Version string                  `yaml:"version"`
	Tasks   map[string]BacklogEntry `yaml:"tasks"`
}

// BacklogManager defines the interface for managing the central task registry.
type BacklogManager interface {
	AddTask(entry BacklogEntry) error
	UpdateTask(taskID string, updates BacklogEntry) error
	RemoveTask(taskID string) error
	GetTask(taskID string) (*BacklogEntry, error)
	GetAllTasks() ([]BacklogEntry, error)
	FilterTasks(filter BacklogFilter) ([]BacklogEntry, error)
	Load() error
	Save() error
}

type fileBacklogManager struct {
	basePath string
	data     BacklogFile
}

// NewBacklogManager creates a new BacklogManager backed by a backlog.yaml file
// in the given base directory.
func NewBacklogManager(basePath string) BacklogManager {
	return &fileBacklogManager{
		basePath: basePath,
		data: BacklogFile{
			Version: "1.0",
			Tasks:   make(map[string]BacklogEntry),
		},
	}
}

func (m *fileBacklogManager) filePath() string {
	return filepath.Join(m.basePath, "backlog.yaml")
}

func (m *fileBacklogManager) AddTask(entry BacklogEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("adding task: ID must not be empty")
	}
	if _, exists := m.data.Tasks[entry.ID]; exists {
		return fmt.Errorf("adding task: task %s already exists", entry.ID)
	}
	m.data.Tasks[entry.ID] = entry
	return nil
}

func (m *fileBacklogManager) UpdateTask(taskID string, updates BacklogEntry) error {
	taskID = normalizeTaskID(taskID)
	existing, exists := m.data.Tasks[taskID]
	if !exists {
		return fmt.Errorf("updating task: task %s not found", taskID)
	}

	if updates.Title != "" {
		existing.Title = updates.Title
	}
	if updates.Source != "" {
		existing.Source = updates.Source
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.Priority != "" {
		existing.Priority = updates.Priority
	}
	if updates.Owner != "" {
		existing.Owner = updates.Owner
	}
	if updates.Repo != "" {
		existing.Repo = updates.Repo
	}
	if updates.Branch != "" {
		existing.Branch = updates.Branch
	}
	if updates.Created != "" {
		existing.Created = updates.Created
	}
	if updates.Tags != nil {
		existing.Tags = updates.Tags
	}
	if updates.BlockedBy != nil {
		existing.BlockedBy = updates.BlockedBy
	}
	if updates.Related != nil {
		existing.Related = updates.Related
	}

	m.data.Tasks[taskID] = existing
	return nil
}

func (m *fileBacklogManager) RemoveTask(taskID string) error {
	taskID = normalizeTaskID(taskID)
	if _, exists := m.data.Tasks[taskID]; !exists {
		return fmt.Errorf("removing task: task %s not found", taskID)
	}
	delete(m.data.Tasks, taskID)
	return nil
}

func (m *fileBacklogManager) GetTask(taskID string) (*BacklogEntry, error) {
	taskID = normalizeTaskID(taskID)
	entry, exists := m.data.Tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return &entry, nil
}

func (m *fileBacklogManager) GetAllTasks() ([]BacklogEntry, error) {
	entries := make([]BacklogEntry, 0, len(m.data.Tasks))
	for _, entry := range m.data.Tasks {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

func (m *fileBacklogManager) FilterTasks(filter BacklogFilter) ([]BacklogEntry, error) {
	all, err := m.GetAllTasks()
	if err != nil {
		return nil, err
	}

	var result []BacklogEntry
	for _, entry := range all {
		if matchesFilter(entry, filter) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func matchesFilter(entry BacklogEntry, filter BacklogFilter) bool {
	if len(filter.Status) > 0 && !containsStatus(filter.Status, entry.Status) {
		return false
	}
	if len(filter.Priority) > 0 && !containsPriority(filter.Priority, entry.Priority) {
		return false
	}
	if filter.Owner != "" && entry.Owner != filter.Owner {
		return false
	}
	if filter.Repo != "" && entry.Repo != filter.Repo {
		return false
	}
	if len(filter.Tags) > 0 && !hasAllTags(entry.Tags, filter.Tags) {
		return false
	}
	return true
}

func containsStatus(haystack []models.TaskStatus, needle models.TaskStatus) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsPriority(haystack []models.Priority, needle models.Priority) bool {
	for _, p := range haystack {
		if p == needle {
			return true
		}
	}
	return false
}

func hasAllTags(entryTags []string, requiredTags []string) bool {
	tagSet := make(map[string]struct{}, len(entryTags))
	for _, t := range entryTags {
		tagSet[t] = struct{}{}
	}
	for _, req := range requiredTags {
		if _, found := tagSet[req]; !found {
			return false
		}
	}
	return true
}

func (m *fileBacklogManager) Load() error {
	data, err := os.ReadFile(m.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			m.data = BacklogFile{
				Version: "1.0",
				Tasks:   make(map[string]BacklogEntry),
			}
			return nil
		}
		return fmt.Errorf("loading backlog: %w", err)
	}

	var bf BacklogFile
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("loading backlog: parsing YAML: %w", err)
	}
	if bf.Tasks == nil {
		bf.Tasks = make(map[string]BacklogEntry)
	}
	m.data = bf
	return nil
}

func (m *fileBacklogManager) Save() error {
	if err := os.MkdirAll(m.basePath, 0o750); err != nil {
		return fmt.Errorf("saving backlog: creating directory: %w", err)
	}
	data, err := yaml.Marshal(&m.data)
	if err != nil {
		return fmt.Errorf("saving backlog: marshaling YAML: %w", err)
	}
	if err := os.WriteFile(m.filePath(), data, 0o600); err != nil {
		return fmt.Errorf("saving backlog: writing file: %w", err)
	}
	return nil
}
