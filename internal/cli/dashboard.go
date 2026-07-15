package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewDashboardCmd creates the dashboard command
func NewDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open TUI dashboard",
		Long:  `Open an interactive terminal dashboard for workspace metrics and tasks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Create and run TUI
			p := tea.NewProgram(
				initialModel(),
				tea.WithAltScreen(),
			)

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("failed to run dashboard: %w", err)
			}

			return nil
		},
	}

	return cmd
}

// Model for the dashboard TUI
type dashboardModel struct {
	viewport viewport.Model
	ready    bool
	content  string
}

func initialModel() dashboardModel {
	return dashboardModel{}
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-2)
			m.viewport.YPosition = 0
			m.content = generateDashboardContent()
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 2
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m dashboardModel) View() string {
	if !m.ready {
		return "Loading dashboard..."
	}

	// Create header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.viewport.Width)

	header := headerStyle.Render("AI Dev Brain Dashboard")

	// Create footer with help text
	footerStyle := lipgloss.NewStyle().
		Faint(true).
		Foreground(lipgloss.Color("241"))

	footer := footerStyle.Render("Press 'q' or ESC to quit")

	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}

// generateDashboardContent generates the dashboard content
func generateDashboardContent() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		MarginTop(1).
		MarginBottom(1)

	content.WriteString(titleStyle.Render("📊 Workspace Overview"))
	content.WriteString("\n\n")

	// Get metrics
	if App != nil && App.MetricsCalculator != nil {
		metrics, err := App.MetricsCalculator.ComputeMetrics()
		if err == nil {
			// Metrics section
			content.WriteString(formatMetricsSection(metrics))
		} else {
			content.WriteString("⚠️  Failed to load metrics\n")
		}

		// Alerts section
		if App.AlertEvaluator != nil {
			alerts, err := App.AlertEvaluator.EvaluateAll()
			if err == nil {
				content.WriteString("\n")
				content.WriteString(formatAlertsSection(alerts))
			}
		}

		// Tasks section
		if App.BacklogManager != nil {
			backlog, err := App.BacklogManager.Load()
			if err == nil {
				content.WriteString("\n")
				content.WriteString(formatTasksSection(backlog.Tasks))
			}
		}
	} else {
		content.WriteString("⚠️  App not initialized\n")
	}

	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("Last updated: %s\n", time.Now().Format("15:04:05")))

	return content.String()
}

// formatMetricsSection formats the metrics display
func formatMetricsSection(metrics interface{}) string {
	// Import observability to use Metrics type
	m, ok := metrics.(*observability.Metrics)
	if !ok {
		return "⚠️  Invalid metrics data\n"
	}

	var sb strings.Builder

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212"))

	sb.WriteString(sectionStyle.Render("📈 Metrics"))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("  Tasks Created:     %d\n", m.TasksCreated))
	sb.WriteString(fmt.Sprintf("  Tasks Completed:   %d\n", m.TasksCompleted))
	sb.WriteString(fmt.Sprintf("  Agent Sessions:    %d\n", m.AgentSessions))
	sb.WriteString(fmt.Sprintf("  Worktrees Active:  %d\n", m.WorktreesCreated-m.WorktreesRemoved))

	if len(m.TasksByStatus) > 0 {
		sb.WriteString("\n  Status Breakdown:\n")
		for status, count := range m.TasksByStatus {
			sb.WriteString(fmt.Sprintf("    %s: %d\n", status, count))
		}
	}

	return sb.String()
}

// formatAlertsSection formats the alerts display
func formatAlertsSection(alerts interface{}) string {
	// Import observability to use Alert type
	alertList, ok := alerts.([]observability.Alert)
	if !ok {
		return "⚠️  Invalid alerts data\n"
	}

	var sb strings.Builder

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("208"))

	sb.WriteString(sectionStyle.Render("🚨 Alerts"))
	sb.WriteString("\n\n")

	if len(alertList) == 0 {
		sb.WriteString("  ✓ No active alerts\n")
	} else {
		for _, alert := range alertList {
			emoji := getAlertEmoji(string(alert.Severity))
			sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", emoji, alert.Severity, alert.Message))
		}
	}

	return sb.String()
}

// formatTasksSection formats the tasks display
func formatTasksSection(tasks interface{}) string {
	// Import models to use Task type
	taskList, ok := tasks.([]models.Task)
	if !ok {
		return "⚠️  Invalid tasks data\n"
	}

	var sb strings.Builder

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	sb.WriteString(sectionStyle.Render("📋 Recent Tasks"))
	sb.WriteString("\n\n")

	if len(taskList) == 0 {
		sb.WriteString("  No tasks found\n")
	} else {
		// Show up to 10 most recent tasks
		limit := 10
		if len(taskList) < limit {
			limit = len(taskList)
		}

		for i := 0; i < limit; i++ {
			task := taskList[i]
			statusEmoji := getStatusEmoji(string(task.Status))
			sb.WriteString(fmt.Sprintf("  %s %s: %s [%s]\n",
				statusEmoji, task.ID, task.Title, task.Priority))
		}

		if len(taskList) > limit {
			sb.WriteString(fmt.Sprintf("\n  ... and %d more tasks\n", len(taskList)-limit))
		}
	}

	return sb.String()
}

// getStatusEmoji returns an emoji for the task status
func getStatusEmoji(status string) string {
	switch status {
	case "in_progress":
		return "🔵"
	case "review":
		return "🟣"
	case "blocked":
		return "🔴"
	case "done":
		return "✅"
	case "backlog":
		return "⚪"
	default:
		return "⚫"
	}
}
