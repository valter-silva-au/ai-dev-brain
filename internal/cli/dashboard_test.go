package cli

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// mockDashboardTaskMgr implements the subset of core.TaskManager needed by the dashboard.
type mockDashboardTaskMgr struct {
	tasks []*models.Task
	err   error
}

func (m *mockDashboardTaskMgr) GetAllTasks() ([]*models.Task, error) {
	return m.tasks, m.err
}

func (m *mockDashboardTaskMgr) CreateTask(_ models.TaskType, _ string, _ string, _ core.CreateTaskOpts) (*models.Task, error) {
	return nil, nil
}
func (m *mockDashboardTaskMgr) ResumeTask(_ string) (*models.Task, error) { return nil, nil }
func (m *mockDashboardTaskMgr) ArchiveTask(_ string) (*models.HandoffDocument, error) {
	return nil, nil
}
func (m *mockDashboardTaskMgr) UnarchiveTask(_ string) (*models.Task, error) { return nil, nil }
func (m *mockDashboardTaskMgr) GetTasksByStatus(_ models.TaskStatus) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockDashboardTaskMgr) GetTask(_ string) (*models.Task, error)               { return nil, nil }
func (m *mockDashboardTaskMgr) UpdateTaskStatus(_ string, _ models.TaskStatus) error { return nil }
func (m *mockDashboardTaskMgr) UpdateTaskPriority(_ string, _ models.Priority) error { return nil }
func (m *mockDashboardTaskMgr) ReorderPriorities(_ []string) error                   { return nil }
func (m *mockDashboardTaskMgr) CleanupWorktree(_ string) error                       { return nil }

// mockDashboardMetrics implements observability.MetricsCalculator.
type mockDashboardMetrics struct {
	metrics *observability.Metrics
	err     error
}

func (m *mockDashboardMetrics) Calculate(_ time.Time) (*observability.Metrics, error) {
	return m.metrics, m.err
}

// mockDashboardAlerts implements observability.AlertEngine.
type mockDashboardAlerts struct {
	alerts []observability.Alert
	err    error
}

func (m *mockDashboardAlerts) Evaluate() ([]observability.Alert, error) {
	return m.alerts, m.err
}

func TestDashboardModel_Init(t *testing.T) {
	m := newDashboardModel()

	if m.activePanel != panelTasks {
		t.Errorf("expected activePanel = %d, got %d", panelTasks, m.activePanel)
	}
	if !m.loading {
		t.Error("expected loading = true on init")
	}
	if m.taskCounts == nil {
		t.Error("expected taskCounts to be initialized")
	}

	// Init should return a command (loadData).
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected Init to return a non-nil command")
	}
}

func TestDashboardModel_KeyQ(t *testing.T) {
	m := newDashboardModel()
	m.loading = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected tea.Quit command from q key")
	}

	// Verify the command produces a quit message.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}

	// Model should be unchanged.
	dm := updated.(dashboardModel)
	if dm.activePanel != panelTasks {
		t.Errorf("expected activePanel unchanged, got %d", dm.activePanel)
	}
}

func TestDashboardModel_KeyEsc(t *testing.T) {
	m := newDashboardModel()
	m.loading = false

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected tea.Quit command from esc key")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestDashboardModel_KeyTab(t *testing.T) {
	m := newDashboardModel()
	if m.activePanel != panelTasks {
		t.Fatalf("expected initial panel = %d, got %d", panelTasks, m.activePanel)
	}

	// Tab should cycle forward.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Error("expected no command from tab key")
	}
	dm := updated.(dashboardModel)
	if dm.activePanel != panelMetrics {
		t.Errorf("expected panel %d after first tab, got %d", panelMetrics, dm.activePanel)
	}

	// Tab again.
	updated, _ = dm.Update(tea.KeyMsg{Type: tea.KeyTab})
	dm = updated.(dashboardModel)
	if dm.activePanel != panelAlerts {
		t.Errorf("expected panel %d after second tab, got %d", panelAlerts, dm.activePanel)
	}

	// Tab wraps around.
	updated, _ = dm.Update(tea.KeyMsg{Type: tea.KeyTab})
	dm = updated.(dashboardModel)
	if dm.activePanel != panelTasks {
		t.Errorf("expected panel %d after wrap, got %d", panelTasks, dm.activePanel)
	}
}

func TestDashboardModel_KeyShiftTab(t *testing.T) {
	m := newDashboardModel()

	// Shift+Tab should cycle backward (wrap from 0 to panelCount-1).
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if cmd != nil {
		t.Error("expected no command from shift+tab")
	}
	dm := updated.(dashboardModel)
	if dm.activePanel != panelAlerts {
		t.Errorf("expected panel %d after shift+tab from 0, got %d", panelAlerts, dm.activePanel)
	}
}

func TestDashboardModel_KeyR(t *testing.T) {
	m := newDashboardModel()
	m.loading = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	dm := updated.(dashboardModel)
	if !dm.loading {
		t.Error("expected loading = true after pressing r")
	}
	if cmd == nil {
		t.Error("expected a command (loadData) from r key")
	}
}

func TestDashboardModel_DataLoaded(t *testing.T) {
	m := newDashboardModel()

	msg := dataLoadedMsg{
		taskCounts: map[string]int{
			"in_progress": 3,
			"backlog":     5,
			"done":        2,
		},
		metrics: &metricsSnapshot{
			tasksCreated:       8,
			tasksCompleted:     4,
			agentSessions:      12,
			knowledgeExtracted: 1,
			eventCount:         42,
		},
		alerts: []alertSnapshot{
			{severity: "high", message: "task blocked", time: "2025-01-15 10:30 UTC"},
			{severity: "low", message: "large backlog", time: "2025-01-15 10:30 UTC"},
		},
	}

	updated, cmd := m.Update(msg)
	if cmd != nil {
		t.Error("expected no command after dataLoadedMsg")
	}

	dm := updated.(dashboardModel)
	if dm.loading {
		t.Error("expected loading = false after data loaded")
	}
	if dm.err != nil {
		t.Errorf("expected no error, got: %v", dm.err)
	}
	if dm.taskCounts["in_progress"] != 3 {
		t.Errorf("expected in_progress = 3, got %d", dm.taskCounts["in_progress"])
	}
	if dm.taskCounts["backlog"] != 5 {
		t.Errorf("expected backlog = 5, got %d", dm.taskCounts["backlog"])
	}
	if dm.metricsData == nil {
		t.Fatal("expected metricsData to be set")
	}
	if dm.metricsData.tasksCreated != 8 {
		t.Errorf("expected tasksCreated = 8, got %d", dm.metricsData.tasksCreated)
	}
	if dm.metricsData.eventCount != 42 {
		t.Errorf("expected eventCount = 42, got %d", dm.metricsData.eventCount)
	}
	if len(dm.alerts) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(dm.alerts))
	}
}

func TestDashboardModel_DataLoadedError(t *testing.T) {
	m := newDashboardModel()

	msg := dataLoadedMsg{
		err: errors.New("connection failed"),
	}

	updated, _ := m.Update(msg)
	dm := updated.(dashboardModel)
	if dm.loading {
		t.Error("expected loading = false after error")
	}
	if dm.err == nil {
		t.Fatal("expected error to be set")
	}
	if dm.err.Error() != "connection failed" {
		t.Errorf("expected error 'connection failed', got %q", dm.err.Error())
	}
}

func TestDashboardModel_WindowResize(t *testing.T) {
	m := newDashboardModel()

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if cmd != nil {
		t.Error("expected no command from window resize")
	}
	dm := updated.(dashboardModel)
	if dm.width != 200 {
		t.Errorf("expected width = 200, got %d", dm.width)
	}
	if dm.height != 50 {
		t.Errorf("expected height = 50, got %d", dm.height)
	}
}

func TestDashboardModel_ViewLoading(t *testing.T) {
	m := newDashboardModel()
	m.width = 100
	m.height = 40

	view := m.View()
	if !contains(view, "Loading data") {
		t.Error("expected loading view to contain 'Loading data'")
	}
}

func TestDashboardModel_ViewWithData(t *testing.T) {
	m := newDashboardModel()
	m.width = 130
	m.height = 40
	m.loading = false
	m.taskCounts = map[string]int{
		"in_progress": 2,
		"done":        1,
	}
	m.metricsData = &metricsSnapshot{
		tasksCreated:   5,
		tasksCompleted: 3,
		eventCount:     20,
	}
	m.alerts = []alertSnapshot{
		{severity: "high", message: "task TASK-00001 blocked"},
	}

	view := m.View()
	if !contains(view, "Tasks") {
		t.Error("expected view to contain 'Tasks' panel")
	}
	if !contains(view, "Metrics") {
		t.Error("expected view to contain 'Metrics' panel")
	}
	if !contains(view, "Alerts") {
		t.Error("expected view to contain 'Alerts' panel")
	}
	if !contains(view, "in_progress") {
		t.Error("expected view to contain 'in_progress' status")
	}
}

func TestDashboardModel_ViewVerticalLayout(t *testing.T) {
	m := newDashboardModel()
	m.width = 80 // Less than 120, should use vertical layout.
	m.height = 40
	m.loading = false
	m.taskCounts = map[string]int{"backlog": 1}

	view := m.View()
	if !contains(view, "Tasks") {
		t.Error("expected vertical layout view to contain 'Tasks'")
	}
}

func TestDashboardLoadData(t *testing.T) {
	// Save and restore package-level vars.
	origTaskMgr := TaskMgr
	origMetrics := MetricsCalc
	origAlerts := AlertEngine
	defer func() {
		TaskMgr = origTaskMgr
		MetricsCalc = origMetrics
		AlertEngine = origAlerts
	}()

	TaskMgr = &mockDashboardTaskMgr{
		tasks: []*models.Task{
			{ID: "TASK-00001", Status: models.StatusInProgress},
			{ID: "TASK-00002", Status: models.StatusInProgress},
			{ID: "TASK-00003", Status: models.StatusBacklog},
		},
	}

	now := time.Now().UTC()
	MetricsCalc = &mockDashboardMetrics{
		metrics: &observability.Metrics{
			TasksCreated:       3,
			TasksCompleted:     1,
			AgentSessions:      5,
			KnowledgeExtracted: 0,
			EventCount:         15,
			OldestEvent:        &now,
			NewestEvent:        &now,
			TasksByStatus:      map[string]int{"in_progress": 2, "backlog": 1},
			TasksByType:        map[string]int{"feat": 2, "bug": 1},
		},
	}

	AlertEngine = &mockDashboardAlerts{
		alerts: []observability.Alert{
			{
				Severity:    observability.SeverityHigh,
				Message:     "task blocked too long",
				TriggeredAt: now,
			},
		},
	}

	msg := loadData()
	data, ok := msg.(dataLoadedMsg)
	if !ok {
		t.Fatalf("expected dataLoadedMsg, got %T", msg)
	}
	if data.err != nil {
		t.Fatalf("unexpected error: %v", data.err)
	}
	if data.taskCounts["in_progress"] != 2 {
		t.Errorf("expected in_progress = 2, got %d", data.taskCounts["in_progress"])
	}
	if data.taskCounts["backlog"] != 1 {
		t.Errorf("expected backlog = 1, got %d", data.taskCounts["backlog"])
	}
	if data.metrics == nil {
		t.Fatal("expected metrics to be set")
	}
	if data.metrics.tasksCreated != 3 {
		t.Errorf("expected tasksCreated = 3, got %d", data.metrics.tasksCreated)
	}
	if len(data.alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(data.alerts))
	}
	if data.alerts[0].severity != "high" {
		t.Errorf("expected alert severity 'high', got %q", data.alerts[0].severity)
	}
}

func TestDashboardCmd_NilMetricsCalc(t *testing.T) {
	origMetrics := MetricsCalc
	defer func() { MetricsCalc = origMetrics }()
	MetricsCalc = nil

	err := dashboardCmd.RunE(dashboardCmd, nil)
	if err == nil {
		t.Fatal("expected error when MetricsCalc is nil")
	}
	if !contains(err.Error(), "metrics calculator not initialized") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
