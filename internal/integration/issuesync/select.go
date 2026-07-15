package issuesync

import "strings"

// ProviderFor resolves the sync provider for a ticket's platform-qualified
// Repo. It returns ok=false — meaning "skip this ticket" — for repo-less
// (_local) tasks, local-path repos, non-triple repos, and any host that is
// NOT github.com or a gitlab host (e.g. an enterprise-internal git host that
// is not gh/glab-backed). This is the resolved open question: only tickets
// whose repo names a real GitHub/GitLab remote are synced.
//
// Task.Repo is normalised to <platform>/<org>/<repo> at task-create time by
// DefaultGitWorktreeManager.NormalizeRepoPath (internal/integration/worktree.go:86);
// this function is defensive against un-normalised legacy entries and the
// assumed-triple fallback in NormalizeRepoPath.
func ProviderFor(repo string) (p Provider, owner, name string, ok bool) {
	repo = strings.TrimSuffix(repo, "/")
	if repo == "" ||
		strings.HasPrefix(repo, "/") ||
		strings.HasPrefix(repo, "./") ||
		strings.HasPrefix(repo, "../") {
		return nil, "", "", false
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 3 {
		return nil, "", "", false
	}
	host, owner, name := parts[0], parts[1], parts[2]
	if owner == "" || name == "" {
		return nil, "", "", false
	}
	switch {
	case host == "github.com":
		return NewGitHubProvider(), owner, name, true
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		return NewGitLabProvider(), owner, name, true
	default:
		return nil, "", "", false
	}
}
