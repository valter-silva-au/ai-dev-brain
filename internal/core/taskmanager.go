package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// BacklogStore is the subset of storage.BacklogManager that TaskManager needs.
// Defining it here keeps core independent of the storage package.
type BacklogStore interface {
	AddTask(entry BacklogStoreEntry) error
	UpdateTask(taskID string, updates BacklogStoreEntry) error
	GetTask(taskID string) (*BacklogStoreEntry, error)
	GetAllTasks() ([]BacklogStoreEntry, error)
	FilterTasks(filter BacklogStoreFilter) ([]BacklogStoreEntry, error)
	Load() error
	Save() error
}

// BacklogStoreEntry mirrors storage.BacklogEntry.
type BacklogStoreEntry struct {
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

// BacklogStoreFilter mirrors storage.BacklogFilter.
type BacklogStoreFilter struct {
	Status   []models.TaskStatus
	Priority []models.Priority
	Owner    string
	Repo     string
	Tags     []string
}

// ContextStore is the subset of storage.ContextManager that TaskManager needs.
type ContextStore interface {
	LoadContext(taskID string) (interface{}, error)
}

// WorktreeRemover is the subset of GitWorktreeManager that TaskManager needs
// for worktree cleanup. Defining it here avoids importing the integration package.
type WorktreeRemover interface {
	RemoveWorktree(worktreePath string) error
}

// TaskManager defines the interface for task lifecycle operations.
type TaskManager interface {
	CreateTask(taskType models.TaskType, branchName string, repoPath string) (*models.Task, error)
	ResumeTask(taskID string) (*models.Task, error)
	ArchiveTask(taskID string) (*models.HandoffDocument, error)
	UnarchiveTask(taskID string) (*models.Task, error)
	GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error)
	GetAllTasks() ([]*models.Task, error)
	GetTask(taskID string) (*models.Task, error)
	UpdateTaskStatus(taskID string, status models.TaskStatus) error
	UpdateTaskPriority(taskID string, priority models.Priority) error
	ReorderPriorities(taskIDs []string) error
	CleanupWorktree(taskID string) error
}

// taskManager implements TaskManager by coordinating the BootstrapSystem,
// BacklogStore, and ContextStore.
type taskManager struct {
	basePath   string
	bootstrap  BootstrapSystem
	backlog    BacklogStore
	ctxStore   ContextStore
	worktreeRm WorktreeRemover
}

// NewTaskManager creates a new TaskManager with all dependencies injected.
// worktreeRm may be nil if worktree cleanup is not needed.
func NewTaskManager(basePath string, bootstrap BootstrapSystem, backlog BacklogStore, ctxStore ContextStore, worktreeRm WorktreeRemover) TaskManager {
	return &taskManager{
		basePath:   basePath,
		bootstrap:  bootstrap,
		backlog:    backlog,
		ctxStore:   ctxStore,
		worktreeRm: worktreeRm,
	}
}

// CreateTask orchestrates creating a new task: generates ID, bootstraps
// directory structure, creates worktree, and adds to the backlog.
func (tm *taskManager) CreateTask(taskType models.TaskType, branchName string, repoPath string) (*models.Task, error) {
	result, err := tm.bootstrap.Bootstrap(BootstrapConfig{
		Type:       taskType,
		Title:      branchName,
		BranchName: branchName,
		RepoPath:   repoPath,
	})
	if err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}

	task, err := tm.loadTaskFromTicket(result.TaskID)
	if err != nil {
		return nil, fmt.Errorf("creating task: loading status: %w", err)
	}

	entry := BacklogStoreEntry{
		ID:       task.ID,
		Title:    task.Title,
		Status:   task.Status,
		Priority: task.Priority,
		Owner:    task.Owner,
		Repo:     task.Repo,
		Branch:   task.Branch,
		Created:  task.Created.Format(time.RFC3339),
		Source:   task.Source,
	}

	if err := tm.backlog.Load(); err != nil {
		return nil, fmt.Errorf("creating task: loading backlog: %w", err)
	}
	if err := tm.backlog.AddTask(entry); err != nil {
		return nil, fmt.Errorf("creating task: adding to backlog: %w", err)
	}
	if err := tm.backlog.Save(); err != nil {
		return nil, fmt.Errorf("creating task: saving backlog: %w", err)
	}

	return task, nil
}

// ResumeTask loads a task's context and returns the task details.
// If the task is in backlog status, it is promoted to in_progress.
func (tm *taskManager) ResumeTask(taskID string) (*models.Task, error) {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return nil, fmt.Errorf("resuming task %s: %w", taskID, err)
	}

	if tm.ctxStore != nil {
		if _, err := tm.ctxStore.LoadContext(taskID); err != nil {
			return nil, fmt.Errorf("resuming task %s: loading context: %w", taskID, err)
		}
	}

	if task.Status == models.StatusBacklog {
		task.Status = models.StatusInProgress
		task.Updated = time.Now().UTC()
		if err := tm.saveTaskStatus(task); err != nil {
			return nil, fmt.Errorf("resuming task %s: updating status: %w", taskID, err)
		}
		if err := tm.updateBacklogStatus(taskID, models.StatusInProgress); err != nil {
			return nil, fmt.Errorf("resuming task %s: updating backlog: %w", taskID, err)
		}
	}

	return task, nil
}

// ArchiveTask generates a handoff document, writes handoff.md, saves the
// pre-archive status for later unarchive, and sets the task status to archived.
func (tm *taskManager) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return nil, fmt.Errorf("archiving task %s: %w", taskID, err)
	}

	if task.Status == models.StatusArchived {
		return nil, fmt.Errorf("archiving task %s: task is already archived", taskID)
	}

	ticketDir := resolveTicketDir(tm.basePath, taskID)

	// Save the pre-archive status so unarchive can restore it.
	preArchivePath := filepath.Join(ticketDir, ".pre_archive_status")
	if err := os.WriteFile(preArchivePath, []byte(string(task.Status)), 0o600); err != nil {
		return nil, fmt.Errorf("archiving task %s: saving pre-archive status: %w", taskID, err)
	}

	// Build the handoff document from notes.md and context.md content.
	handoff := tm.buildHandoffDocument(taskID, task)

	// Render and write handoff.md.
	handoffContent, err := renderHandoff(handoff)
	if err != nil {
		return nil, fmt.Errorf("archiving task %s: rendering handoff: %w", taskID, err)
	}
	handoffPath := filepath.Join(ticketDir, "handoff.md")
	if err := os.WriteFile(handoffPath, []byte(handoffContent), 0o600); err != nil {
		return nil, fmt.Errorf("archiving task %s: writing handoff.md: %w", taskID, err)
	}

	// Update status to archived.
	task.Status = models.StatusArchived
	task.Updated = time.Now().UTC()

	if err := tm.saveTaskStatus(task); err != nil {
		return nil, fmt.Errorf("archiving task %s: saving status: %w", taskID, err)
	}
	if err := tm.updateBacklogStatus(taskID, models.StatusArchived); err != nil {
		return nil, fmt.Errorf("archiving task %s: updating backlog: %w", taskID, err)
	}

	// Move ticket directory to _archived/.
	destDir := archivedTicketDir(tm.basePath, taskID)
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return nil, fmt.Errorf("archiving task %s: creating archive directory: %w", taskID, err)
	}
	if err := os.Rename(ticketDir, destDir); err != nil {
		return nil, fmt.Errorf("archiving task %s: moving to archive: %w", taskID, err)
	}

	// Update TicketPath in the moved status.yaml.
	task.TicketPath = destDir
	movedStatusPath := filepath.Join(destDir, "status.yaml")
	if statusData, marshalErr := yaml.Marshal(task); marshalErr == nil {
		_ = os.WriteFile(movedStatusPath, statusData, 0o600)
	}

	return handoff, nil
}

// UnarchiveTask restores a previously archived task to its pre-archive status.
// If the ticket was moved to _archived/, it is moved back to the active tickets/ directory.
func (tm *taskManager) UnarchiveTask(taskID string) (*models.Task, error) {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return nil, fmt.Errorf("unarchiving task %s: %w", taskID, err)
	}

	if task.Status != models.StatusArchived {
		return nil, fmt.Errorf("unarchiving task %s: task is not archived (status: %s)", taskID, task.Status)
	}

	// Read the pre-archive status from the current location.
	currentDir := resolveTicketDir(tm.basePath, taskID)
	preArchivePath := filepath.Join(currentDir, ".pre_archive_status")
	previousStatus := models.StatusBacklog
	data, err := os.ReadFile(preArchivePath)
	if err == nil {
		previousStatus = models.TaskStatus(strings.TrimSpace(string(data)))
	}

	// Move ticket directory back to active tickets/ if it's in _archived/.
	activeDir := activeTicketDir(tm.basePath, taskID)
	if currentDir != activeDir {
		if err := os.Rename(currentDir, activeDir); err != nil {
			return nil, fmt.Errorf("unarchiving task %s: moving from archive: %w", taskID, err)
		}
	}

	task.Status = previousStatus
	task.Updated = time.Now().UTC()
	task.TicketPath = activeDir

	if err := tm.saveTaskStatus(task); err != nil {
		return nil, fmt.Errorf("unarchiving task %s: saving status: %w", taskID, err)
	}
	if err := tm.updateBacklogStatus(taskID, previousStatus); err != nil {
		return nil, fmt.Errorf("unarchiving task %s: updating backlog: %w", taskID, err)
	}

	// Clean up the pre-archive status file.
	_ = os.Remove(filepath.Join(activeDir, ".pre_archive_status"))

	return task, nil
}

// buildHandoffDocument creates a HandoffDocument from the task's files.
func (tm *taskManager) buildHandoffDocument(taskID string, task *models.Task) *models.HandoffDocument {
	ticketDir := resolveTicketDir(tm.basePath, taskID)
	handoff := &models.HandoffDocument{
		TaskID:      taskID,
		GeneratedAt: time.Now().UTC(),
	}

	// Extract content from notes.md.
	notesPath := filepath.Join(ticketDir, "notes.md")
	if notesData, err := os.ReadFile(notesPath); err == nil {
		handoff.Summary = fmt.Sprintf("Task %s (%s): %s", taskID, task.Type, task.Title)
		handoff.Learnings = extractMarkdownListItems(string(notesData))
	}

	// Extract context for open items.
	contextPath := filepath.Join(ticketDir, "context.md")
	if contextData, err := os.ReadFile(contextPath); err == nil {
		content := string(contextData)
		handoff.OpenItems = extractSectionList(content, "## Open Questions")
		handoff.CompletedWork = extractSectionList(content, "## Recent Progress")
	}

	// List related docs.
	designPath := filepath.Join(ticketDir, "design.md")
	if _, err := os.Stat(designPath); err == nil {
		handoff.RelatedDocs = append(handoff.RelatedDocs, filepath.Join("tickets", taskID, "design.md"))
	}

	return handoff
}

// handoffTemplate is the Go text/template for rendering handoff.md.
var handoffTemplate = template.Must(template.New("handoff").Parse(`# Handoff: {{.TaskID}}

**Generated:** {{.GeneratedAt.Format "2006-01-02T15:04:05Z"}}
**Status:** Archived

## Summary
{{.Summary}}

## Completed Work
{{- range .CompletedWork}}
- {{.}}
{{- end}}
{{- if not .CompletedWork}}
- No completed work items recorded
{{- end}}

## Open Items
{{- range .OpenItems}}
- [ ] {{.}}
{{- end}}
{{- if not .OpenItems}}
- No open items
{{- end}}

## Key Learnings
{{- range .Learnings}}
- {{.}}
{{- end}}
{{- if not .Learnings}}
- No learnings recorded
{{- end}}

## Related Documentation
{{- range .RelatedDocs}}
- {{.}}
{{- end}}
{{- if not .RelatedDocs}}
- No related documentation
{{- end}}

## Provenance
This handoff was generated from {{.TaskID}} communications and notes.
`))

// renderHandoff renders a HandoffDocument to markdown.
func renderHandoff(doc *models.HandoffDocument) (string, error) {
	var buf bytes.Buffer
	if err := handoffTemplate.Execute(&buf, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// extractMarkdownListItems extracts top-level list items from markdown content.
func extractMarkdownListItems(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimPrefix(trimmed, "- ")
			item = strings.TrimPrefix(item, "[ ] ")
			item = strings.TrimPrefix(item, "[x] ")
			item = strings.TrimSpace(item)
			if item != "" && !strings.HasPrefix(item, "[") {
				items = append(items, item)
			}
		}
	}
	return items
}

// extractSectionList extracts list items from a specific markdown section.
func extractSectionList(content, heading string) []string {
	idx := strings.Index(content, heading)
	if idx < 0 {
		return nil
	}
	rest := content[idx+len(heading):]
	// Find next section.
	if endIdx := strings.Index(rest, "\n## "); endIdx >= 0 {
		rest = rest[:endIdx]
	}
	return extractMarkdownListItems(rest)
}

// GetTasksByStatus returns all tasks matching the given status.
func (tm *taskManager) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	if err := tm.backlog.Load(); err != nil {
		return nil, fmt.Errorf("getting tasks by status: %w", err)
	}

	entries, err := tm.backlog.FilterTasks(BacklogStoreFilter{
		Status: []models.TaskStatus{status},
	})
	if err != nil {
		return nil, fmt.Errorf("getting tasks by status %s: %w", status, err)
	}

	var tasks []*models.Task
	for _, entry := range entries {
		task, err := tm.loadTaskFromTicket(entry.ID)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetAllTasks returns all tasks from the backlog.
func (tm *taskManager) GetAllTasks() ([]*models.Task, error) {
	if err := tm.backlog.Load(); err != nil {
		return nil, fmt.Errorf("getting all tasks: %w", err)
	}

	entries, err := tm.backlog.GetAllTasks()
	if err != nil {
		return nil, fmt.Errorf("getting all tasks: %w", err)
	}

	var tasks []*models.Task
	for _, entry := range entries {
		task, err := tm.loadTaskFromTicket(entry.ID)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetTask returns a single task by ID.
func (tm *taskManager) GetTask(taskID string) (*models.Task, error) {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return nil, fmt.Errorf("getting task %s: %w", taskID, err)
	}
	return task, nil
}

// UpdateTaskStatus changes the status of a task and persists the change
// to both status.yaml and the backlog.
func (tm *taskManager) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return fmt.Errorf("updating task status %s: %w", taskID, err)
	}

	task.Status = status
	task.Updated = time.Now().UTC()

	if err := tm.saveTaskStatus(task); err != nil {
		return fmt.Errorf("updating task status %s: saving: %w", taskID, err)
	}

	if err := tm.updateBacklogStatus(taskID, status); err != nil {
		return fmt.Errorf("updating task status %s: backlog: %w", taskID, err)
	}

	return nil
}

// UpdateTaskPriority changes the priority of a task and persists the change.
func (tm *taskManager) UpdateTaskPriority(taskID string, priority models.Priority) error {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return fmt.Errorf("updating task priority %s: %w", taskID, err)
	}

	task.Priority = priority
	task.Updated = time.Now().UTC()

	if err := tm.saveTaskStatus(task); err != nil {
		return fmt.Errorf("updating task priority %s: saving: %w", taskID, err)
	}

	if err := tm.backlog.Load(); err != nil {
		return fmt.Errorf("updating task priority %s: loading backlog: %w", taskID, err)
	}
	if err := tm.backlog.UpdateTask(taskID, BacklogStoreEntry{Priority: priority}); err != nil {
		return fmt.Errorf("updating task priority %s: backlog: %w", taskID, err)
	}
	return tm.backlog.Save()
}

// ReorderPriorities assigns priorities P0..P3 to tasks in the order given.
// The first task gets P0 (highest), subsequent ones get P1, P2, P3.
// Tasks beyond the fourth keep P3.
func (tm *taskManager) ReorderPriorities(taskIDs []string) error {
	priorities := []models.Priority{models.P0, models.P1, models.P2, models.P3}

	for i, taskID := range taskIDs {
		p := models.P3
		if i < len(priorities) {
			p = priorities[i]
		}
		if err := tm.UpdateTaskPriority(taskID, p); err != nil {
			return fmt.Errorf("reordering priorities: %w", err)
		}
	}

	return nil
}

// CleanupWorktree removes the git worktree for a task and clears the
// worktree path from status.yaml.
func (tm *taskManager) CleanupWorktree(taskID string) error {
	task, err := tm.loadTaskFromTicket(taskID)
	if err != nil {
		return fmt.Errorf("cleaning up worktree for %s: %w", taskID, err)
	}

	if task.WorktreePath == "" {
		return fmt.Errorf("cleaning up worktree for %s: task has no worktree", taskID)
	}

	if tm.worktreeRm == nil {
		return fmt.Errorf("cleaning up worktree for %s: worktree remover not available", taskID)
	}

	if err := tm.worktreeRm.RemoveWorktree(task.WorktreePath); err != nil {
		return fmt.Errorf("cleaning up worktree for %s: %w", taskID, err)
	}

	// Clear the worktree path in status.yaml.
	task.WorktreePath = ""
	task.Updated = time.Now().UTC()
	if err := tm.saveTaskStatus(task); err != nil {
		return fmt.Errorf("cleaning up worktree for %s: saving status: %w", taskID, err)
	}

	return nil
}

// loadTaskFromTicket reads a task from its status.yaml file, checking both
// the active (tickets/{taskID}) and archived (tickets/_archived/{taskID}) locations.
func (tm *taskManager) loadTaskFromTicket(taskID string) (*models.Task, error) {
	ticketDir := resolveTicketDir(tm.basePath, taskID)
	statusPath := filepath.Join(ticketDir, "status.yaml")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, fmt.Errorf("reading status.yaml for %s: %w", taskID, err)
	}

	var task models.Task
	if err := yaml.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("parsing status.yaml for %s: %w", taskID, err)
	}

	return &task, nil
}

// saveTaskStatus writes the task back to its status.yaml file, checking both
// the active and archived locations to find the correct directory.
func (tm *taskManager) saveTaskStatus(task *models.Task) error {
	ticketDir := resolveTicketDir(tm.basePath, task.ID)
	statusPath := filepath.Join(ticketDir, "status.yaml")
	data, err := yaml.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshalling status.yaml for %s: %w", task.ID, err)
	}
	return os.WriteFile(statusPath, data, 0o600)
}

// updateBacklogStatus updates the task's status in the backlog file.
func (tm *taskManager) updateBacklogStatus(taskID string, status models.TaskStatus) error {
	if err := tm.backlog.Load(); err != nil {
		return err
	}
	if err := tm.backlog.UpdateTask(taskID, BacklogStoreEntry{Status: status}); err != nil {
		return err
	}
	return tm.backlog.Save()
}
