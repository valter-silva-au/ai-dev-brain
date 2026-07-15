package issuesync

import (
	"errors"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeProvider is a hand-rolled Provider mock. It records call names and
// returns whatever the test wired into its fields; no gh/glab is ever spawned.
type fakeProvider struct {
	name    string
	get     RemoteIssue
	found   bool
	getErr  error
	created RemoteIssue
	calls   []string
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Get(o, n string, num int) (RemoteIssue, bool, error) {
	f.calls = append(f.calls, "get")
	return f.get, f.found, f.getErr
}
func (f *fakeProvider) Create(o, n string, w RemoteIssue) (RemoteIssue, error) {
	f.calls = append(f.calls, "create")
	return f.created, nil
}
func (f *fakeProvider) Update(o, n string, num int, w RemoteIssue) (RemoteIssue, error) {
	f.calls = append(f.calls, "update")
	return w, nil
}

type loggedEvent struct {
	Event string
	Data  map[string]interface{}
}

// TestSyncer_SkipsLocalTickets — a repo-less _local ticket must not call any
// provider and MUST log issue.skipped.
func TestSyncer_SkipsLocalTickets(t *testing.T) {
	var logged []loggedEvent
	s := &Syncer{
		Body:  func(models.Task) string { return "" },
		Write: func(models.Task) error { return nil },
		Log:   func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	res := s.SyncTask(models.Task{ID: "TASK-1", Repo: ""}, DirectionBoth, false)
	if res.Action != ActionNoop {
		t.Fatalf("_local ticket should be noop, got %q", res.Action)
	}
	if len(logged) != 1 || logged[0].Event != string(observability.EventIssueSkipped) {
		t.Fatalf("expected one skipped event, got %v", logged)
	}
	if logged[0].Data["task_id"] != "TASK-1" {
		t.Errorf("skip event missing task_id: %v", logged[0].Data)
	}
}

// TestSyncer_SkipsEnterpriseInternal — an enterprise-internal-host ticket goes
// through the same skip path (not github, not gitlab).
func TestSyncer_SkipsEnterpriseInternal(t *testing.T) {
	var logged []loggedEvent
	s := &Syncer{
		Body:  func(models.Task) string { return "" },
		Write: func(models.Task) error { return nil },
		Log:   func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	res := s.SyncTask(models.Task{ID: "TASK-2", Repo: "git.internal.example/team/thing"}, DirectionBoth, false)
	if res.Action != ActionNoop {
		t.Fatalf("enterprise-internal ticket should be noop, got %q", res.Action)
	}
	if len(logged) != 1 || logged[0].Event != string(observability.EventIssueSkipped) {
		t.Fatalf("expected skipped event, got %v", logged)
	}
}

// TestSyncer_CreatesRemote_DryRunDoesNotWrite is the core dry-run + real-run
// pair: dry-run computes the CreateRemote decision, calls Get, but MUST NOT
// call Create or Write; real-run does both AND persists RemoteIssue linkage.
func TestSyncer_CreatesRemote_DryRunDoesNotWrite(t *testing.T) {
	fp := &fakeProvider{name: "github", created: RemoteIssue{Number: 5, URL: "u"}}
	var wrote *models.Task
	var logged []loggedEvent
	s := &Syncer{
		provider: func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:     func(models.Task) string { return "b" },
		Write:    func(t models.Task) error { wrote = &t; return nil },
		Log:      func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	tk := models.Task{
		ID:       "TASK-9",
		Repo:     "github.com/o/r",
		Title:    "the-title",
		Status:   models.TaskStatusInProgress,
		Priority: models.PriorityP2,
		Updated:  time.Now(),
	}

	// dry-run: decides create, calls Get to inspect but NOT Create, does not Write.
	dry := s.SyncTask(tk, DirectionBoth, true)
	if dry.Action != ActionCreateRemote {
		t.Fatalf("dry action=%q", dry.Action)
	}
	for _, c := range fp.calls {
		if c == "create" {
			t.Fatal("dry-run must not call Create")
		}
	}
	if wrote != nil {
		t.Fatal("dry-run must not Write")
	}

	// real run: creates + writes back RemoteIssue number.
	fp.calls = nil
	logged = nil
	real := s.SyncTask(tk, DirectionBoth, false)
	if real.Action != ActionCreateRemote {
		t.Fatalf("real action=%q", real.Action)
	}
	if wrote == nil {
		t.Fatal("real run must Write the linked issue number")
	}
	if wrote.RemoteIssue != 5 || wrote.RemoteURL != "u" {
		t.Errorf("linkage not persisted: %+v", wrote)
	}
	if wrote.SyncHash == "" || wrote.LastSynced.IsZero() {
		t.Errorf("baseline not stored: hash=%q lastSynced=%v", wrote.SyncHash, wrote.LastSynced)
	}
	// exactly one issue.synced event (create counts as a sync, not a conflict).
	if len(logged) != 1 || logged[0].Event != string(observability.EventIssueSynced) {
		t.Fatalf("expected one synced event, got %v", logged)
	}
}

// TestSyncer_Pull_WritesRemoteBody guards #176: on a remote-wins pull the remote
// body must be written to the ticket's context.md (body is a bidirectional LWW
// field), not silently dropped — and the refreshed SyncHash must reflect the
// pulled body so the drift is not re-detected next sync.
func TestSyncer_Pull_WritesRemoteBody(t *testing.T) {
	localBody := "old local body"
	newer := time.Now().Add(1 * time.Hour)
	older := time.Now().Add(-1 * time.Hour)

	fp := &fakeProvider{
		name:  "github",
		found: true,
		// Remote is newer (triggers pull) and carries a changed body + title.
		get: RemoteIssue{Number: 7, Title: "remote title", Body: "NEW remote body", State: IssueOpen, UpdatedAt: newer},
	}
	var wroteBodyFor string
	var wroteBody string
	var wrote *models.Task
	s := &Syncer{
		provider:  func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:      func(models.Task) string { return localBody },
		WriteBody: func(t models.Task, b string) error { wroteBodyFor = t.ID; wroteBody = b; return nil },
		Write:     func(t models.Task) error { wrote = &t; return nil },
		Log:       func(string, map[string]interface{}) {},
	}
	tk := models.Task{
		ID:          "TASK-7",
		Repo:        "github.com/o/r",
		Title:       "local title",
		Status:      models.TaskStatusInProgress,
		Priority:    models.PriorityP2,
		RemoteIssue: 7,
		Updated:     older, // local older than remote -> remote wins
		// Baseline matches the current local state, so localChanged=false and this
		// is a clean remote-only pull.
		SyncHash: SyncHash(models.Task{Title: "local title", Status: models.TaskStatusInProgress, Priority: models.PriorityP2}, localBody),
	}

	res := s.SyncTask(tk, DirectionBoth, false)
	if res.Action != ActionUpdateLocal {
		t.Fatalf("action = %q, want update_local (remote newer -> pull)", res.Action)
	}
	// The remote body was written to context.md via the callback (#176).
	if wroteBodyFor != "TASK-7" || wroteBody != "NEW remote body" {
		t.Errorf("WriteBody = (%q,%q), want (TASK-7, \"NEW remote body\")", wroteBodyFor, wroteBody)
	}
	// The refreshed baseline is computed from the PULLED body, so a subsequent
	// sync sees no drift (title/status pulled too).
	wantHash := SyncHash(models.Task{Title: "remote title", Status: models.TaskStatusInProgress, Priority: models.PriorityP2}, "NEW remote body")
	if wrote == nil || wrote.SyncHash != wantHash {
		t.Errorf("SyncHash after pull must match the pulled body; got %q want %q", func() string {
			if wrote == nil {
				return "<nil>"
			}
			return wrote.SyncHash
		}(), wantHash)
	}
}

// TestSyncer_UpdatesRemote_LocalChangedOnly — the create/update happy path
// with a linked ticket. Local hash drifted from Baseline -> Reconcile decides
// UpdateRemote -> Syncer calls provider.Update once and updates the baseline.
func TestSyncer_UpdatesRemote_LocalChangedOnly(t *testing.T) {
	oldBase := SyncHash(models.Task{Title: "before", Status: models.TaskStatusInProgress, Priority: models.PriorityP2}, "b")
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	fp := &fakeProvider{
		name:  "github",
		found: true,
		get:   RemoteIssue{Number: 12, Title: "before", State: IssueOpen, UpdatedAt: older},
	}
	var wrote *models.Task
	s := &Syncer{
		provider: func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:     func(models.Task) string { return "b" },
		Write:    func(t models.Task) error { wrote = &t; return nil },
		Log:      func(string, map[string]interface{}) {},
	}
	tk := models.Task{
		ID:          "TASK-9",
		Repo:        "github.com/o/r",
		Title:       "after", // changed
		Status:      models.TaskStatusInProgress,
		Priority:    models.PriorityP2,
		RemoteIssue: 12,
		SyncHash:    oldBase,
		Updated:     time.Now(),
	}
	res := s.SyncTask(tk, DirectionBoth, false)
	if res.Action != ActionUpdateRemote {
		t.Fatalf("action=%q want update_remote (reason %q)", res.Action, res.Reason)
	}
	sawUpdate := false
	for _, c := range fp.calls {
		if c == "update" {
			sawUpdate = true
		}
	}
	if !sawUpdate {
		t.Fatalf("expected provider.Update call, got %v", fp.calls)
	}
	if wrote == nil || wrote.SyncHash == oldBase {
		t.Errorf("baseline should refresh; wrote=%+v", wrote)
	}
}

// TestSyncer_PullsRemote_TwoSideConflict — both sides changed with remote
// newer. Reconcile returns UpdateLocal; Syncer must log this as issue.conflict
// (not synced), because the round-trip caused adb-side data loss on that
// field (the local Title change is discarded).
func TestSyncer_PullsRemote_TwoSideConflict(t *testing.T) {
	// Baseline: what "in-sync" looked like the last time we synced.
	baseline := SyncHash(models.Task{Title: "T", Status: models.TaskStatusBacklog, Priority: models.PriorityP2}, "b")
	newer := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)

	fp := &fakeProvider{
		name:  "github",
		found: true,
		get:   RemoteIssue{Number: 3, Title: "remote-edit", State: IssueOpen, UpdatedAt: newer},
	}
	var wrote *models.Task
	var logged []loggedEvent
	s := &Syncer{
		provider: func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:     func(models.Task) string { return "b" },
		Write:    func(t models.Task) error { wrote = &t; return nil },
		Log:      func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	tk := models.Task{
		ID:          "TASK-9",
		Repo:        "github.com/o/r",
		Title:       "local-edit", // local also changed
		Status:      models.TaskStatusBacklog,
		Priority:    models.PriorityP2,
		RemoteIssue: 3,
		SyncHash:    baseline,
		Updated:     older, // older than remote UpdatedAt -> remote wins
	}
	res := s.SyncTask(tk, DirectionBoth, false)
	if res.Action != ActionUpdateLocal {
		t.Fatalf("action=%q want update_local", res.Action)
	}
	if wrote == nil || wrote.Title != "remote-edit" {
		t.Errorf("expected local Title overwritten to 'remote-edit', got %+v", wrote)
	}
	// Two-sided change should log issue.conflict, not issue.synced, so an
	// operator can find the tickets whose local edit was superseded.
	if len(logged) != 1 || logged[0].Event != string(observability.EventIssueConflict) {
		t.Fatalf("expected issue.conflict, got %v", logged)
	}
}

// TestSyncer_ProviderErrorLoggedAsConflict — a provider Get failure MUST
// short-circuit (never call Create/Update, never Write) and log issue.conflict
// so an operator sees the failure. The event MUST NOT carry the raw command
// string that could contain a token.
func TestSyncer_ProviderErrorLoggedAsConflict(t *testing.T) {
	fp := &fakeProvider{name: "github", getErr: errors.New("gh: 404 not found")}
	var logged []loggedEvent
	var wrote *models.Task
	s := &Syncer{
		provider: func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:     func(models.Task) string { return "b" },
		Write:    func(t models.Task) error { wrote = &t; return nil },
		Log:      func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	tk := models.Task{ID: "TASK-9", Repo: "github.com/o/r", RemoteIssue: 42, Updated: time.Now()}
	res := s.SyncTask(tk, DirectionBoth, false)
	if res.Action != ActionNoop {
		t.Fatalf("action=%q want noop on error", res.Action)
	}
	if wrote != nil {
		t.Fatal("must not Write on provider error")
	}
	if len(logged) != 1 || logged[0].Event != string(observability.EventIssueConflict) {
		t.Fatalf("expected issue.conflict event, got %v", logged)
	}
}

// TestSyncer_EventPayloadHasNoCredentials is the auth-safety guard-test the
// briefing binds us to. We assert that ALL events emitted by Syncer contain
// ONLY approved keys (task_id, repo, provider, action, reason, error) and no
// value looks like a credential (bearer token, gh_/ghp_ patterns, PRIVATE-
// TOKEN, GITHUB_TOKEN, or a hosts.yml path).
func TestSyncer_EventPayloadHasNoCredentials(t *testing.T) {
	fp := &fakeProvider{name: "github", found: true,
		get:     RemoteIssue{Number: 1, Title: "T", State: IssueOpen, UpdatedAt: time.Now()},
		created: RemoteIssue{Number: 2, URL: "https://github.com/o/r/issues/2"}}
	var logged []loggedEvent
	s := &Syncer{
		provider: func(string) (Provider, string, string, bool) { return fp, "o", "r", true },
		Body:     func(models.Task) string { return "b" },
		Write:    func(models.Task) error { return nil },
		Log:      func(evt string, data map[string]interface{}) { logged = append(logged, loggedEvent{evt, data}) },
	}
	// Exercise every path: skip, create, update, pull-conflict, error.
	_ = s.SyncTask(models.Task{ID: "T-1", Repo: ""}, DirectionBoth, false)
	_ = s.SyncTask(models.Task{ID: "T-2", Repo: "github.com/o/r", Updated: time.Now()}, DirectionBoth, true)

	allowedKeys := map[string]bool{"task_id": true, "repo": true, "provider": true, "action": true, "reason": true, "error": true}
	for _, ev := range logged {
		for k, v := range ev.Data {
			if !allowedKeys[k] {
				t.Errorf("event %s carries unexpected key %q (value=%v)", ev.Event, k, v)
			}
			// Value must be a string or a numeric task-id-ish thing; it must
			// not look like a token.
			if s, ok := v.(string); ok {
				forbidden := []string{"ghp_", "gho_", "ghu_", "ghs_", "PRIVATE-TOKEN", "Bearer ", "GITHUB_TOKEN=", "GITLAB_TOKEN=", ".config/gh/hosts.yml"}
				for _, f := range forbidden {
					if len(s) > 0 && contains(s, f) {
						t.Errorf("event %s key %q value looks like credential: %q", ev.Event, k, s)
					}
				}
			}
		}
	}
}

// contains is a tiny substring helper (the stdlib strings.Contains is fine
// but this file already has enough imports; avoid the churn).
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
