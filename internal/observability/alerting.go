package observability

import (
	"fmt"
	"time"
)

// AlertSeverity represents the urgency of an alert.
type AlertSeverity string

const (
	SeverityHigh   AlertSeverity = "high"
	SeverityMedium AlertSeverity = "medium"
	SeverityLow    AlertSeverity = "low"
)

// Alert represents a triggered alert condition.
type Alert struct {
	ID          string        `json:"id"`
	Condition   string        `json:"condition"`
	Severity    AlertSeverity `json:"severity"`
	Message     string        `json:"message"`
	TriggeredAt time.Time     `json:"triggered_at"`
}

// AlertThresholds configures when alerts should fire.
type AlertThresholds struct {
	BlockedHours   int `yaml:"blocked_threshold_hours" json:"blocked_threshold_hours"`
	StaleDays      int `yaml:"stale_threshold_days" json:"stale_threshold_days"`
	ReviewDays     int `yaml:"review_threshold_days" json:"review_threshold_days"`
	MaxBacklogSize int `yaml:"max_backlog_size" json:"max_backlog_size"`
}

// DefaultAlertThresholds returns sensible defaults for alert thresholds.
func DefaultAlertThresholds() AlertThresholds {
	return AlertThresholds{
		BlockedHours:   24,
		StaleDays:      3,
		ReviewDays:     5,
		MaxBacklogSize: 10,
	}
}

// AlertEngine evaluates alert conditions against the event log.
type AlertEngine interface {
	Evaluate() ([]Alert, error)
}

// alertEngine implements AlertEngine by reading events and checking thresholds.
type alertEngine struct {
	eventLog   EventLog
	thresholds AlertThresholds
}

// NewAlertEngine creates a new AlertEngine with the given EventLog and thresholds.
func NewAlertEngine(eventLog EventLog, thresholds AlertThresholds) AlertEngine {
	return &alertEngine{
		eventLog:   eventLog,
		thresholds: thresholds,
	}
}

// Evaluate reads events and checks all alert conditions, returning any triggered alerts.
func (ae *alertEngine) Evaluate() ([]Alert, error) {
	now := time.Now().UTC()
	var alerts []Alert

	blockedAlerts, err := ae.checkBlockedTasks(now)
	if err != nil {
		return nil, fmt.Errorf("checking blocked tasks: %w", err)
	}
	alerts = append(alerts, blockedAlerts...)

	staleAlerts, err := ae.checkStaleTasks(now)
	if err != nil {
		return nil, fmt.Errorf("checking stale tasks: %w", err)
	}
	alerts = append(alerts, staleAlerts...)

	reviewAlerts, err := ae.checkLongReviews(now)
	if err != nil {
		return nil, fmt.Errorf("checking long reviews: %w", err)
	}
	alerts = append(alerts, reviewAlerts...)

	backlogAlerts, err := ae.checkBacklogSize()
	if err != nil {
		return nil, fmt.Errorf("checking backlog size: %w", err)
	}
	alerts = append(alerts, backlogAlerts...)

	return alerts, nil
}

// checkBlockedTasks looks for tasks that have been blocked longer than the threshold.
func (ae *alertEngine) checkBlockedTasks(now time.Time) ([]Alert, error) {
	events, err := ae.eventLog.Read(EventFilter{Type: "task.status_changed"})
	if err != nil {
		return nil, err
	}

	// Track the latest status change per task.
	type taskState struct {
		status    string
		changedAt time.Time
	}
	tasks := make(map[string]*taskState)

	for _, event := range events {
		taskID, _ := event.Data["task_id"].(string)
		newStatus, _ := event.Data["new_status"].(string)
		if taskID == "" || newStatus == "" {
			continue
		}
		tasks[taskID] = &taskState{status: newStatus, changedAt: event.Time}
	}

	threshold := time.Duration(ae.thresholds.BlockedHours) * time.Hour
	var alerts []Alert
	for taskID, state := range tasks {
		if state.status == "blocked" && now.Sub(state.changedAt) > threshold {
			alerts = append(alerts, Alert{
				ID:          fmt.Sprintf("blocked-%s", taskID),
				Condition:   "task_blocked_too_long",
				Severity:    SeverityHigh,
				Message:     fmt.Sprintf("task %s has been blocked for more than %d hours", taskID, ae.thresholds.BlockedHours),
				TriggeredAt: now,
			})
		}
	}

	return alerts, nil
}

// checkStaleTasks looks for in-progress tasks with no recent activity.
func (ae *alertEngine) checkStaleTasks(now time.Time) ([]Alert, error) {
	events, err := ae.eventLog.Read(EventFilter{})
	if err != nil {
		return nil, err
	}

	// Track latest activity per task.
	lastActivity := make(map[string]time.Time)
	currentStatus := make(map[string]string)

	for _, event := range events {
		taskID, _ := event.Data["task_id"].(string)
		if taskID == "" {
			continue
		}
		if event.Time.After(lastActivity[taskID]) {
			lastActivity[taskID] = event.Time
		}
		if event.Type == "task.status_changed" {
			if newStatus, ok := event.Data["new_status"].(string); ok {
				currentStatus[taskID] = newStatus
			}
		}
	}

	threshold := time.Duration(ae.thresholds.StaleDays) * 24 * time.Hour
	var alerts []Alert
	for taskID, lastTime := range lastActivity {
		status := currentStatus[taskID]
		if status == "in_progress" && now.Sub(lastTime) > threshold {
			alerts = append(alerts, Alert{
				ID:          fmt.Sprintf("stale-%s", taskID),
				Condition:   "task_stale",
				Severity:    SeverityMedium,
				Message:     fmt.Sprintf("task %s has had no activity for more than %d days", taskID, ae.thresholds.StaleDays),
				TriggeredAt: now,
			})
		}
	}

	return alerts, nil
}

// checkLongReviews looks for tasks in review status longer than the threshold.
func (ae *alertEngine) checkLongReviews(now time.Time) ([]Alert, error) {
	events, err := ae.eventLog.Read(EventFilter{Type: "task.status_changed"})
	if err != nil {
		return nil, err
	}

	type taskState struct {
		status    string
		changedAt time.Time
	}
	tasks := make(map[string]*taskState)

	for _, event := range events {
		taskID, _ := event.Data["task_id"].(string)
		newStatus, _ := event.Data["new_status"].(string)
		if taskID == "" || newStatus == "" {
			continue
		}
		tasks[taskID] = &taskState{status: newStatus, changedAt: event.Time}
	}

	threshold := time.Duration(ae.thresholds.ReviewDays) * 24 * time.Hour
	var alerts []Alert
	for taskID, state := range tasks {
		if state.status == "review" && now.Sub(state.changedAt) > threshold {
			alerts = append(alerts, Alert{
				ID:          fmt.Sprintf("review-%s", taskID),
				Condition:   "review_too_long",
				Severity:    SeverityMedium,
				Message:     fmt.Sprintf("task %s has been in review for more than %d days", taskID, ae.thresholds.ReviewDays),
				TriggeredAt: now,
			})
		}
	}

	return alerts, nil
}

// checkBacklogSize counts tasks currently in backlog and alerts if over the threshold.
func (ae *alertEngine) checkBacklogSize() ([]Alert, error) {
	events, err := ae.eventLog.Read(EventFilter{Type: "task.status_changed"})
	if err != nil {
		return nil, err
	}

	// Also count task.created events since new tasks start in backlog.
	createdEvents, err := ae.eventLog.Read(EventFilter{Type: "task.created"})
	if err != nil {
		return nil, err
	}

	// Track current status of each task.
	currentStatus := make(map[string]string)

	// Tasks start in backlog when created.
	for _, event := range createdEvents {
		taskID, _ := event.Data["task_id"].(string)
		if taskID != "" {
			currentStatus[taskID] = "backlog"
		}
	}

	// Apply status changes.
	for _, event := range events {
		taskID, _ := event.Data["task_id"].(string)
		newStatus, _ := event.Data["new_status"].(string)
		if taskID != "" && newStatus != "" {
			currentStatus[taskID] = newStatus
		}
	}

	backlogCount := 0
	for _, status := range currentStatus {
		if status == "backlog" {
			backlogCount++
		}
	}

	var alerts []Alert
	if backlogCount > ae.thresholds.MaxBacklogSize {
		alerts = append(alerts, Alert{
			ID:          "backlog-size",
			Condition:   "backlog_too_large",
			Severity:    SeverityLow,
			Message:     fmt.Sprintf("backlog has %d tasks, exceeding the maximum of %d", backlogCount, ae.thresholds.MaxBacklogSize),
			TriggeredAt: time.Now().UTC(),
		})
	}

	return alerts, nil
}
