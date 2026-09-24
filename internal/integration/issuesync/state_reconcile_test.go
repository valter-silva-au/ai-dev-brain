package issuesync

import (
	"fmt"
	"strings"
	"testing"
)

// The open/closed transition is how the `status` synced field actually lands on
// the remote — StatusToState is the mapping, and the close/reopen shell-out is
// the only call that applies it. While its error was discarded, a failed close
// made Update return (want, nil): the syncer recorded a successful sync and
// refreshed Task.SyncHash to the local state, so adb believed the remote was
// closed while it was still open and no later run would retry. Update is
// idempotent, so propagating the error lets the next `adb issues sync` converge
// and puts the reason in the Result an operator reads.

func TestGitHubProvider_Update_PropagatesCloseFailure(t *testing.T) {
	p := &githubProvider{run: func(args ...string) (string, error) {
		if len(args) > 1 && args[0] == "issue" && args[1] == "view" {
			return `{"number":3,"labels":[]}`, nil
		}
		if len(args) > 1 && args[1] == "close" {
			return "", fmt.Errorf("gh issue close 3: exit status 1")
		}
		return "", nil
	}}

	if _, err := p.Update("o", "r", 3, RemoteIssue{Title: "t", Body: "b", State: IssueClosed}); err == nil {
		t.Fatal("Update must report a failed close: the status field did not reach the remote")
	} else if !strings.Contains(err.Error(), "close") {
		t.Errorf("error should name the failed operation, got %q", err)
	}
}

func TestGitHubProvider_Update_PropagatesReopenFailure(t *testing.T) {
	p := &githubProvider{run: func(args ...string) (string, error) {
		if len(args) > 1 && args[0] == "issue" && args[1] == "view" {
			return `{"number":3,"labels":[]}`, nil
		}
		if len(args) > 1 && args[1] == "reopen" {
			return "", fmt.Errorf("gh issue reopen 3: exit status 1")
		}
		return "", nil
	}}

	if _, err := p.Update("o", "r", 3, RemoteIssue{Title: "t", Body: "b", State: IssueOpen}); err == nil {
		t.Fatal("Update must report a failed reopen")
	}
}

func TestGitLabProvider_Update_PropagatesCloseFailure(t *testing.T) {
	p := &gitlabProvider{run: func(args ...string) (string, error) {
		if len(args) > 1 && args[1] == "close" {
			return "", fmt.Errorf("glab issue close 3: exit status 1")
		}
		return "", nil
	}}

	if _, err := p.Update("o", "r", 3, RemoteIssue{Title: "t", Body: "b", State: IssueClosed}); err == nil {
		t.Fatal("Update must report a failed close: the status field did not reach the remote")
	} else if !strings.Contains(err.Error(), "close") {
		t.Errorf("error should name the failed operation, got %q", err)
	}
}

// TestProviders_Update_StateReconcileErrorCarriesNoCredential keeps the
// issuesync auth boundary intact on the path this change added: the propagated
// error is the provider's own "<cli> <args>: <exit-error>" wrapping, and the
// argv it names carries no token.
func TestProviders_Update_StateReconcileErrorCarriesNoCredential(t *testing.T) {
	forbidden := []string{"--token", "--with-token", ".config/gh", "hosts.yml", "GITHUB_TOKEN", "GITLAB_TOKEN", "PRIVATE-TOKEN"}

	gh := &githubProvider{run: func(args ...string) (string, error) {
		if len(args) > 1 && args[0] == "issue" && args[1] == "view" {
			return `{"number":1,"labels":[]}`, nil
		}
		return "", fmt.Errorf("gh %s: exit status 1", strings.Join(args, " "))
	}}
	gl := &gitlabProvider{run: func(args ...string) (string, error) {
		return "", fmt.Errorf("glab %s: exit status 1", strings.Join(args, " "))
	}}

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"github", func() error {
			_, err := gh.Update("o", "r", 1, RemoteIssue{Title: "t", Body: "b", State: IssueClosed})
			return err
		}},
		{"gitlab", func() error {
			_, err := gl.Update("o", "r", 1, RemoteIssue{Title: "t", Body: "b", State: IssueClosed})
			return err
		}},
	} {
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: expected an error to inspect", tc.name)
		}
		for _, f := range forbidden {
			if strings.Contains(err.Error(), f) {
				t.Errorf("%s: error leaked auth-related token: %q contains %q", tc.name, err, f)
			}
		}
	}
}
