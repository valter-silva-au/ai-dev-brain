package issuesync

import (
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Syncer orchestrates one reconcile per ticket. All I/O is behind callbacks
// so it is unit-testable with no network:
//   - `provider` (unexported, defaults to ProviderFor) resolves the sync
//     provider for a ticket's Repo — tests inject a fakeProvider.
//   - Body returns the issue body for a task (adb keeps it in context.md).
//   - Write persists a mutated task back to the backlog (BacklogManager.UpdateTask).
//   - Log records a reconcile decision (wired to App.EventLog.Log). The set
//     of keys the CLI wraps into the payload is fixed and audited by
//     TestSyncer_EventPayloadHasNoCredentials — auth-safety guard.
type Syncer struct {
	// provider selects a Provider for a repo; defaults to ProviderFor when nil.
	provider func(repo string) (Provider, string, string, bool)
	// Body returns the issue body for a task (adb keeps it in context.md).
	Body func(models.Task) string
	// WriteBody persists a pulled remote body back to the ticket's context.md.
	// Optional: when nil, a remote-wins pull applies title/status only and the
	// body change is not written locally (the pre-#176 behaviour). The CLI wires
	// it to write <ticketDir>/context.md so body is a real bidirectional field.
	WriteBody func(models.Task, string) error
	// Write persists a mutated task back to the backlog.
	Write func(models.Task) error
	// Log records a reconcile decision. Callers wrap this over App.EventLog.Log.
	Log func(event string, data map[string]interface{})
}

// Result is the per-ticket outcome (mirrors Decision + the resolved link).
type Result struct {
	TaskID string
	Action Action
	Reason string
}

func (s *Syncer) providerFor(repo string) (Provider, string, string, bool) {
	if s.provider != nil {
		return s.provider(repo)
	}
	return ProviderFor(repo)
}

// SyncTask reconciles one ticket end-to-end. When dryRun is true it computes
// and logs the decision but performs no remote write and no backlog write.
//
// Event payloads are limited to five keys (task_id, repo, provider, action,
// reason) — plus `error` on the provider-error branch. NEVER carry raw
// provider stdout/stderr, tokens, or hosts.yml paths (verified by
// TestSyncer_EventPayloadHasNoCredentials, the WS-E auth guard).
func (s *Syncer) SyncTask(tk models.Task, dir Direction, dryRun bool) Result {
	p, owner, name, ok := s.providerFor(tk.Repo)
	if !ok {
		s.Log(string(observability.EventIssueSkipped), map[string]interface{}{
			"task_id": tk.ID,
			"repo":    tk.Repo,
		})
		return Result{TaskID: tk.ID, Action: ActionNoop, Reason: "no gh/glab remote"}
	}

	body := s.Body(tk)
	remote, found, err := p.Get(owner, name, tk.RemoteIssue)
	if err != nil {
		// A provider error is logged as a conflict so an operator sees
		// the failure in the observability dashboard. Only err.Error() is
		// carried — provider implementations wrap the raw exec output as
		// "gh <args>: <exit-error>", never with a token.
		s.Log(string(observability.EventIssueConflict), map[string]interface{}{
			"task_id":  tk.ID,
			"repo":     tk.Repo,
			"provider": p.Name(),
			"error":    err.Error(),
		})
		return Result{TaskID: tk.ID, Action: ActionNoop, Reason: "provider get failed: " + err.Error()}
	}

	d := Reconcile(Input{
		Local: tk, Body: body, Remote: remote, RemoteFound: found,
		Baseline: tk.SyncHash, LocalUpdated: tk.Updated, Direction: dir,
	})

	// A pull triggered by BOTH-sides-changed is a conflict (the local edit
	// is discarded); a plain remote-only pull is a normal sync.
	evt := observability.EventIssueSynced
	if d.Action == ActionUpdateLocal && found {
		localChanged := tk.SyncHash == "" || SyncHash(tk, body) != tk.SyncHash
		if localChanged {
			evt = observability.EventIssueConflict
		}
	}
	s.Log(string(evt), map[string]interface{}{
		"task_id":  tk.ID,
		"repo":     tk.Repo,
		"provider": p.Name(),
		"action":   string(d.Action),
		"reason":   d.Reason,
	})

	if dryRun || d.Action == ActionNoop {
		return Result{TaskID: tk.ID, Action: d.Action, Reason: d.Reason}
	}

	want := RemoteIssue{
		Title:  tk.Title,
		Body:   body,
		Labels: []string{StatusLabel(tk.Status), PriorityLabel(tk.Priority)},
		State:  StatusToState(tk.Status),
	}

	switch d.Action {
	case ActionCreateRemote:
		created, cerr := p.Create(owner, name, want)
		if cerr != nil {
			return Result{TaskID: tk.ID, Action: ActionNoop, Reason: cerr.Error()}
		}
		tk.RemoteIssue, tk.RemoteURL = created.Number, created.URL
	case ActionUpdateRemote:
		if _, uerr := p.Update(owner, name, tk.RemoteIssue, want); uerr != nil {
			return Result{TaskID: tk.ID, Action: ActionNoop, Reason: uerr.Error()}
		}
	case ActionUpdateLocal:
		tk.Title = remote.Title
		tk.Status = StateToStatus(remote.State, AdbLabelFrom(remote.Labels))
		tk.RemoteURL = remote.URL
		// body is a documented bidirectional LWW field, so a remote-wins pull
		// must apply the remote body to the ticket's context.md — not silently
		// drop it (#176). Persist it and use it as the body the refreshed
		// SyncHash is computed from, so the pulled state is the new baseline.
		if s.WriteBody != nil && remote.Body != body {
			if werr := s.WriteBody(tk, remote.Body); werr != nil {
				return Result{TaskID: tk.ID, Action: ActionNoop, Reason: "write-body failed: " + werr.Error()}
			}
			body = remote.Body
		}
	}

	// Refresh the reconcile baseline after any successful write. LastSynced
	// is stamped from the local Updated so subsequent runs can compare
	// remote UpdatedAt against a stable local marker. body reflects the pulled
	// remote body on an ActionUpdateLocal (above), so the baseline matches what
	// is now on disk and the drift is not re-detected on the next sync.
	tk.SyncHash = SyncHash(tk, body)
	tk.LastSynced = tk.Updated
	if werr := s.Write(tk); werr != nil {
		return Result{TaskID: tk.ID, Action: d.Action, Reason: "write-back failed: " + werr.Error()}
	}
	return Result{TaskID: tk.ID, Action: d.Action, Reason: d.Reason}
}
