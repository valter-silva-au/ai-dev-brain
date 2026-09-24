package cloudsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mkRepoWithOrigin creates a throwaway git repo at reposRoot/rel with one
// empty commit and `origin` set to originURL. Shared by the manifest tests so
// they exercise the same real-git capture path GenerateManifest uses.
func mkRepoWithOrigin(t *testing.T, reposRoot, rel, originURL string) {
	t.Helper()
	full := filepath.Join(reposRoot, rel)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = full
		// Silence git config warnings under tmpdir.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, full, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "--allow-empty", "-q", "-m", "init")
	run("remote", "add", "origin", originURL)
}

// TestFormatManifest is a pure formatter test — no I/O, no git — so the
// TSV shape is pinned independently of the walker.
func TestFormatManifest(t *testing.T) {
	entries := []RepoEntry{
		{Path: "github.com/awslabs/mcp", Origin: "https://github.com/awslabs/mcp.git", Head: "abc123", Branch: "main"},
		{Path: "github.com/aws/aws-cli", Origin: "git@github.com:aws/aws-cli.git", Head: "def456", Branch: "develop"},
	}
	got := FormatManifest(entries)
	want := "path\torigin\thead\tbranch\n" +
		"github.com/awslabs/mcp\thttps://github.com/awslabs/mcp.git\tabc123\tmain\n" +
		"github.com/aws/aws-cli\tgit@github.com:aws/aws-cli.git\tdef456\tdevelop\n"
	if got != want {
		t.Errorf("FormatManifest mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestGenerateManifest creates two throwaway repos with fake origins and
// asserts the walker returns them sorted, with origin/HEAD/branch. Skips
// automatically when git is not on PATH so the CI matrix stays green.
func TestGenerateManifest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	base := t.TempDir()
	reposRoot := filepath.Join(base, "repos")

	mkRepoWithOrigin(t, reposRoot, "github.com/awslabs/mcp", "https://github.com/awslabs/mcp.git")
	mkRepoWithOrigin(t, reposRoot, "github.com/aws/aws-cli", "git@github.com:aws/aws-cli.git")

	got, err := GenerateManifest(base)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	// Sorted by Path.
	if got[0].Path != "github.com/aws/aws-cli" || got[1].Path != "github.com/awslabs/mcp" {
		t.Errorf("wrong sort order: %+v", got)
	}
	if got[0].Origin != "git@github.com:aws/aws-cli.git" {
		t.Errorf("origin[0] = %q", got[0].Origin)
	}
	if got[0].Head == "" || len(got[0].Head) < 7 {
		t.Errorf("head[0] = %q, want a commit sha", got[0].Head)
	}
	if got[0].Branch != "main" {
		t.Errorf("branch[0] = %q, want main", got[0].Branch)
	}
}

// TestGenerateManifest_StripsCredentialsFromRenderedManifest is the guard for
// the leak this fix closes: a clone configured with an embedded token must not
// put that token into the bytes that Push uploads as repos-manifest.tsv.
//
// It deliberately asserts on the SERIALISED manifest — the exact string
// sync.Push writes to the staging dir and then Puts — rather than on the
// helper or even on RepoEntry.Origin. A guard that only checked the helper
// would keep passing if a future refactor stopped calling it on the capture
// path, which is precisely the bug class here.
//
// The re-cloneability half is proved by comparison rather than by assertion:
// the credentialed clone's row must be BYTE-IDENTICAL to the row of a clone of
// the same repo configured with no credentials at all. That is the URL a
// credential-helper-based clone uses, so matching it is the strongest
// available offline proof that what remains still clones.
func TestGenerateManifest_StripsCredentialsFromRenderedManifest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	// fakeToken (origin_test.go) is obviously-fake credential material.
	const fakeUser = "svcuser"

	base := t.TempDir()
	reposRoot := filepath.Join(base, "repos")
	mkRepoWithOrigin(t, reposRoot, "clean/org/repo", "https://github.com/org/repo.git")
	mkRepoWithOrigin(t, reposRoot, "userpass/org/repo",
		"https://"+fakeUser+":"+fakeToken+"@github.com/org/repo.git")
	mkRepoWithOrigin(t, reposRoot, "baretoken/org/repo",
		"https://"+fakeToken+"@github.com/org/repo.git")

	entries, err := GenerateManifest(base)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	body := FormatManifest(entries)

	// 1. The credential must not appear anywhere in the uploaded bytes.
	for _, secret := range []string{fakeToken, fakeUser + ":" + fakeToken, fakeUser + "@"} {
		if strings.Contains(body, secret) {
			t.Errorf("rendered manifest leaks %q:\n%s", secret, body)
		}
	}
	// 2. Nor may any userinfo survive at all — "@" in a manifest origin can
	//    only come from userinfo or from an scp-style remote, and there is no
	//    scp-style remote in this fixture.
	if strings.Contains(body, "@") {
		t.Errorf("rendered manifest still carries userinfo:\n%s", body)
	}
	// 3. What remains must still be re-cloneable: identical to the row a
	//    credential-free clone of the same repo produces.
	const wantURL = "https://github.com/org/repo.git"
	byPath := map[string]string{}
	for _, e := range entries {
		byPath[e.Path] = e.Origin
	}
	for _, p := range []string{"clean/org/repo", "userpass/org/repo", "baretoken/org/repo"} {
		if got := byPath[p]; got != wantURL {
			t.Errorf("origin for %s = %q, want the credential-free URL %q", p, got, wantURL)
		}
		if !strings.Contains(body, "\t"+wantURL+"\t") {
			t.Errorf("rendered manifest lost the clone-able URL for %s:\n%s", p, body)
		}
	}
}

// TestGenerateManifest_MissingReposRoot returns no entries and no error
// when there is no repos/ dir at all — a fresh workspace with nothing
// cloned should not fail the manifest step.
func TestGenerateManifest_MissingReposRoot(t *testing.T) {
	base := t.TempDir()
	got, err := GenerateManifest(base)
	if err != nil {
		t.Fatalf("GenerateManifest on empty basePath: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 entries, got %+v", got)
	}
}
