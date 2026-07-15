package issuesync

import (
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// statusLabelPrefix namespaces adb-owned labels so we never collide with a
// maintainer's own labels and can round-trip fine-grained adb status through
// them. Labels with this prefix are considered the source of truth on pull;
// labels without it are preserved but ignored.
const statusLabelPrefix = "adb:"

// priorityLabelPrefix namespaces the informational priority label.
const priorityLabelPrefix = "priority:"

// isAdbOwnedLabel reports whether a remote label is one adb manages (a status
// or priority label) and may therefore be reconciled/removed on push — as
// opposed to a maintainer's own label, which is always preserved.
func isAdbOwnedLabel(l string) bool {
	return strings.HasPrefix(l, statusLabelPrefix) || strings.HasPrefix(l, priorityLabelPrefix)
}

// StatusToState maps an adb status to the coarse remote issue state. Active
// statuses (backlog/in_progress/blocked/review) keep the issue open — the
// fine-grained status rides on an "adb:<status>" label so a later pull can
// restore it precisely. Terminal statuses (done/archived) close the issue.
func StatusToState(s models.TaskStatus) IssueState {
	switch s {
	case models.TaskStatusDone, models.TaskStatusArchived:
		return IssueClosed
	default:
		return IssueOpen
	}
}

// StatusLabel is the round-trip label carrying the fine-grained adb status
// (e.g. "adb:blocked"). Push writes it; pull reads it back via
// StateToStatus/AdbLabelFrom to restore the exact status.
func StatusLabel(s models.TaskStatus) string { return statusLabelPrefix + string(s) }

// PriorityLabel is the label carrying priority (e.g. "priority:P0"). Purely
// informational on the remote side — pull does not rewrite Task.Priority
// from it; the source of truth for priority stays local.
func PriorityLabel(p models.Priority) string { return priorityLabelPrefix + string(p) }

// StateToStatus is the inverse used on pull. Closed always dominates and
// maps to done. When open, an adb: status label (if present) restores the
// precise status; otherwise it defaults to in_progress — a best-effort
// choice for an open issue with no adb-owned status label.
func StateToStatus(state IssueState, adbLabel string) models.TaskStatus {
	if state == IssueClosed {
		return models.TaskStatusDone
	}
	if strings.HasPrefix(adbLabel, statusLabelPrefix) {
		if lbl := strings.TrimPrefix(adbLabel, statusLabelPrefix); lbl != "" {
			return models.TaskStatus(lbl)
		}
	}
	return models.TaskStatusInProgress
}

// AdbLabelFrom returns the first adb: status label found in labels, or "".
// It ignores non-adb labels so the reconcile can hand back a single string
// instead of a slice.
func AdbLabelFrom(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, statusLabelPrefix) {
			return l
		}
	}
	return ""
}
