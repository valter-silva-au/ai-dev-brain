package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/internal/observability"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Fake implementations ---

type fakeTaskManager struct {
	tasks map[string]*models.Task
}

func newFakeTaskManager(tasks ...*models.Task) *fakeTaskManager {
	m := &fakeTaskManager{tasks: make(map[string]*models.Task)}
	for _, t := range tasks {
		m.tasks[t.ID] = t
	}
	return m
}

func (f *fakeTaskManager) CreateTask(_ models.TaskType, _ string, _ string, _ core.CreateTaskOpts) (*models.Task, error) {
	return nil, nil
}

func (f *fakeTaskManager) ResumeTask(_ string) (*models.Task, error) {
	return nil, nil
}

func (f *fakeTaskManager) ArchiveTask(_ string) (*models.HandoffDocument, error) {
	return nil, nil
}

func (f *fakeTaskManager) UnarchiveTask(_ string) (*models.Task, error) {
	return nil, nil
}

func (f *fakeTaskManager) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	var result []*models.Task
	for _, t := range f.tasks {
		if t.Status == status {
			result = append(result, t)
		}
	}
	return result, nil
}

func (f *fakeTaskManager) GetAllTasks() ([]*models.Task, error) {
	result := make([]*models.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		result = append(result, t)
	}
	return result, nil
}

func (f *fakeTaskManager) GetTask(taskID string) (*models.Task, error) {
	t, ok := f.tasks[taskID]
	if !ok {
		return nil, &taskNotFoundError{taskID: taskID}
	}
	return t, nil
}

func (f *fakeTaskManager) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return &taskNotFoundError{taskID: taskID}
	}
	t.Status = status
	return nil
}

func (f *fakeTaskManager) UpdateTaskPriority(_ string, _ models.Priority) error {
	return nil
}

func (f *fakeTaskManager) ReorderPriorities(_ []string) error {
	return nil
}

func (f *fakeTaskManager) CleanupWorktree(_ string) error {
	return nil
}

type taskNotFoundError struct {
	taskID string
}

func (e *taskNotFoundError) Error() string {
	return "task not found: " + e.taskID
}

type fakeMetricsCalculator struct {
	metrics *observability.Metrics
}

func (f *fakeMetricsCalculator) Calculate(_ time.Time) (*observability.Metrics, error) {
	return f.metrics, nil
}

type fakeAlertEngine struct {
	alerts []observability.Alert
}

func (f *fakeAlertEngine) Evaluate() ([]observability.Alert, error) {
	return f.alerts, nil
}

// --- Test helpers ---

func sampleTask() *models.Task {
	return &models.Task{
		ID:         "TASK-00001",
		Title:      "add-auth",
		Type:       models.TaskTypeFeat,
		Status:     models.StatusInProgress,
		Priority:   models.P1,
		Owner:      "alice",
		Repo:       "github.com/acme/backend",
		Branch:     "feat/TASK-00001-add-auth",
		TicketPath: "/home/user/tickets/TASK-00001",
		Created:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Updated:    time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC),
		Tags:       []string{"security", "backend"},
	}
}

func sampleTask2() *models.Task {
	return &models.Task{
		ID:       "TASK-00002",
		Title:    "fix-login",
		Type:     models.TaskTypeBug,
		Status:   models.StatusBacklog,
		Priority: models.P2,
		Branch:   "bug/TASK-00002-fix-login",
		Created:  time.Date(2025, 1, 16, 9, 0, 0, 0, time.UTC),
		Updated:  time.Date(2025, 1, 16, 9, 0, 0, 0, time.UTC),
	}
}

// callTool is a helper that connects a client to the server and calls a tool.
func callTool(t *testing.T, srv *Server, toolName string, args map[string]any) *gomcp.CallToolResult {
	t.Helper()

	ctx := context.Background()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	t1, t2 := gomcp.NewInMemoryTransports()

	// Connect server (non-blocking).
	go func() {
		_ = srv.MCPServer().Run(ctx, t1)
	}()

	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &gomcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("call tool %s: %v", toolName, err)
	}

	return result
}

// callToolAllowError is like callTool but returns nil instead of failing when
// the tool call returns an error (e.g. schema validation failure).
func callToolAllowError(t *testing.T, srv *Server, toolName string, args map[string]any) *gomcp.CallToolResult {
	t.Helper()

	ctx := context.Background()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	t1, t2 := gomcp.NewInMemoryTransports()

	go func() {
		_ = srv.MCPServer().Run(ctx, t1)
	}()

	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &gomcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		// Protocol-level error (e.g. schema validation) -- return nil.
		return nil
	}

	return result
}

// --- Tests ---

func TestGetTask(t *testing.T) {
	task := sampleTask()
	tm := newFakeTaskManager(task)
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "get_task", map[string]any{"task_id": "TASK-00001"})

	if result.IsError {
		t.Fatalf("expected success, got error: %v", extractText(result))
	}

	text := extractText(result)
	var out taskOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		// The SDK may marshal the structured output differently;
		// try parsing the structured content.
		if result.StructuredContent != nil {
			data, _ := json.Marshal(result.StructuredContent)
			if err2 := json.Unmarshal(data, &out); err2 != nil {
				t.Fatalf("unmarshalling task output: %v (text was: %s)", err, text)
			}
		} else {
			t.Fatalf("unmarshalling task output: %v (text was: %s)", err, text)
		}
	}

	if out.ID != "TASK-00001" {
		t.Errorf("expected task ID TASK-00001, got %s", out.ID)
	}
	if out.Status != "in_progress" {
		t.Errorf("expected status in_progress, got %s", out.Status)
	}
	if out.Type != "feat" {
		t.Errorf("expected type feat, got %s", out.Type)
	}
	if out.Priority != "P1" {
		t.Errorf("expected priority P1, got %s", out.Priority)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "get_task", map[string]any{"task_id": "TASK-99999"})

	if !result.IsError {
		t.Fatal("expected error result for non-existent task")
	}

	text := extractText(result)
	if text == "" {
		t.Fatal("expected error message in result content")
	}
}

func TestGetTaskMissingID(t *testing.T) {
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, nil, "test")

	// The SDK validates required fields at the schema level, so calling
	// get_task without task_id produces a protocol-level validation error.
	result := callToolAllowError(t, srv, "get_task", map[string]any{})
	if result == nil {
		// Expected: the SDK rejected the call before it reached the handler.
		return
	}
	if !result.IsError {
		t.Fatal("expected error result for missing task_id")
	}
}

func TestListTasksAll(t *testing.T) {
	task1 := sampleTask()
	task2 := sampleTask2()
	tm := newFakeTaskManager(task1, task2)
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "list_tasks", map[string]any{})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	text := extractText(result)
	var out listTasksOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		if result.StructuredContent != nil {
			data, _ := json.Marshal(result.StructuredContent)
			if err2 := json.Unmarshal(data, &out); err2 != nil {
				t.Fatalf("unmarshalling list output: %v", err)
			}
		} else {
			t.Fatalf("unmarshalling list output: %v (text was: %s)", err, text)
		}
	}

	if out.Count != 2 {
		t.Errorf("expected 2 tasks, got %d", out.Count)
	}
}

func TestListTasksWithFilter(t *testing.T) {
	task1 := sampleTask()
	task2 := sampleTask2()
	tm := newFakeTaskManager(task1, task2)
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "list_tasks", map[string]any{"status": "backlog"})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	text := extractText(result)
	var out listTasksOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		if result.StructuredContent != nil {
			data, _ := json.Marshal(result.StructuredContent)
			if err2 := json.Unmarshal(data, &out); err2 != nil {
				t.Fatalf("unmarshalling list output: %v", err)
			}
		} else {
			t.Fatalf("unmarshalling list output: %v (text was: %s)", err, text)
		}
	}

	if out.Count != 1 {
		t.Errorf("expected 1 task with backlog status, got %d", out.Count)
	}
	if len(out.Tasks) > 0 && out.Tasks[0].ID != "TASK-00002" {
		t.Errorf("expected TASK-00002, got %s", out.Tasks[0].ID)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	task := sampleTask()
	tm := newFakeTaskManager(task)
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "update_task_status", map[string]any{
		"task_id": "TASK-00001",
		"status":  "review",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	if tm.tasks["TASK-00001"].Status != models.StatusReview {
		t.Errorf("expected task status to be review, got %s", tm.tasks["TASK-00001"].Status)
	}
}

func TestUpdateTaskStatusInvalid(t *testing.T) {
	task := sampleTask()
	tm := newFakeTaskManager(task)
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "update_task_status", map[string]any{
		"task_id": "TASK-00001",
		"status":  "invalid_status",
	})

	if !result.IsError {
		t.Fatal("expected error for invalid status")
	}
}

func TestGetMetrics(t *testing.T) {
	now := time.Now().UTC()
	mc := &fakeMetricsCalculator{
		metrics: &observability.Metrics{
			TasksCreated:   5,
			TasksCompleted: 3,
			TasksByStatus:  map[string]int{"in_progress": 2, "done": 3},
			TasksByType:    map[string]int{"feat": 3, "bug": 2},
			AgentSessions:  10,
			EventCount:     42,
			OldestEvent:    &now,
			NewestEvent:    &now,
		},
	}
	tm := newFakeTaskManager()
	srv := NewServer(tm, mc, nil, "test")

	result := callTool(t, srv, "get_metrics", map[string]any{})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	// Parse from structured content or text.
	var m metricsOutput
	if result.StructuredContent != nil {
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshalling structured metrics: %v", err)
		}
	} else {
		text := extractText(result)
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatalf("unmarshalling metrics text: %v (text was: %s)", err, text)
		}
	}

	if m.TasksCreated != 5 {
		t.Errorf("expected 5 tasks created, got %d", m.TasksCreated)
	}
	if m.EventCount != 42 {
		t.Errorf("expected 42 events, got %d", m.EventCount)
	}
}

func TestGetMetricsDisabled(t *testing.T) {
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "get_metrics", map[string]any{})

	if !result.IsError {
		t.Fatal("expected error when metrics calculator is nil")
	}

	text := extractText(result)
	if text == "" {
		t.Fatal("expected error message in result")
	}
}

func TestGetAlerts(t *testing.T) {
	now := time.Now().UTC()
	ae := &fakeAlertEngine{
		alerts: []observability.Alert{
			{
				ID:          "blocked-TASK-00001",
				Condition:   "task_blocked_too_long",
				Severity:    observability.SeverityHigh,
				Message:     "task TASK-00001 has been blocked for more than 24 hours",
				TriggeredAt: now,
			},
		},
	}
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, ae, "test")

	result := callTool(t, srv, "get_alerts", map[string]any{})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	text := extractText(result)
	var out getAlertsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		if result.StructuredContent != nil {
			data, _ := json.Marshal(result.StructuredContent)
			if err2 := json.Unmarshal(data, &out); err2 != nil {
				t.Fatalf("unmarshalling alerts output: %v", err)
			}
		} else {
			t.Fatalf("unmarshalling alerts output: %v (text was: %s)", err, text)
		}
	}

	if out.Count != 1 {
		t.Errorf("expected 1 alert, got %d", out.Count)
	}
	if len(out.Alerts) > 0 && out.Alerts[0].Severity != "high" {
		t.Errorf("expected high severity, got %s", out.Alerts[0].Severity)
	}
}

func TestGetAlertsDisabled(t *testing.T) {
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, nil, "test")

	result := callTool(t, srv, "get_alerts", map[string]any{})

	if !result.IsError {
		t.Fatal("expected error when alert engine is nil")
	}
}

func TestGetAlertsEmpty(t *testing.T) {
	ae := &fakeAlertEngine{alerts: []observability.Alert{}}
	tm := newFakeTaskManager()
	srv := NewServer(tm, nil, ae, "test")

	result := callTool(t, srv, "get_alerts", map[string]any{})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractText(result))
	}

	text := extractText(result)
	var out getAlertsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		if result.StructuredContent != nil {
			data, _ := json.Marshal(result.StructuredContent)
			_ = json.Unmarshal(data, &out)
		}
	}

	if out.Count != 0 {
		t.Errorf("expected 0 alerts, got %d", out.Count)
	}
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"7d", false},
		{"30d", false},
		{"24h", false},
		{"1h", false},
		{"", true},
		{"x", true},
		{"7x", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseSince(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSince(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// extractText extracts the text from the first TextContent in a CallToolResult.
func extractText(result *gomcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*gomcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
