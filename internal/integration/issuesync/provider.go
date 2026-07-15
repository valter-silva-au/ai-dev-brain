// Package issuesync reconciles adb tickets with GitHub/GitLab issues.
//
// Providers shell out to the host's `gh` / `glab` (the same os/exec model
// used by internal/integration/reposync.go — this package is the FIRST gh/glab
// consumer in adb; there is no prior gh precedent). Auth is per-host and
// owned by the CLIs; we never read ~/.config/gh/hosts.yml, never accept a
// --token flag, and never write a token or PII into backlog.yaml, status.yaml,
// or the .events.jsonl event log.
//
// Reconcile is pure and last-writer-wins over a fixed synced-fields allowlist
// (title, body, labels, status, priority). Status maps as
// backlog/in_progress/blocked/review -> open + adb: label ; done/archived ->
// closed. Change-detection uses a stored per-sync baseline (Task.SyncHash)
// rather than the local Updated timestamp, so a local edit made after the
// last sync is detectable regardless of remote-clock skew.
package issuesync

import "time"

// IssueState is the coarse open/closed a remote issue can be in. Fine-grained
// adb status (blocked, review, in_progress, backlog) rides an "adb:<status>"
// label so a closed->open remote toggle can restore the exact adb status on
// pull.
type IssueState string

const (
	IssueOpen   IssueState = "open"
	IssueClosed IssueState = "closed"
)

// RemoteIssue is the provider-agnostic view of an issue. Number == 0 means
// "no remote issue yet" — a push will create one via Provider.Create. URL is
// the issue's html_url, echoed back into Task.RemoteURL after a create/update.
type RemoteIssue struct {
	Number    int
	URL       string
	Title     string
	Body      string
	Labels    []string
	State     IssueState
	UpdatedAt time.Time
}

// Provider is the GitHub/GitLab seam. owner/name are the <org>/<repo> pair
// resolved from the ticket's platform-qualified Repo (canonicalised at
// task-create time by DefaultGitWorktreeManager.NormalizeRepoPath, so an
// unlinked ticket's Repo is already `github.com/org/name` for github tasks).
// All methods are allowed to shell out; unit tests use a fake implementation
// (see issuesync_test.go's fakeProvider).
type Provider interface {
	// Name identifies the backend for logging ("github" / "gitlab").
	Name() string
	// Get fetches the issue by number. found=false (nil error) when number is
	// 0 or the remote has no such issue — callers use this to decide between
	// create-remote and update-remote.
	Get(owner, name string, number int) (issue RemoteIssue, found bool, err error)
	// Create opens a new issue and returns it (with the assigned Number/URL).
	Create(owner, name string, want RemoteIssue) (RemoteIssue, error)
	// Update pushes title/body/labels/state onto an existing issue.
	Update(owner, name string, number int, want RemoteIssue) (RemoteIssue, error)
}
