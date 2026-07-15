package issuesync

import (
	"strings"
	"testing"
)

// TestGitHubProvider_Get_ParsesJSON asserts we build the correct `gh issue
// view` argv AND parse the canned JSON. `run` is injected — no real gh call.
func TestGitHubProvider_Get_ParsesJSON(t *testing.T) {
	var gotArgs []string
	p := &githubProvider{run: func(args ...string) (string, error) {
		gotArgs = args
		return `{"number":7,"url":"https://github.com/o/r/issues/7","title":"hi","body":"b","state":"OPEN","labels":[{"name":"adb:blocked"}],"updatedAt":"2026-07-01T00:00:00Z"}`, nil
	}}
	iss, found, err := p.Get("o", "r", 7)
	if err != nil || !found {
		t.Fatalf("Get err=%v found=%v", err, found)
	}
	if iss.Number != 7 || iss.State != IssueOpen || iss.Title != "hi" || iss.URL != "https://github.com/o/r/issues/7" {
		t.Fatalf("parsed wrong: %+v", iss)
	}
	if len(iss.Labels) != 1 || iss.Labels[0] != "adb:blocked" {
		t.Fatalf("labels: %v", iss.Labels)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "issue view 7") || !strings.Contains(joined, "--repo o/r") {
		t.Errorf("argv wrong: %q", joined)
	}
	if !strings.Contains(joined, "--json number,url,title,body,state,labels,updatedAt") {
		t.Errorf("argv missing --json fields: %q", joined)
	}
}

// TestGitHubProvider_Get_Number0IsNotFound short-circuits the shell-out for
// unlinked tickets — critical because a real gh issue view 0 would error.
func TestGitHubProvider_Get_Number0IsNotFound(t *testing.T) {
	p := &githubProvider{run: func(args ...string) (string, error) {
		t.Fatal("should not shell out")
		return "", nil
	}}
	iss, found, err := p.Get("o", "r", 0)
	if err != nil || found {
		t.Fatalf("number 0 must be not-found, no shell-out; found=%v err=%v", found, err)
	}
	if iss.Number != 0 {
		t.Errorf("expected zero-value RemoteIssue, got %+v", iss)
	}
}

// TestGitHubProvider_Get_ClosedStateNormalised proves the "OPEN"/"CLOSED"
// (uppercase from gh) is folded to our lowercase enum. Regression guard —
// the plan's ghToRemote uses EqualFold; a straight `==` would drop this.
func TestGitHubProvider_Get_ClosedStateNormalised(t *testing.T) {
	p := &githubProvider{run: func(args ...string) (string, error) {
		return `{"number":9,"url":"u","title":"t","state":"CLOSED","labels":[],"updatedAt":"2026-07-01T00:00:00Z"}`, nil
	}}
	iss, _, _ := p.Get("o", "r", 9)
	if iss.State != IssueClosed {
		t.Errorf("state=%q want closed", iss.State)
	}
}

func TestGitHubProvider_Create_BuildsArgv(t *testing.T) {
	var gotArgs []string
	p := &githubProvider{run: func(args ...string) (string, error) {
		gotArgs = args
		return "https://github.com/o/r/issues/12\n", nil
	}}
	iss, err := p.Create("o", "r", RemoteIssue{
		Title:  "new",
		Body:   "body",
		Labels: []string{"priority:P1", "adb:in_progress"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if iss.Number != 12 {
		t.Errorf("expected number 12 parsed from URL, got %d", iss.Number)
	}
	if iss.URL != "https://github.com/o/r/issues/12" {
		t.Errorf("URL=%q", iss.URL)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"issue create", "--repo o/r", "--title new", "--body body",
		"--label priority:P1", "--label adb:in_progress"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q: %q", want, joined)
		}
	}
}

func TestGitHubProvider_Update_BuildsArgvAndTogglesState(t *testing.T) {
	var got [][]string
	// Update first does an `issue view` (to reconcile stale adb:/priority:
	// labels, #175). Return a current issue that carries a STALE adb: label
	// (adb:blocked) plus a maintainer label so we can assert the reconcile.
	p := &githubProvider{run: func(args ...string) (string, error) {
		got = append(got, append([]string(nil), args...))
		if len(args) > 0 && args[0] == "issue" && len(args) > 1 && args[1] == "view" {
			return `{"number":3,"labels":[{"name":"adb:blocked"},{"name":"priority:P1"},{"name":"keep-me"}]}`, nil
		}
		return "", nil
	}}

	// State=IssueClosed should trigger an issue close after edit.
	if _, err := p.Update("o", "r", 3, RemoteIssue{
		Title:  "t2",
		Body:   "b2",
		Labels: []string{"adb:done", "priority:P1"},
		State:  IssueClosed,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Shell-outs: view (label reconcile) + edit + close.
	if len(got) < 3 {
		t.Fatalf("expected three shell-outs (view + edit + close), got %d: %v", len(got), got)
	}
	editJoined := strings.Join(got[1], " ")
	for _, want := range []string{"issue edit 3", "--repo o/r", "--title t2", "--body b2", "--add-label adb:done"} {
		if !strings.Contains(editJoined, want) {
			t.Errorf("edit argv missing %q: %q", want, editJoined)
		}
	}
	// The stale adb:blocked must be removed; the still-wanted priority:P1 and the
	// maintainer's keep-me must NOT be removed (#175).
	if !strings.Contains(editJoined, "--remove-label adb:blocked") {
		t.Errorf("edit argv should remove the stale adb:blocked: %q", editJoined)
	}
	if strings.Contains(editJoined, "--remove-label priority:P1") {
		t.Errorf("edit argv must NOT remove a still-desired label priority:P1: %q", editJoined)
	}
	if strings.Contains(editJoined, "--remove-label keep-me") {
		t.Errorf("edit argv must NOT touch a maintainer label keep-me: %q", editJoined)
	}
	closeJoined := strings.Join(got[2], " ")
	if !strings.Contains(closeJoined, "issue close 3") || !strings.Contains(closeJoined, "--repo o/r") {
		t.Errorf("close argv wrong: %q", closeJoined)
	}

	// Reopen path when State=IssueOpen.
	got = nil
	if _, err := p.Update("o", "r", 3, RemoteIssue{Title: "t", Body: "b", State: IssueOpen}); err != nil {
		t.Fatalf("Update reopen: %v", err)
	}
	if len(got) < 3 {
		t.Fatalf("expected three shell-outs, got %d", len(got))
	}
	if !strings.Contains(strings.Join(got[len(got)-1], " "), "issue reopen 3") {
		t.Errorf("expected reopen, got %q", strings.Join(got[len(got)-1], " "))
	}
}

// TestGitHubProvider_ArgvNoTokenLeakage asserts the safety property at the
// argv boundary: `gh` calls NEVER carry --token, --with-token, or a path
// under ~/.config/gh. Auth is per-host and owned by gh; adb never handles
// it. This is the binding gh-hosts.yml-trap guard.
func TestGitHubProvider_ArgvNoTokenLeakage(t *testing.T) {
	var seen []string
	p := &githubProvider{run: func(args ...string) (string, error) {
		seen = append(seen, strings.Join(args, " "))
		return `{"number":1,"url":"u","title":"t","state":"OPEN","labels":[],"updatedAt":"2026-07-01T00:00:00Z"}`, nil
	}}
	_, _, _ = p.Get("o", "r", 1)
	_, _ = p.Create("o", "r", RemoteIssue{Title: "t", Body: "b", Labels: []string{"adb:in_progress"}})
	_, _ = p.Update("o", "r", 1, RemoteIssue{Title: "t2", Body: "b2", State: IssueOpen})

	forbidden := []string{"--token", "--with-token", ".config/gh", "hosts.yml", "GITHUB_TOKEN"}
	for _, argv := range seen {
		for _, f := range forbidden {
			if strings.Contains(argv, f) {
				t.Errorf("argv leaked auth-related token: %q contains %q", argv, f)
			}
		}
	}
}
