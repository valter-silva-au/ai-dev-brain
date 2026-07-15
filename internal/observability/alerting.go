package observability

import (
	"fmt"
	"time"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityHigh   AlertSeverity = "High"
	AlertSeverityMedium AlertSeverity = "Medium"
	AlertSeverityLow    AlertSeverity = "Low"
)

// AlertType represents the type of alert condition
type AlertType string

const (
	AlertTaskBlockedTooLong AlertType = "task_blocked_too_long"
	AlertTaskStale          AlertType = "task_stale"
	AlertReviewTooLong      AlertType = "review_too_long"
	AlertBacklogTooLarge    AlertType = "backlog_too_large"
)

// Alert represents a triggered alert
type Alert struct {
	Type      AlertType              `json:"type"`
	Severity  AlertSeverity          `json:"severity"`
	Message   string                 `json:"message"`
	TaskID    string                 `json:"task_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AlertThreshold represents a threshold configuration for an alert
type AlertThreshold struct {
	Type     AlertType     `json:"type"`
	Severity AlertSeverity `json:"severity"`
	Duration time.Duration `json:"duration,omitempty"` // for time-based thresholds
	Count    int           `json:"count,omitempty"`    // for count-based thresholds
}

// AlertConfig holds all alert threshold configurations
type AlertConfig struct {
	Thresholds []AlertThreshold
}

// DefaultAlertConfig returns the default alert configuration
func DefaultAlertConfig() *AlertConfig {
	return &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertTaskBlockedTooLong,
				Severity: AlertSeverityHigh,
				Duration: 24 * time.Hour,
			},
			{
				Type:     AlertTaskStale,
				Severity: AlertSeverityMedium,
				Duration: 3 * 24 * time.Hour, // 3 days
			},
			{
				Type:     AlertReviewTooLong,
				Severity: AlertSeverityMedium,
				Duration: 5 * 24 * time.Hour, // 5 days
			},
			{
				Type:     AlertBacklogTooLarge,
				Severity: AlertSeverityLow,
				Count:    10,
			},
		},
	}
}

// GetThreshold returns the threshold configuration for a specific alert type
func (ac *AlertConfig) GetThreshold(alertType AlertType) *AlertThreshold {
	for i := range ac.Thresholds {
		if ac.Thresholds[i].Type == alertType {
			return &ac.Thresholds[i]
		}
	}
	return nil
}

// SetThreshold updates or adds a threshold configuration
func (ac *AlertConfig) SetThreshold(threshold AlertThreshold) {
	for i := range ac.Thresholds {
		if ac.Thresholds[i].Type == threshold.Type {
			ac.Thresholds[i] = threshold
			return
		}
	}
	// If not found, add it
	ac.Thresholds = append(ac.Thresholds, threshold)
}

// AlertEvaluator evaluates alert conditions against thresholds
type AlertEvaluator struct {
	config      *AlertConfig
	metricsCalc *MetricsCalculator
}

// NewAlertEvaluator creates a new alert evaluator
func NewAlertEvaluator(config *AlertConfig, metricsCalc *MetricsCalculator) *AlertEvaluator {
	if config == nil {
		config = DefaultAlertConfig()
	}
	return &AlertEvaluator{
		config:      config,
		metricsCalc: metricsCalc,
	}
}

// EvaluateAll evaluates all alert conditions and returns triggered alerts
func (ae *AlertEvaluator) EvaluateAll() ([]Alert, error) {
	var alerts []Alert

	// Check task_blocked_too_long
	blockedAlerts, err := ae.evaluateBlockedTooLong()
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate blocked tasks: %w", err)
	}
	alerts = append(alerts, blockedAlerts...)

	// Check task_stale
	staleAlerts, err := ae.evaluateTaskStale()
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate stale tasks: %w", err)
	}
	alerts = append(alerts, staleAlerts...)

	// Check review_too_long
	reviewAlerts, err := ae.evaluateReviewTooLong()
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate review tasks: %w", err)
	}
	alerts = append(alerts, reviewAlerts...)

	// Check backlog_too_large
	backlogAlerts, err := ae.evaluateBacklogTooLarge()
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate backlog size: %w", err)
	}
	alerts = append(alerts, backlogAlerts...)

	return alerts, nil
}

// evaluateBlockedTooLong checks for tasks blocked longer than threshold
func (ae *AlertEvaluator) evaluateBlockedTooLong() ([]Alert, error) {
	threshold := ae.config.GetThreshold(AlertTaskBlockedTooLong)
	if threshold == nil {
		return nil, nil
	}

	var alerts []Alert
	blockedTasks, err := ae.metricsCalc.GetTasksInStatus("blocked")
	if err != nil {
		return nil, err
	}

	for _, taskID := range blockedTasks {
		duration, err := ae.metricsCalc.GetTaskDuration(taskID, "blocked")
		if err != nil {
			continue
		}

		if duration > threshold.Duration {
			alerts = append(alerts, Alert{
				Type:      AlertTaskBlockedTooLong,
				Severity:  threshold.Severity,
				Message:   fmt.Sprintf("Task %s has been blocked for %v (threshold: %v)", taskID, duration.Round(time.Hour), threshold.Duration),
				TaskID:    taskID,
				Timestamp: time.Now().UTC(),
				Metadata: map[string]interface{}{
					"duration":  duration.String(),
					"threshold": threshold.Duration.String(),
				},
			})
		}
	}

	return alerts, nil
}

// evaluateTaskStale checks for tasks in progress longer than threshold
func (ae *AlertEvaluator) evaluateTaskStale() ([]Alert, error) {
	threshold := ae.config.GetThreshold(AlertTaskStale)
	if threshold == nil {
		return nil, nil
	}

	var alerts []Alert
	inProgressTasks, err := ae.metricsCalc.GetTasksInStatus("in_progress")
	if err != nil {
		return nil, err
	}

	for _, taskID := range inProgressTasks {
		duration, err := ae.metricsCalc.GetTaskDuration(taskID, "in_progress")
		if err != nil {
			continue
		}

		if duration > threshold.Duration {
			alerts = append(alerts, Alert{
				Type:      AlertTaskStale,
				Severity:  threshold.Severity,
				Message:   fmt.Sprintf("Task %s has been in progress for %v (threshold: %v)", taskID, duration.Round(time.Hour), threshold.Duration),
				TaskID:    taskID,
				Timestamp: time.Now().UTC(),
				Metadata: map[string]interface{}{
					"duration":  duration.String(),
					"threshold": threshold.Duration.String(),
				},
			})
		}
	}

	return alerts, nil
}

// evaluateReviewTooLong checks for tasks in review longer than threshold
func (ae *AlertEvaluator) evaluateReviewTooLong() ([]Alert, error) {
	threshold := ae.config.GetThreshold(AlertReviewTooLong)
	if threshold == nil {
		return nil, nil
	}

	var alerts []Alert
	reviewTasks, err := ae.metricsCalc.GetTasksInStatus("review")
	if err != nil {
		return nil, err
	}

	for _, taskID := range reviewTasks {
		duration, err := ae.metricsCalc.GetTaskDuration(taskID, "review")
		if err != nil {
			continue
		}

		if duration > threshold.Duration {
			alerts = append(alerts, Alert{
				Type:      AlertReviewTooLong,
				Severity:  threshold.Severity,
				Message:   fmt.Sprintf("Task %s has been in review for %v (threshold: %v)", taskID, duration.Round(time.Hour), threshold.Duration),
				TaskID:    taskID,
				Timestamp: time.Now().UTC(),
				Metadata: map[string]interface{}{
					"duration":  duration.String(),
					"threshold": threshold.Duration.String(),
				},
			})
		}
	}

	return alerts, nil
}

// evaluateBacklogTooLarge checks if backlog exceeds threshold
func (ae *AlertEvaluator) evaluateBacklogTooLarge() ([]Alert, error) {
	threshold := ae.config.GetThreshold(AlertBacklogTooLarge)
	if threshold == nil {
		return nil, nil
	}

	var alerts []Alert
	backlogTasks, err := ae.metricsCalc.GetTasksInStatus("backlog")
	if err != nil {
		return nil, err
	}

	backlogSize := len(backlogTasks)
	if backlogSize > threshold.Count {
		alerts = append(alerts, Alert{
			Type:      AlertBacklogTooLarge,
			Severity:  threshold.Severity,
			Message:   fmt.Sprintf("Backlog has %d tasks (threshold: %d)", backlogSize, threshold.Count),
			Timestamp: time.Now().UTC(),
			Metadata: map[string]interface{}{
				"backlog_size": backlogSize,
				"threshold":    threshold.Count,
			},
		})
	}

	return alerts, nil
}
