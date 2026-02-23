// Package mcp provides an MCP (Model Context Protocol) server that exposes
// adb functionality as MCP tools for AI coding assistants.
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps adb services and exposes them as MCP tools.
type Server struct {
	server      *gomcp.Server
	taskMgr     core.TaskManager
	metricsCalc observability.MetricsCalculator
	alertEngine observability.AlertEngine
}

// NewServer creates a new MCP server with the given adb service dependencies.
// metricsCalc and alertEngine may be nil if observability is disabled.
func NewServer(taskMgr core.TaskManager, metricsCalc observability.MetricsCalculator, alertEngine observability.AlertEngine, version string) *Server {
	if version == "" {
		version = "dev"
	}

	s := &Server{
		taskMgr:     taskMgr,
		metricsCalc: metricsCalc,
		alertEngine: alertEngine,
	}

	s.server = gomcp.NewServer(
		&gomcp.Implementation{Name: "adb", Version: version},
		nil,
	)

	s.registerTools()

	return s
}

// Run starts the MCP server on the given transport, blocking until the client
// disconnects or the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &gomcp.StdioTransport{})
}

// MCPServer returns the underlying mcp.Server for testing purposes.
func (s *Server) MCPServer() *gomcp.Server {
	return s.server
}

// --- Tool input/output types ---

type getTaskInput struct {
	TaskID string `json:"task_id" jsonschema:"required,the unique task identifier (e.g. TASK-00042 or github.com/org/repo/feature)"`
}

type taskOutput struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	Owner        string   `json:"owner,omitempty"`
	Repo         string   `json:"repo,omitempty"`
	Branch       string   `json:"branch"`
	WorktreePath string   `json:"worktree_path,omitempty"`
	TicketPath   string   `json:"ticket_path,omitempty"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated"`
	Tags         []string `json:"tags,omitempty"`
	BlockedBy    []string `json:"blocked_by,omitempty"`
	Related      []string `json:"related,omitempty"`
}

type listTasksInput struct {
	Status string `json:"status,omitempty" jsonschema:"filter tasks by status (backlog, in_progress, blocked, review, done, archived)"`
}

type listTasksOutput struct {
	Tasks []taskOutput `json:"tasks"`
	Count int          `json:"count"`
}

type updateTaskStatusInput struct {
	TaskID string `json:"task_id" jsonschema:"required,the unique task identifier (e.g. TASK-00042 or finance/new-feature)"`
	Status string `json:"status" jsonschema:"required,the new status (backlog, in_progress, blocked, review, done)"`
}

type updateTaskStatusOutput struct {
	Message string `json:"message"`
}

type getMetricsInput struct {
	Since string `json:"since,omitempty" jsonschema:"time window for metrics (e.g. 7d, 30d, 24h). Defaults to 7d."`
}

type metricsOutput struct {
	TasksCreated       int            `json:"tasks_created"`
	TasksCompleted     int            `json:"tasks_completed"`
	TasksByStatus      map[string]int `json:"tasks_by_status"`
	TasksByType        map[string]int `json:"tasks_by_type"`
	AgentSessions      int            `json:"agent_sessions"`
	KnowledgeExtracted int            `json:"knowledge_extracted"`
	EventCount         int            `json:"event_count"`
	OldestEvent        string         `json:"oldest_event,omitempty"`
	NewestEvent        string         `json:"newest_event,omitempty"`
}

type getAlertsInput struct{}

type alertOutput struct {
	ID          string `json:"id"`
	Condition   string `json:"condition"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	TriggeredAt string `json:"triggered_at"`
}

type getAlertsOutput struct {
	Alerts []alertOutput `json:"alerts"`
	Count  int           `json:"count"`
}

// --- Tool registration ---

func (s *Server) registerTools() {
	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "get_task",
		Description: "Get task details by ID. Returns the full task object including status, priority, branch, and paths.",
	}, s.handleGetTask)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks with an optional status filter. Returns an array of task summaries.",
	}, s.handleListTasks)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "update_task_status",
		Description: "Update a task's lifecycle status. Valid statuses: backlog, in_progress, blocked, review, done.",
	}, s.handleUpdateTaskStatus)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "get_metrics",
		Description: "Get aggregated metrics from the event log, including task counts, status transitions, and agent sessions.",
	}, s.handleGetMetrics)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "get_alerts",
		Description: "Evaluate and return active alerts (blocked tasks, stale tasks, long reviews, backlog size).",
	}, s.handleGetAlerts)
}

// --- Tool handlers ---

func (s *Server) handleGetTask(_ context.Context, _ *gomcp.CallToolRequest, input getTaskInput) (*gomcp.CallToolResult, taskOutput, error) {
	if input.TaskID == "" {
		return errorResult("task_id is required"), taskOutput{}, nil
	}

	task, err := s.taskMgr.GetTask(input.TaskID)
	if err != nil {
		return errorResult(fmt.Sprintf("getting task %s: %s", input.TaskID, err)), taskOutput{}, nil
	}

	out := taskToOutput(task)
	return nil, out, nil
}

func (s *Server) handleListTasks(_ context.Context, _ *gomcp.CallToolRequest, input listTasksInput) (*gomcp.CallToolResult, listTasksOutput, error) {
	var tasks []*models.Task
	var err error

	if input.Status != "" {
		status := models.TaskStatus(input.Status)
		tasks, err = s.taskMgr.GetTasksByStatus(status)
	} else {
		tasks, err = s.taskMgr.GetAllTasks()
	}

	if err != nil {
		return errorResult(fmt.Sprintf("listing tasks: %s", err)), listTasksOutput{}, nil
	}

	out := listTasksOutput{
		Tasks: make([]taskOutput, len(tasks)),
		Count: len(tasks),
	}
	for i, t := range tasks {
		out.Tasks[i] = taskToOutput(t)
	}

	return nil, out, nil
}

func (s *Server) handleUpdateTaskStatus(_ context.Context, _ *gomcp.CallToolRequest, input updateTaskStatusInput) (*gomcp.CallToolResult, updateTaskStatusOutput, error) {
	if input.TaskID == "" {
		return errorResult("task_id is required"), updateTaskStatusOutput{}, nil
	}
	if input.Status == "" {
		return errorResult("status is required"), updateTaskStatusOutput{}, nil
	}

	validStatuses := map[string]bool{
		"backlog": true, "in_progress": true, "blocked": true,
		"review": true, "done": true,
	}
	if !validStatuses[input.Status] {
		return errorResult(fmt.Sprintf("invalid status %q: must be one of backlog, in_progress, blocked, review, done", input.Status)), updateTaskStatusOutput{}, nil
	}

	status := models.TaskStatus(input.Status)
	if err := s.taskMgr.UpdateTaskStatus(input.TaskID, status); err != nil {
		return errorResult(fmt.Sprintf("updating task %s status: %s", input.TaskID, err)), updateTaskStatusOutput{}, nil
	}

	out := updateTaskStatusOutput{
		Message: fmt.Sprintf("task %s status updated to %s", input.TaskID, input.Status),
	}
	return nil, out, nil
}

func (s *Server) handleGetMetrics(_ context.Context, _ *gomcp.CallToolRequest, input getMetricsInput) (*gomcp.CallToolResult, metricsOutput, error) {
	if s.metricsCalc == nil {
		return errorResult("metrics calculator not available (observability may be disabled)"), emptyMetricsOutput(), nil
	}

	sinceStr := input.Since
	if sinceStr == "" {
		sinceStr = "7d"
	}

	sinceTime, err := parseSince(sinceStr)
	if err != nil {
		return errorResult(fmt.Sprintf("parsing since duration: %s", err)), emptyMetricsOutput(), nil
	}

	metrics, err := s.metricsCalc.Calculate(sinceTime)
	if err != nil {
		return errorResult(fmt.Sprintf("calculating metrics: %s", err)), emptyMetricsOutput(), nil
	}

	out := metricsOutput{
		TasksCreated:       metrics.TasksCreated,
		TasksCompleted:     metrics.TasksCompleted,
		TasksByStatus:      metrics.TasksByStatus,
		TasksByType:        metrics.TasksByType,
		AgentSessions:      metrics.AgentSessions,
		KnowledgeExtracted: metrics.KnowledgeExtracted,
		EventCount:         metrics.EventCount,
	}
	if metrics.OldestEvent != nil {
		out.OldestEvent = metrics.OldestEvent.Format(time.RFC3339)
	}
	if metrics.NewestEvent != nil {
		out.NewestEvent = metrics.NewestEvent.Format(time.RFC3339)
	}

	return nil, out, nil
}

func (s *Server) handleGetAlerts(_ context.Context, _ *gomcp.CallToolRequest, _ getAlertsInput) (*gomcp.CallToolResult, getAlertsOutput, error) {
	if s.alertEngine == nil {
		return errorResult("alert engine not available (observability may be disabled)"), getAlertsOutput{}, nil
	}

	alerts, err := s.alertEngine.Evaluate()
	if err != nil {
		return errorResult(fmt.Sprintf("evaluating alerts: %s", err)), getAlertsOutput{}, nil
	}

	out := getAlertsOutput{
		Alerts: make([]alertOutput, len(alerts)),
		Count:  len(alerts),
	}
	for i, a := range alerts {
		out.Alerts[i] = alertOutput{
			ID:          a.ID,
			Condition:   a.Condition,
			Severity:    string(a.Severity),
			Message:     a.Message,
			TriggeredAt: a.TriggeredAt.Format(time.RFC3339),
		}
	}

	return nil, out, nil
}

// --- Helpers ---

func taskToOutput(t *models.Task) taskOutput {
	return taskOutput{
		ID:           t.ID,
		Title:        t.Title,
		Type:         string(t.Type),
		Status:       string(t.Status),
		Priority:     string(t.Priority),
		Owner:        t.Owner,
		Repo:         t.Repo,
		Branch:       t.Branch,
		WorktreePath: t.WorktreePath,
		TicketPath:   t.TicketPath,
		Created:      t.Created.Format(time.RFC3339),
		Updated:      t.Updated.Format(time.RFC3339),
		Tags:         t.Tags,
		BlockedBy:    t.BlockedBy,
		Related:      t.Related,
	}
}

func emptyMetricsOutput() metricsOutput {
	return metricsOutput{
		TasksByStatus: make(map[string]int),
		TasksByType:   make(map[string]int),
	}
}

func errorResult(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// parseSince parses a human-friendly duration string like "7d", "30d", or "24h"
// into the corresponding time in the past.
func parseSince(s string) (time.Time, error) {
	now := time.Now().UTC()

	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("invalid duration %q", s)
	}

	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return time.Time{}, fmt.Errorf("invalid duration %q: %w", s, err)
	}

	switch suffix {
	case 'd':
		return now.AddDate(0, 0, -num), nil
	case 'h':
		return now.Add(-time.Duration(num) * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported duration suffix %q (use d or h)", string(suffix))
	}
}
