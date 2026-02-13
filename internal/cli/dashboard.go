package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Dashboard panel indices.
const (
	panelTasks = iota
	panelMetrics
	panelAlerts
	panelCount
)

type dashboardModel struct {
	activePanel int
	width       int
	height      int

	// Data.
	taskCounts  map[string]int
	metricsData *metricsSnapshot
	alerts      []alertSnapshot

	// State.
	loading bool
	err     error
}

type metricsSnapshot struct {
	tasksCreated       int
	tasksCompleted     int
	agentSessions      int
	knowledgeExtracted int
	eventCount         int
}

type alertSnapshot struct {
	severity string
	message  string
	time     string
}

// dataLoadedMsg carries loaded data back to the model.
type dataLoadedMsg struct {
	taskCounts map[string]int
	metrics    *metricsSnapshot
	alerts     []alertSnapshot
	err        error
}

// Style definitions.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2)

	activePanelStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(1, 2)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62")).
			MarginBottom(1)

	statusInProgress = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	statusDone       = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	statusBlocked    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusReview     = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	statusBacklog    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusArchived   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	severityHigh   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	severityMedium = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	severityLow    = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newDashboardModel() dashboardModel {
	return dashboardModel{
		activePanel: panelTasks,
		loading:     true,
		taskCounts:  make(map[string]int),
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return loadData
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % panelCount
			return m, nil
		case "shift+tab":
			m.activePanel = (m.activePanel - 1 + panelCount) % panelCount
			return m, nil
		case "r":
			m.loading = true
			return m, loadData
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case dataLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.taskCounts = msg.taskCounts
		m.metricsData = msg.metrics
		m.alerts = msg.alerts
		m.err = nil
		return m, nil
	}

	return m, nil
}

func (m dashboardModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	title := titleStyle.Render(" ADB Dashboard ")
	help := helpStyle.Render("tab: switch panel | r: refresh | q: quit")

	if m.loading {
		return fmt.Sprintf("%s\n\n  Loading data...\n\n%s", title, help)
	}

	if m.err != nil {
		return fmt.Sprintf("%s\n\n  Error: %s\n\n%s", title, m.err, help)
	}

	tasksPanel := m.renderTasksPanel()
	metricsPanel := m.renderMetricsPanel()
	alertsPanel := m.renderAlertsPanel()

	// Available width for panels after accounting for margins.
	availableWidth := m.width - 2

	var body string
	if availableWidth > 120 {
		// Horizontal layout: three columns.
		colWidth := availableWidth / 3
		tasksPanel = m.applyPanelStyle(panelTasks, tasksPanel, colWidth-4)
		metricsPanel = m.applyPanelStyle(panelMetrics, metricsPanel, colWidth-4)
		alertsPanel = m.applyPanelStyle(panelAlerts, alertsPanel, colWidth-4)
		body = lipgloss.JoinHorizontal(lipgloss.Top, tasksPanel, metricsPanel, alertsPanel)
	} else {
		// Vertical layout: stacked.
		panelWidth := availableWidth - 4
		if panelWidth < 20 {
			panelWidth = 20
		}
		tasksPanel = m.applyPanelStyle(panelTasks, tasksPanel, panelWidth)
		metricsPanel = m.applyPanelStyle(panelMetrics, metricsPanel, panelWidth)
		alertsPanel = m.applyPanelStyle(panelAlerts, alertsPanel, panelWidth)
		body = lipgloss.JoinVertical(lipgloss.Left, tasksPanel, metricsPanel, alertsPanel)
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s", title, body, help)
}

func (m dashboardModel) applyPanelStyle(panel int, content string, width int) string {
	style := panelStyle
	if m.activePanel == panel {
		style = activePanelStyle
	}
	return style.Width(width).Render(content)
}

func (m dashboardModel) renderTasksPanel() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Tasks"))
	b.WriteString("\n")

	if len(m.taskCounts) == 0 {
		b.WriteString("  No tasks found.")
		return b.String()
	}

	// Display in lifecycle order.
	order := []string{"in_progress", "blocked", "review", "backlog", "done", "archived"}
	for _, status := range order {
		count, ok := m.taskCounts[status]
		if !ok || count == 0 {
			continue
		}
		label := fmt.Sprintf("  %-14s %d", status, count)
		b.WriteString(styleForStatus(status).Render(label))
		b.WriteString("\n")
	}

	total := 0
	for _, c := range m.taskCounts {
		total += c
	}
	b.WriteString(fmt.Sprintf("\n  Total: %d", total))

	return b.String()
}

func (m dashboardModel) renderMetricsPanel() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Metrics (7d)"))
	b.WriteString("\n")

	if m.metricsData == nil {
		b.WriteString("  No metrics available.")
		return b.String()
	}

	md := m.metricsData
	lines := []struct {
		label string
		value int
	}{
		{"Events", md.eventCount},
		{"Created", md.tasksCreated},
		{"Completed", md.tasksCompleted},
		{"Sessions", md.agentSessions},
		{"Knowledge", md.knowledgeExtracted},
	}

	for _, l := range lines {
		b.WriteString(fmt.Sprintf("  %-14s %d\n", l.label, l.value))
	}

	return b.String()
}

func (m dashboardModel) renderAlertsPanel() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Alerts"))
	b.WriteString("\n")

	if len(m.alerts) == 0 {
		b.WriteString("  No active alerts.")
		return b.String()
	}

	for _, a := range m.alerts {
		sev := styleForSeverity(a.severity).Render(fmt.Sprintf("[%s]", strings.ToUpper(a.severity)))
		b.WriteString(fmt.Sprintf("  %s %s\n", sev, a.message))
	}

	b.WriteString(fmt.Sprintf("\n  Total: %d alert(s)", len(m.alerts)))

	return b.String()
}

func styleForStatus(status string) lipgloss.Style {
	switch status {
	case "in_progress":
		return statusInProgress
	case "done":
		return statusDone
	case "blocked":
		return statusBlocked
	case "review":
		return statusReview
	case "backlog":
		return statusBacklog
	case "archived":
		return statusArchived
	default:
		return lipgloss.NewStyle()
	}
}

func styleForSeverity(severity string) lipgloss.Style {
	switch strings.ToLower(severity) {
	case "high":
		return severityHigh
	case "medium":
		return severityMedium
	case "low":
		return severityLow
	default:
		return lipgloss.NewStyle()
	}
}

func loadData() tea.Msg {
	result := dataLoadedMsg{
		taskCounts: make(map[string]int),
	}

	// Load task counts from TaskMgr.
	if TaskMgr != nil {
		tasks, err := TaskMgr.GetAllTasks()
		if err != nil {
			result.err = fmt.Errorf("loading tasks: %w", err)
			return result
		}
		for _, t := range tasks {
			result.taskCounts[string(t.Status)]++
		}
	}

	// Load metrics from MetricsCalc.
	if MetricsCalc != nil {
		since := time.Now().UTC().AddDate(0, 0, -7)
		metrics, err := MetricsCalc.Calculate(since)
		if err != nil {
			result.err = fmt.Errorf("loading metrics: %w", err)
			return result
		}
		result.metrics = &metricsSnapshot{
			tasksCreated:       metrics.TasksCreated,
			tasksCompleted:     metrics.TasksCompleted,
			agentSessions:      metrics.AgentSessions,
			knowledgeExtracted: metrics.KnowledgeExtracted,
			eventCount:         metrics.EventCount,
		}
	}

	// Load alerts from AlertEngine.
	if AlertEngine != nil {
		alerts, err := AlertEngine.Evaluate()
		if err != nil {
			result.err = fmt.Errorf("loading alerts: %w", err)
			return result
		}
		result.alerts = make([]alertSnapshot, 0, len(alerts))

		// Sort alerts by severity: high first, then medium, then low.
		sort.Slice(alerts, func(i, j int) bool {
			return severityRank(string(alerts[i].Severity)) < severityRank(string(alerts[j].Severity))
		})

		for _, a := range alerts {
			result.alerts = append(result.alerts, alertSnapshot{
				severity: string(a.Severity),
				message:  a.Message,
				time:     a.TriggeredAt.Format("2006-01-02 15:04 UTC"),
			})
		}
	}

	return result
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Interactive TUI dashboard for task metrics and alerts",
	Long: `Launch an interactive terminal dashboard showing task status,
metrics, and alerts in a live-updating view.

Navigate between panels with Tab, refresh with r, quit with q.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if MetricsCalc == nil {
			return fmt.Errorf("metrics calculator not initialized (observability may be disabled)")
		}
		p := tea.NewProgram(newDashboardModel(), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}
