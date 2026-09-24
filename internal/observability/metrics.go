package observability

import (
	"time"
)

// Metrics represents aggregated metrics derived from the event log
type Metrics struct {
	TasksCreated       int                       `json:"tasks_created"`
	TasksCompleted     int                       `json:"tasks_completed"`
	TasksByStatus      map[string]int            `json:"tasks_by_status"`
	TasksByType        map[string]int            `json:"tasks_by_type"`
	AgentSessions      int                       `json:"agent_sessions"`
	KnowledgeExtracts  int                       `json:"knowledge_extracts"`
	WorktreesCreated   int                       `json:"worktrees_created"`
	WorktreesRemoved   int                       `json:"worktrees_removed"`
	LastEventTimestamp time.Time                 `json:"last_event_timestamp"`
	TaskStatusHistory  map[string][]StatusChange `json:"task_status_history"` // task_id -> status changes
}

// unarchivedStatus is the status a task returns to when it is unarchived.
// core.TaskManager.Unarchive sets models.TaskStatusBacklog unconditionally but
// emits only "task.unarchived" (no task.status_changed), and this package
// cannot import pkg/models without an import cycle — so the fold has to know
// the value. A payload "status" key, if one is ever added at the emit site,
// takes precedence over this default.
const unarchivedStatus = "backlog"

// StatusChange represents a status change for a task
type StatusChange struct {
	Timestamp time.Time `json:"timestamp"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
}

// MetricsCalculator computes metrics on-demand from event log
type MetricsCalculator struct {
	eventLog *EventLog
}

// NewMetricsCalculator creates a new metrics calculator
func NewMetricsCalculator(eventLog *EventLog) *MetricsCalculator {
	return &MetricsCalculator{
		eventLog: eventLog,
	}
}

// ComputeMetrics derives all metrics from the entire event log (no time window).
func (mc *MetricsCalculator) ComputeMetrics() (*Metrics, error) {
	return mc.ComputeMetricsSince(time.Time{})
}

// ComputeMetricsSince derives metrics from only the events at or after cutoff.
// A zero cutoff (time.Time{}) means "no window" — every event is counted, which
// is what ComputeMetrics uses. This is the real fix for `adb metrics --since`:
// the counts reflect the window's events, rather than the whole log with an
// all-or-nothing blank when the newest event predates the cutoff (#153).
func (mc *MetricsCalculator) ComputeMetricsSince(cutoff time.Time) (*Metrics, error) {
	events, err := mc.eventLog.ReadAll()
	if err != nil {
		return nil, err
	}

	metrics := &Metrics{
		TasksByStatus:     make(map[string]int),
		TasksByType:       make(map[string]int),
		TaskStatusHistory: make(map[string][]StatusChange),
	}
	// currentStatus tracks each task's live status within the window so the
	// terminal events (archived/deleted) can decrement the right bucket even for
	// a task that never emitted a status_changed (e.g. created → deleted). It is
	// the single source of truth for what TasksByStatus should hold (#154).
	currentStatus := make(map[string]string)

	// Process each event
	for _, event := range events {
		// Skip events before the window (zero cutoff ⇒ keep everything).
		if !cutoff.IsZero() && event.Timestamp.Before(cutoff) {
			continue
		}

		// Update last event timestamp
		if event.Timestamp.After(metrics.LastEventTimestamp) {
			metrics.LastEventTimestamp = event.Timestamp
		}

		switch event.Type {
		case EventTaskCreated:
			metrics.TasksCreated++

			// Extract task type if available
			if taskType, ok := event.Data["type"].(string); ok {
				metrics.TasksByType[taskType]++
			}

			// Extract initial status if available
			if status, ok := event.Data["status"].(string); ok {
				metrics.TasksByStatus[status]++
				if taskID, ok := event.Data["task_id"].(string); ok {
					currentStatus[taskID] = status
				}
			}

		case EventTaskCompleted:
			metrics.TasksCompleted++

		case EventTaskStatusChanged:
			// Track status changes
			taskID, hasTaskID := event.Data["task_id"].(string)
			oldStatus, hasOldStatus := event.Data["old_status"].(string)
			newStatus, hasNewStatus := event.Data["new_status"].(string)

			if hasTaskID && hasOldStatus && hasNewStatus {
				// Add to history
				if metrics.TaskStatusHistory[taskID] == nil {
					metrics.TaskStatusHistory[taskID] = []StatusChange{}
				}
				metrics.TaskStatusHistory[taskID] = append(
					metrics.TaskStatusHistory[taskID],
					StatusChange{
						Timestamp: event.Timestamp,
						OldStatus: oldStatus,
						NewStatus: newStatus,
					},
				)

				// Update status counts
				decrementStatus(metrics.TasksByStatus, oldStatus)
				metrics.TasksByStatus[newStatus]++
				currentStatus[taskID] = newStatus
			}

		case EventTaskArchived, EventTaskDeleted:
			// A retired task leaves the live status tally. Decrement whatever
			// bucket it currently sits in (its last status_changed, or its
			// created status if it never changed). Without this an archived or
			// deleted task is counted in its last live status forever (#154).
			if taskID, ok := event.Data["task_id"].(string); ok {
				decrementStatus(metrics.TasksByStatus, currentStatus[taskID])
				delete(currentStatus, taskID)
			}

		case EventTaskUnarchived:
			// The inverse of the archived case above, and the sibling half of
			// the same #154 defect: archiving decrements the task's bucket and
			// drops it from currentStatus, so unarchiving has to put it back.
			// Unarchive emits no task.status_changed, so while this case was
			// missing the task never re-entered TasksByStatus — `adb metrics`
			// under-reported the backlog by one per unarchived task, forever.
			if taskID, ok := event.Data["task_id"].(string); ok {
				status := unarchivedStatus
				if s, ok := event.Data["status"].(string); ok && s != "" {
					status = s
				}
				// A task the log claims is still live moves buckets rather than
				// being counted twice — the fold reads a log it does not own.
				if prev, live := currentStatus[taskID]; live {
					decrementStatus(metrics.TasksByStatus, prev)
				}
				metrics.TasksByStatus[status]++
				currentStatus[taskID] = status
			}

		case EventAgentSessionStarted:
			metrics.AgentSessions++

		case EventKnowledgeExtracted:
			metrics.KnowledgeExtracts++

		case EventWorktreeCreated:
			metrics.WorktreesCreated++

		case EventWorktreeRemoved:
			metrics.WorktreesRemoved++

		case EventTaskPriorityChanged,
			EventAgentSessionActive,
			EventAgentSessionEnded,
			EventIssueSynced,
			EventIssueConflict,
			EventIssueSkipped,
			EventStageAdvanced,
			EventStageOverride,
			EventConfigTaskContextSynced:
			// Deliberately not folded: Metrics has no field these contribute to.
			// Priority is not tallied; AgentSessions counts starts, so the
			// heartbeat and the end event would double-count it; issue-sync,
			// stage-gate governance and context-sync are separate surfaces
			// (`adb issues sync`, `adb governance`). Listed explicitly rather
			// than left to a default so adding an EventType to the schema
			// (schema.go) fails the exhaustive linter here and forces the
			// "does this move a metric?" decision.
		}
	}

	return metrics, nil
}

// decrementStatus lowers a status bucket by one, deleting it at zero so the map
// only ever holds live positive counts.
func decrementStatus(byStatus map[string]int, status string) {
	if status == "" {
		return
	}
	byStatus[status]--
	if byStatus[status] <= 0 {
		delete(byStatus, status)
	}
}

// GetTaskDuration calculates how long a task has been in a specific status
func (mc *MetricsCalculator) GetTaskDuration(taskID, status string) (time.Duration, error) {
	events, err := mc.eventLog.ReadAll()
	if err != nil {
		return 0, err
	}

	var lastStatusChange time.Time
	currentStatus := ""

	// Find when the task entered the given status
	for _, event := range events {
		if event.Type == EventTaskCreated {
			if tid, ok := event.Data["task_id"].(string); ok && tid == taskID {
				if s, ok := event.Data["status"].(string); ok {
					currentStatus = s
					lastStatusChange = event.Timestamp
				}
			}
		} else if event.Type == EventTaskStatusChanged {
			if tid, ok := event.Data["task_id"].(string); ok && tid == taskID {
				if newStatus, ok := event.Data["new_status"].(string); ok {
					currentStatus = newStatus
					lastStatusChange = event.Timestamp
				}
			}
		}
	}

	// If the task is currently in the requested status, return duration
	if currentStatus == status {
		return time.Since(lastStatusChange), nil
	}

	return 0, nil
}

// GetTasksInStatus returns task IDs currently in a specific status
func (mc *MetricsCalculator) GetTasksInStatus(status string) ([]string, error) {
	events, err := mc.eventLog.ReadAll()
	if err != nil {
		return nil, err
	}

	// Track current status of each task
	taskStatuses := make(map[string]string)

	for _, event := range events {
		switch event.Type {
		case EventTaskCreated:
			if taskID, ok := event.Data["task_id"].(string); ok {
				if s, ok := event.Data["status"].(string); ok {
					taskStatuses[taskID] = s
				}
			}
		case EventTaskStatusChanged:
			if taskID, ok := event.Data["task_id"].(string); ok {
				if newStatus, ok := event.Data["new_status"].(string); ok {
					taskStatuses[taskID] = newStatus
				}
			}
		case EventTaskArchived, EventTaskDeleted:
			// A retired task is no longer "in" any live status — drop it so alerts
			// (AlertEvaluator consumes this) stop firing "blocked/review for Nh"
			// on archived or deleted tasks forever (#154).
			if taskID, ok := event.Data["task_id"].(string); ok {
				delete(taskStatuses, taskID)
			}
		case EventTaskUnarchived:
			// The inverse of the delete above. Unarchive emits no
			// task.status_changed, so while this case was missing an unarchived
			// task stayed invisible to the alert evaluator forever:
			// backlog_too_large under-counted it and it could never raise a
			// stale/blocked alert again.
			if taskID, ok := event.Data["task_id"].(string); ok {
				status := unarchivedStatus
				if s, ok := event.Data["status"].(string); ok && s != "" {
					status = s
				}
				taskStatuses[taskID] = status
			}
		case EventTaskCompleted,
			EventTaskPriorityChanged,
			EventWorktreeCreated,
			EventWorktreeRemoved,
			EventKnowledgeExtracted,
			EventAgentSessionStarted,
			EventAgentSessionActive,
			EventAgentSessionEnded,
			EventIssueSynced,
			EventIssueConflict,
			EventIssueSkipped,
			EventStageAdvanced,
			EventStageOverride,
			EventConfigTaskContextSynced:
			// None of these move a task between live statuses. task.completed is
			// reserved and never emitted; the status change that accompanies a
			// close arrives as task.status_changed. Listed explicitly rather than
			// left to a default so a new EventType fails the exhaustive linter
			// here and forces the "does this change a task's live status?"
			// decision — which is exactly the question task.unarchived got wrong.
		}
	}

	// Filter by requested status
	var taskIDs []string
	for taskID, s := range taskStatuses {
		if s == status {
			taskIDs = append(taskIDs, taskID)
		}
	}

	return taskIDs, nil
}
