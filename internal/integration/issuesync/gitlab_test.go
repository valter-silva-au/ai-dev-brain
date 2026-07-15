package issuesync

import (
	"strings"
	"testing"
)

// TestGitLabProvider_Get_ParsesJSON — mirrors the github variant with
// `glab issue view <n> -R o/r -F json` and lowercase state.
func TestGitLabProvider_Get_ParsesJSON(t *testing.T) {
	var gotArgs []string
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		gotArgs = args
		return `{"iid":11,"web_url":"https://gitlab.com/o/r/-/issues/11","title":"gl","description":"d","state":"opened","labels":["adb:review"],"updated_at":"2026-07-01T00:00:00Z"}`, nil
	}}
	iss, found, err := p.Get("o", "r", 11)
	if err != nil || !found {
		t.Fatalf("Get err=%v found=%v", err, found)
	}
	if iss.Number != 11 || iss.State != IssueOpen || iss.Title != "gl" {
		t.Fatalf("parsed wrong: %+v", iss)
	}
	if len(iss.Labels) != 1 || iss.Labels[0] != "adb:review" {
		t.Fatalf("labels: %v", iss.Labels)
	}
	if iss.Body != "d" {
		t.Fatalf("body from description: %q", iss.Body)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "issue view 11") || !strings.Contains(joined, "-R o/r") || !strings.Contains(joined, "-F json") {
		t.Errorf("argv wrong: %q", joined)
	}
}

func TestGitLabProvider_Get_Number0IsNotFound(t *testing.T) {
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		t.Fatal("should not shell out")
		return "", nil
	}}
	_, found, err := p.Get("o", "r", 0)
	if err != nil || found {
		t.Fatalf("number 0 must be not-found; found=%v err=%v", found, err)
	}
}

func TestGitLabProvider_Get_ClosedStateNormalised(t *testing.T) {
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		return `{"iid":9,"web_url":"u","title":"t","description":"","state":"closed","labels":[],"updated_at":"2026-07-01T00:00:00Z"}`, nil
	}}
	iss, _, _ := p.Get("o", "r", 9)
	if iss.State != IssueClosed {
		t.Errorf("state=%q want closed", iss.State)
	}
}

func TestGitLabProvider_Create_BuildsArgv(t *testing.T) {
	var gotArgs []string
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		gotArgs = args
		return "https://gitlab.com/o/r/-/issues/22\n", nil
	}}
	iss, err := p.Create("o", "r", RemoteIssue{
		Title:  "new",
		Body:   "body",
		Labels: []string{"priority:P1", "adb:in_progress"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if iss.Number != 22 {
		t.Errorf("expected number 22 parsed from URL, got %d", iss.Number)
	}
	joined := strings.Join(gotArgs, " ")
	// glab uses a single --label flag with comma-separated values, not
	// per-label repetition like gh.
	for _, want := range []string{"issue create", "-R o/r", "--title new", "--description body", "--label priority:P1,adb:in_progress"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q: %q", want, joined)
		}
	}
}

func TestGitLabProvider_Update_BuildsArgvAndTogglesState(t *testing.T) {
	var got [][]string
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		got = append(got, append([]string(nil), args...))
		return "", nil
	}}
	if _, err := p.Update("o", "r", 3, RemoteIssue{
		Title:  "t2",
		Body:   "b2",
		Labels: []string{"adb:done"},
		State:  IssueClosed,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected two shell-outs (update + close), got %d", len(got))
	}
	editJoined := strings.Join(got[0], " ")
	for _, want := range []string{"issue update 3", "-R o/r", "--title t2", "--description b2", "--label adb:done"} {
		if !strings.Contains(editJoined, want) {
			t.Errorf("update argv missing %q: %q", want, editJoined)
		}
	}
	if !strings.Contains(strings.Join(got[1], " "), "issue close 3") {
		t.Errorf("close argv wrong: %q", strings.Join(got[1], " "))
	}
}

// TestGitLabProvider_ArgvNoTokenLeakage — twin of the gh guard.
func TestGitLabProvider_ArgvNoTokenLeakage(t *testing.T) {
	var seen []string
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		seen = append(seen, strings.Join(args, " "))
		return `{"iid":1,"web_url":"u","title":"t","description":"","state":"opened","labels":[],"updated_at":"2026-07-01T00:00:00Z"}`, nil
	}}
	_, _, _ = p.Get("o", "r", 1)
	_, _ = p.Create("o", "r", RemoteIssue{Title: "t", Body: "b", Labels: []string{"adb:in_progress"}})
	_, _ = p.Update("o", "r", 1, RemoteIssue{Title: "t2", Body: "b2", State: IssueOpen})

	forbidden := []string{"--token", ".config/glab-cli", "config.yml", "GITLAB_TOKEN", "PRIVATE-TOKEN"}
	for _, argv := range seen {
		for _, f := range forbidden {
			if strings.Contains(argv, f) {
				t.Errorf("argv leaked auth-related token: %q contains %q", argv, f)
			}
		}
	}
}
