package models

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestTask_SyncFields_YAMLRoundTrip pins the four issue-sync fields (WS-E) so
// they survive a backlog.yaml round-trip via the yaml.v3 codec, matching how
// FileBacklogManager persists tasks.
func TestTask_SyncFields_YAMLRoundTrip(t *testing.T) {
	in := Task{
		ID:          "TASK-00099",
		Title:       "sync me",
		Type:        TaskTypeFeat,
		Status:      TaskStatusBacklog,
		Priority:    PriorityP2,
		RemoteIssue: 42,
		RemoteURL:   "https://github.com/valter-silva-au/ai-dev-brain/issues/42",
		LastSynced:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		SyncHash:    "abc123",
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Task
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RemoteIssue != 42 {
		t.Errorf("RemoteIssue=%d want 42", out.RemoteIssue)
	}
	if out.RemoteURL != in.RemoteURL {
		t.Errorf("RemoteURL=%q want %q", out.RemoteURL, in.RemoteURL)
	}
	if out.SyncHash != "abc123" {
		t.Errorf("SyncHash=%q want abc123", out.SyncHash)
	}
	if !out.LastSynced.Equal(in.LastSynced) {
		t.Errorf("LastSynced=%v want %v", out.LastSynced, in.LastSynced)
	}
}

// TestTask_SyncFields_OmitemptyOnZero locks in that a task that has never been
// synced does NOT emit the four sync keys — old backlog entries created before
// WS-E stay stable, and the file diff on task creation is unchanged.
func TestTask_SyncFields_OmitemptyOnZero(t *testing.T) {
	data, err := yaml.Marshal(Task{
		ID:       "TASK-1",
		Type:     TaskTypeFeat,
		Status:   TaskStatusBacklog,
		Priority: PriorityP2,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rendered := string(data)
	for _, k := range []string{"remote_issue", "remote_url", "last_synced", "sync_hash"} {
		if strings.Contains(rendered, k) {
			t.Errorf("zero-value task should omit %q, got:\n%s", k, rendered)
		}
	}
}
