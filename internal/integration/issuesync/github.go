package issuesync

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// githubProvider shells out to the host's authenticated `gh` for issue
// operations. `run` is injectable so tests can assert argv shape and parse a
// canned JSON without ever spawning `gh`. This mirrors the os/exec pattern
// used by internal/integration/reposync.go for `git`.
type githubProvider struct {
	// run executes `gh <args...>` and returns stdout. Injectable for tests.
	run func(args ...string) (string, error)
}

// NewGitHubProvider returns a GitHubProvider that shells out to the host's
// authenticated `gh` (per-host auth; token owned by gh, never handled here).
func NewGitHubProvider() Provider {
	return &githubProvider{run: func(args ...string) (string, error) {
		out, err := exec.Command("gh", args...).Output()
		if err != nil {
			return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
		}
		return string(out), nil
	}}
}

func (p *githubProvider) Name() string { return "github" }

// ghIssue matches the JSON shape returned by
// `gh issue view <n> --json number,url,title,body,state,labels,updatedAt`.
type ghIssue struct {
	Number    int       `json:"number"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // OPEN / CLOSED
	Labels    []ghLabel `json:"labels"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type ghLabel struct {
	Name string `json:"name"`
}

func (p *githubProvider) Get(owner, name string, number int) (RemoteIssue, bool, error) {
	// Number 0 is the "unlinked" sentinel. Return not-found without shelling
	// out so the plan's create-remote path is exercised even in offline tests.
	if number == 0 {
		return RemoteIssue{}, false, nil
	}
	out, err := p.run("issue", "view", strconv.Itoa(number),
		"--repo", owner+"/"+name,
		"--json", "number,url,title,body,state,labels,updatedAt")
	if err != nil {
		return RemoteIssue{}, false, err
	}
	var gi ghIssue
	if err := json.Unmarshal([]byte(out), &gi); err != nil {
		return RemoteIssue{}, false, fmt.Errorf("parse gh issue: %w", err)
	}
	return ghToRemote(gi), true, nil
}

func (p *githubProvider) Create(owner, name string, want RemoteIssue) (RemoteIssue, error) {
	args := []string{"issue", "create",
		"--repo", owner + "/" + name,
		"--title", want.Title,
		"--body", want.Body,
	}
	for _, l := range want.Labels {
		args = append(args, "--label", l)
	}
	out, err := p.run(args...)
	if err != nil {
		return RemoteIssue{}, err
	}
	url := strings.TrimSpace(out)
	return RemoteIssue{
		Number: issueNumberFromURL(url),
		URL:    url,
		Title:  want.Title,
		Body:   want.Body,
		Labels: want.Labels,
		State:  IssueOpen,
	}, nil
}

func (p *githubProvider) Update(owner, name string, number int, want RemoteIssue) (RemoteIssue, error) {
	args := []string{"issue", "edit", strconv.Itoa(number),
		"--repo", owner + "/" + name,
		"--title", want.Title,
		"--body", want.Body,
	}
	// Reconcile the adb-owned labels rather than blindly adding: adb:<status>
	// and priority:<P> change KEYS as the ticket moves (adb:blocked ->
	// adb:review), so a pure --add-label accumulates stale keys on the remote.
	// A later pull's AdbLabelFrom would then read whichever adb: label gh lists
	// first and silently revert the local status (#175). So we --remove-label
	// every current adb:/priority: label that is NOT in the desired set, then
	// --add-label the desired ones. Non-adb maintainer labels are left untouched.
	current, _, gerr := p.Get(owner, name, number)
	if gerr == nil {
		wantSet := make(map[string]bool, len(want.Labels))
		for _, l := range want.Labels {
			wantSet[l] = true
		}
		for _, l := range current.Labels {
			if isAdbOwnedLabel(l) && !wantSet[l] {
				args = append(args, "--remove-label", l)
			}
		}
	}
	for _, l := range want.Labels {
		args = append(args, "--add-label", l)
	}
	if _, err := p.run(args...); err != nil {
		return RemoteIssue{}, err
	}
	// State reconcile: close or reopen to match desired state. Best-effort;
	// the primary write above is what carries the field changes.
	if want.State == IssueClosed {
		_, _ = p.run("issue", "close", strconv.Itoa(number), "--repo", owner+"/"+name)
	} else {
		_, _ = p.run("issue", "reopen", strconv.Itoa(number), "--repo", owner+"/"+name)
	}
	return want, nil
}

func ghToRemote(gi ghIssue) RemoteIssue {
	labels := make([]string, 0, len(gi.Labels))
	for _, l := range gi.Labels {
		labels = append(labels, l.Name)
	}
	state := IssueOpen
	if strings.EqualFold(gi.State, "CLOSED") {
		state = IssueClosed
	}
	return RemoteIssue{
		Number:    gi.Number,
		URL:       gi.URL,
		Title:     gi.Title,
		Body:      gi.Body,
		Labels:    labels,
		State:     state,
		UpdatedAt: gi.UpdatedAt,
	}
}

// issueNumberFromURL extracts the trailing integer from a .../issues/<n> URL.
// Returns 0 for a malformed URL rather than erroring — the caller records
// what it got in RemoteIssue.URL, and the syncer sees 0 as "unlinked" on
// subsequent reads (self-healing, but rare).
func issueNumberFromURL(url string) int {
	i := strings.LastIndex(url, "/")
	if i < 0 || i+1 >= len(url) {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(url[i+1:]))
	return n
}
