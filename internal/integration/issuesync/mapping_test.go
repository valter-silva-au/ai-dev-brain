package issuesync

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestStatusToState pins the roadmap mapping: the four active adb statuses
// (backlog, in_progress, blocked, review) all keep the issue OPEN;
// done/archived close it. Fine-grained status is preserved via an "adb:<n>"
// label — see StatusLabel below.
func TestStatusToState(t *testing.T) {
	cases := []struct {
		in   models.TaskStatus
		want IssueState
	}{
		{models.TaskStatusBacklog, IssueOpen},
		{models.TaskStatusInProgress, IssueOpen},
		{models.TaskStatusBlocked, IssueOpen},
		{models.TaskStatusReview, IssueOpen},
		{models.TaskStatusDone, IssueClosed},
		{models.TaskStatusArchived, IssueClosed},
	}
	for _, c := range cases {
		if got := StatusToState(c.in); got != c.want {
			t.Errorf("StatusToState(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestStatusLabel_And_PriorityLabel(t *testing.T) {
	if got := StatusLabel(models.TaskStatusBlocked); got != "adb:blocked" {
		t.Errorf("StatusLabel=%q want adb:blocked", got)
	}
	if got := PriorityLabel(models.PriorityP0); got != "priority:P0" {
		t.Errorf("PriorityLabel=%q want priority:P0", got)
	}
}

// TestStateToStatus_ClosedMapsToDone covers the inverse used on pull. A
// closed remote issue always maps to done. An open issue with an adb: status
// label restores the precise status; an open issue with no adb label defaults
// to in_progress.
func TestStateToStatus(t *testing.T) {
	cases := []struct {
		state    IssueState
		adbLabel string
		want     models.TaskStatus
	}{
		{IssueClosed, "", models.TaskStatusDone},
		{IssueClosed, "adb:blocked", models.TaskStatusDone}, // closed dominates
		{IssueOpen, "adb:blocked", models.TaskStatusBlocked},
		{IssueOpen, "adb:review", models.TaskStatusReview},
		{IssueOpen, "adb:backlog", models.TaskStatusBacklog},
		{IssueOpen, "", models.TaskStatusInProgress}, // best-effort default for open
	}
	for _, c := range cases {
		if got := StateToStatus(c.state, c.adbLabel); got != c.want {
			t.Errorf("StateToStatus(%q,%q)=%q want %q", c.state, c.adbLabel, got, c.want)
		}
	}
}

func TestAdbLabelFrom(t *testing.T) {
	if got := AdbLabelFrom([]string{"priority:P1", "adb:blocked", "bug"}); got != "adb:blocked" {
		t.Errorf("AdbLabelFrom=%q want adb:blocked", got)
	}
	if got := AdbLabelFrom([]string{"priority:P1", "bug"}); got != "" {
		t.Errorf("AdbLabelFrom=%q want empty", got)
	}
}
