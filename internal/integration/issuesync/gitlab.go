package issuesync

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitlabProvider is the GitLab counterpart to githubProvider — same shape,
// different argv (`glab issue view <n> -R <owner>/<name> -F json`) and JSON
// keys (iid, web_url, description, opened/closed).
type gitlabProvider struct {
	run func(args ...string) (string, error)
}

// NewGitLabProvider shells out to the host's authenticated `glab`.
func NewGitLabProvider() Provider {
	return &gitlabProvider{run: func(args ...string) (string, error) {
		out, err := exec.Command("glab", args...).Output()
		if err != nil {
			return "", fmt.Errorf("glab %s: %w", strings.Join(args, " "), err)
		}
		return string(out), nil
	}}
}

func (p *gitlabProvider) Name() string { return "gitlab" }

// glIssue matches the JSON shape from `glab issue view <iid> -R o/r -F json`.
type glIssue struct {
	IID       int       `json:"iid"`
	WebURL    string    `json:"web_url"`
	Title     string    `json:"title"`
	Body      string    `json:"description"`
	State     string    `json:"state"` // opened / closed
	Labels    []string  `json:"labels"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p *gitlabProvider) Get(owner, name string, number int) (RemoteIssue, bool, error) {
	if number == 0 {
		return RemoteIssue{}, false, nil
	}
	out, err := p.run("issue", "view", strconv.Itoa(number),
		"-R", owner+"/"+name, "-F", "json")
	if err != nil {
		return RemoteIssue{}, false, err
	}
	var gi glIssue
	if err := json.Unmarshal([]byte(out), &gi); err != nil {
		return RemoteIssue{}, false, fmt.Errorf("parse glab issue: %w", err)
	}
	state := IssueOpen
	if gi.State == "closed" {
		state = IssueClosed
	}
	return RemoteIssue{
		Number:    gi.IID,
		URL:       gi.WebURL,
		Title:     gi.Title,
		Body:      gi.Body,
		Labels:    gi.Labels,
		State:     state,
		UpdatedAt: gi.UpdatedAt,
	}, true, nil
}

func (p *gitlabProvider) Create(owner, name string, want RemoteIssue) (RemoteIssue, error) {
	args := []string{"issue", "create", "-R", owner + "/" + name,
		"--title", want.Title, "--description", want.Body}
	if len(want.Labels) > 0 {
		args = append(args, "--label", strings.Join(want.Labels, ","))
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

func (p *gitlabProvider) Update(owner, name string, number int, want RemoteIssue) (RemoteIssue, error) {
	args := []string{"issue", "update", strconv.Itoa(number),
		"-R", owner + "/" + name,
		"--title", want.Title,
		"--description", want.Body,
	}
	if len(want.Labels) > 0 {
		args = append(args, "--label", strings.Join(want.Labels, ","))
	}
	if _, err := p.run(args...); err != nil {
		return RemoteIssue{}, err
	}
	verb := "reopen"
	if want.State == IssueClosed {
		verb = "close"
	}
	_, _ = p.run("issue", verb, strconv.Itoa(number), "-R", owner+"/"+name)
	return want, nil
}
