package issuesync

import (
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// pTask is a small helper that builds a Task with just enough fields for the
// reconcile input (title/status/priority determine both StatusToState and the
// synced-fields hash).
func pTask(title string, s models.TaskStatus) models.Task {
	return models.Task{Title: title, Status: s, Priority: models.PriorityP2}
}

// TestSyncHash_StableAndSensitive nails the reconcile baseline: the hash is
// deterministic across identical inputs, changes when a synced field changes,
// and stays stable when a NON-synced field (Owner) changes. Without this the
// baseline check in Reconcile would be a coin-flip.
func TestSyncHash_StableAndSensitive(t *testing.T) {
	tk := pTask("a", models.TaskStatusBacklog)

	h1 := SyncHash(tk, "body")
	h2 := SyncHash(tk, "body")
	if h1 != h2 {
		t.Fatalf("hash not stable: %q != %q", h1, h2)
	}

	tk2 := tk
	tk2.Title = "b"
	if SyncHash(tk2, "body") == h1 {
		t.Errorf("hash should change when Title (a synced field) changes")
	}

	tk3 := tk
	tk3.Priority = models.PriorityP0
	if SyncHash(tk3, "body") == h1 {
		t.Errorf("hash should change when Priority (a synced field) changes")
	}

	// Body is passed separately (adb has no body field on Task; it lives in
	// the ticket's context.md).
	if SyncHash(tk, "different-body") == h1 {
		t.Errorf("hash should change when body changes")
	}

	// A non-synced field must NOT affect the hash.
	tk4 := tk
	tk4.Owner = "someone"
	tk4.Tags = []string{"x", "y"}
	if SyncHash(tk4, "body") != h1 {
		t.Errorf("hash changed on a non-synced field (Owner or Tags)")
	}
}

// TestReconcile is the core LWW test surface. Naming is
// {who-changed}-{direction}-{expected}, one row per decision-matrix cell.
//
// Change-detection semantics (fold-in #3, made explicit here):
//   - localChanged = SyncHash(local,body) != stored Baseline (proper baseline check).
//   - remoteChanged is HEURISTIC — because we don't store a remote-state hash
//     baseline, we approximate it with `RemoteIssue.UpdatedAt.After(LocalUpdated)`.
//     This is last-writer-wins by timestamp when both sides drifted, and is the
//     documented, tested limitation until a remote-hash baseline is added.
//   - Both changed -> newer timestamp wins; never-synced (Baseline "") is
//     always treated as localChanged so the first sync pushes.
func TestReconcile(t *testing.T) {
	base := SyncHash(pTask("T", models.TaskStatusBacklog), "body")
	newer := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)

	cases := []struct {
		name     string
		local    models.Task
		body     string
		remote   RemoteIssue
		found    bool
		baseline string
		localUpd time.Time
		dir      Direction
		want     Action
	}{
		// no-remote-yet cases
		{
			name:  "no-remote/both/creates",
			local: pTask("T", models.TaskStatusInProgress), body: "body", found: false,
			dir: DirectionBoth, want: ActionCreateRemote,
		},
		{
			name:  "no-remote/pull-only/noop",
			local: pTask("T", models.TaskStatusInProgress), body: "body", found: false,
			dir: DirectionPull, want: ActionNoop,
		},
		{
			name:  "no-remote/push-only/creates",
			local: pTask("T", models.TaskStatusInProgress), body: "body", found: false,
			dir: DirectionPush, want: ActionCreateRemote,
		},

		// in-sync case: baseline matches current hash, remote updated <= local
		{
			name:  "in-sync/both/noop",
			local: pTask("T", models.TaskStatusBacklog), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "T", State: IssueOpen, UpdatedAt: older},
			baseline: base, localUpd: older, dir: DirectionBoth, want: ActionNoop,
		},

		// local-only-changed
		{
			name:  "local-only/both/push",
			local: pTask("changed", models.TaskStatusReview), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "T", State: IssueOpen, UpdatedAt: older},
			baseline: base, localUpd: newer, dir: DirectionBoth, want: ActionUpdateRemote,
		},
		{
			name:  "local-only/pull-only/noop",
			local: pTask("changed", models.TaskStatusReview), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "T", State: IssueOpen, UpdatedAt: older},
			baseline: base, localUpd: newer, dir: DirectionPull, want: ActionNoop,
		},

		// remote-only-changed
		{
			name:  "remote-only/both/pull",
			local: pTask("T", models.TaskStatusBacklog), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "remote-edit", State: IssueOpen, UpdatedAt: newer},
			baseline: base, localUpd: older, dir: DirectionBoth, want: ActionUpdateLocal,
		},
		{
			name:  "remote-only/push-only/noop",
			local: pTask("T", models.TaskStatusBacklog), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "remote-edit", State: IssueOpen, UpdatedAt: newer},
			baseline: base, localUpd: older, dir: DirectionPush, want: ActionNoop,
		},

		// both-changed - LWW by timestamp
		{
			name:  "both-changed/both/remote-newer/pull",
			local: pTask("local-edit", models.TaskStatusReview), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "remote-edit", State: IssueOpen, UpdatedAt: newer},
			baseline: base, localUpd: older, dir: DirectionBoth, want: ActionUpdateLocal,
		},
		{
			name:  "both-changed/both/local-newer/push",
			local: pTask("local-edit", models.TaskStatusReview), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "remote-edit", State: IssueOpen, UpdatedAt: older},
			baseline: base, localUpd: newer, dir: DirectionBoth, want: ActionUpdateRemote,
		},

		// never-synced (empty baseline) with remote found - treat as localChanged
		{
			name:  "never-synced/both/found-remote/push",
			local: pTask("T", models.TaskStatusBacklog), body: "body", found: true,
			remote:   RemoteIssue{Number: 1, Title: "T", State: IssueOpen, UpdatedAt: older},
			baseline: "", localUpd: older, dir: DirectionBoth, want: ActionUpdateRemote,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Reconcile(Input{
				Local: c.local, Body: c.body, Remote: c.remote, RemoteFound: c.found,
				Baseline: c.baseline, LocalUpdated: c.localUpd, Direction: c.dir,
			})
			if d.Action != c.want {
				t.Fatalf("Action=%q want %q (reason %q)", d.Action, c.want, d.Reason)
			}
		})
	}
}
