package cloudsync

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// RepoEntry is one row of the re-cloneable repos manifest. The manifest
// captures enough state (origin URL + HEAD sha + branch) that a `pull` can
// reconstruct the read-only mirror at exactly the archived commit.
type RepoEntry struct {
	Path string // <platform>/<org>/<repo>, relative to <workspace>/repos
	// Origin is the origin remote URL with any credential userinfo removed by
	// StripOriginCredentials — never a rewritten path, and still re-cloneable
	// (git's credential helper supplies the credential at clone time). It is
	// stripped at CAPTURE, so this field never holds a token even in memory.
	Origin string
	Head   string // current HEAD commit sha
	Branch string // current branch name (or detached-HEAD marker)
}

// GenerateManifest walks <basePath>/repos via the shared FindGitRepos
// helper and reads each clone's origin/HEAD/branch. Repos with no origin
// are skipped (nothing to re-clone from). Errors on individual repos are
// non-fatal — a broken clone must not fail the whole cloud push.
//
// SECURITY: `git remote get-url origin` returns whatever the clone was
// configured with, which for a token-embedding clone is the token itself. Each
// origin is passed through StripOriginCredentials before it is stored, so no
// RepoEntry ever carries a credential — the manifest that Push uploads cannot
// leak one. The emptiness test stays on the RAW output so a URL that is
// nothing but userinfo is still treated as "no origin" and skipped.
func GenerateManifest(basePath string) ([]RepoEntry, error) {
	reposRoot := filepath.Join(basePath, "repos")
	dirs, err := integration.FindGitRepos(reposRoot)
	if err != nil {
		return nil, fmt.Errorf("scan repos: %w", err)
	}
	var out []RepoEntry
	for _, d := range dirs {
		origin := gitOut(d, "remote", "get-url", "origin")
		if origin == "" {
			continue
		}
		rel, relErr := filepath.Rel(reposRoot, d)
		if relErr != nil {
			continue
		}
		out = append(out, RepoEntry{
			Path:   filepath.ToSlash(rel),
			Origin: StripOriginCredentials(origin),
			Head:   gitOut(d, "rev-parse", "HEAD"),
			Branch: gitOut(d, "rev-parse", "--abbrev-ref", "HEAD"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// FormatManifest renders entries as a header-terminated TSV with one row
// per repo: path\torigin\thead\tbranch. Order is the caller's — pass a
// sorted slice for a deterministic archive.
//
// SECURITY: origins are re-stripped here. GenerateManifest already strips at
// capture, so for the Push path this is a no-op (StripOriginCredentials is
// idempotent) — but this function renders the exact bytes that get uploaded,
// and a RepoEntry can also be built by hand. Making the artifact boundary
// enforce the property means no future caller has to remember to. Same
// defence-in-depth shape as stageFile's os.Lstat behind the walker's
// symlink skip.
func FormatManifest(entries []RepoEntry) string {
	var b strings.Builder
	b.WriteString("path\torigin\thead\tbranch\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", e.Path, StripOriginCredentials(e.Origin), e.Head, e.Branch)
	}
	return b.String()
}

// gitOut runs `git <args>` in dir and returns the trimmed stdout, or ""
// on any error. Non-fatal by design — the manifest tolerates weird repos.
func gitOut(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
