package issuesync

import "testing"

// TestProviderFor pins the resolved open question: sync ONLY tickets whose
// Task.Repo names a real GitHub or GitLab remote. Everything else -
// _local repo-less tickets, local-path repos (absolute or ./ or ../),
// enterprise-internal hosts (anything that isn't a github.com/gitlab remote),
// non-triple repos - returns ok=false and is skipped by the syncer.
//
// Task.Repo is the raw --repo argument at task-create time; it has ALREADY
// gone through DefaultGitWorktreeManager.NormalizeRepoPath (see
// internal/integration/worktree.go:86), so github/gitlab entries are the
// canonical <platform>/<org>/<repo> triple by construction. This filter
// still guards against un-normalised legacy entries and the assumed-triple
// fallback in NormalizeRepoPath.
func TestProviderFor(t *testing.T) {
	cases := []struct {
		repo      string
		wantOK    bool
		wantName  string
		wantOwner string
		wantRepo  string
	}{
		{"github.com/awslabs/mcp", true, "github", "awslabs", "mcp"},
		{"github.com/valter-silva-au/ai-dev-brain", true, "github", "valter-silva-au", "ai-dev-brain"},
		{"gitlab.com/group/proj", true, "gitlab", "group", "proj"},
		{"gitlab.example.com/team/service", true, "gitlab", "team", "service"}, // self-hosted GitLab

		// Skip cases (ok=false)
		{"", false, "", "", ""},
		{"/Users/me/local/repo", false, "", "", ""}, // absolute local
		{"./relative", false, "", "", ""},
		{"../also-local", false, "", "", ""},
		{"git.internal.example/team/thing", false, "", "", ""}, // enterprise-internal
		{"code.corp.example/packages/foo", false, "", "", ""},  // enterprise-internal
		{"github.com/awslabs", false, "", "", ""},              // not a full triple
		{"github.com/awslabs/mcp/extra/depth", false, "", "", ""},
		{"bitbucket.org/team/repo", false, "", "", ""}, // unsupported host
	}
	for _, c := range cases {
		p, owner, name, ok := ProviderFor(c.repo)
		if ok != c.wantOK {
			t.Errorf("ProviderFor(%q) ok=%v want %v", c.repo, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if p == nil {
			t.Errorf("ProviderFor(%q) returned nil provider despite ok=true", c.repo)
			continue
		}
		if p.Name() != c.wantName {
			t.Errorf("ProviderFor(%q) name=%q want %q", c.repo, p.Name(), c.wantName)
		}
		if owner != c.wantOwner || name != c.wantRepo {
			t.Errorf("ProviderFor(%q) owner/name=%s/%s want %s/%s",
				c.repo, owner, name, c.wantOwner, c.wantRepo)
		}
	}
}
