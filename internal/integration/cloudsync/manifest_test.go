package cloudsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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

	mkRepo := func(rel, originURL string) {
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

	mkRepo("github.com/awslabs/mcp", "https://github.com/awslabs/mcp.git")
	mkRepo("github.com/aws/aws-cli", "git@github.com:aws/aws-cli.git")

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
