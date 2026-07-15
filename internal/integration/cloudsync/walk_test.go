package cloudsync

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// TestWalkUploadSet asserts the walker returns only allowlisted paths and
// SKIPS denied directories entirely (never descends into communications/,
// sessions/, work/, repos/, .omnictx/, .adb/) — so a huge repos/ mirror
// never gets stat'd, and a symlink inside communications/ can't be followed.
func TestWalkUploadSet(t *testing.T) {
	root := t.TempDir()

	// Build a fixture that exercises include roots + hard denials + traversal.
	files := map[string]string{
		"raw/articles/x.md": "raw",
		"tickets/github.com/awslabs/mcp/TASK-1-x/context.md":              "ok",
		"tickets/github.com/awslabs/mcp/TASK-1-x/communications/slack.md": "PRIVATE",
		"tickets/x/sessions/2026.jsonl":                                   "PRIVATE",
		"work/github.com/o/r/TASK-1/main.go":                              "PRIVATE",
		"repos/github.com/o/r/README.md":                                  "PRIVATE",
		".omnictx/corpus.enc":                                             "PRIVATE",
		".env":                                                            "SECRET",
		".env.local":                                                      "SECRET",
		"backlog.yaml":                                                    "PRIVATE",
		"CLAUDE.md":                                                       "root config",
		"Taskfile.yaml":                                                   "root config",
		"wiki/index.md":                                                   "wiki",
		"skills/cohere/SKILL.md":                                          "skill",
		"scripts/update-all.sh":                                           "script",
		".adb/state":                                                      "PRIVATE",
		".adb_memory.sqlite":                                              "PRIVATE",
		"randomfile.txt":                                                  "not allowlisted",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := WalkUploadSet(root)
	if err != nil {
		t.Fatalf("WalkUploadSet: %v", err)
	}

	want := []string{
		"CLAUDE.md",
		"Taskfile.yaml",
		"raw/articles/x.md",
		"scripts/update-all.sh",
		"skills/cohere/SKILL.md",
		"tickets/github.com/awslabs/mcp/TASK-1-x/context.md",
		"wiki/index.md",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WalkUploadSet mismatch\n got=%v\nwant=%v", got, want)
	}
}

// TestWalkUploadSet_SkipsDeniedDirsEarly confirms the walker returns
// filepath.SkipDir when it hits a denied directory, so we never even stat
// files inside communications/, sessions/, work/, repos/, .omnictx/. This
// matters for security AND performance — repos/ can hold millions of files.
func TestWalkUploadSet_SkipsDeniedDirsEarly(t *testing.T) {
	root := t.TempDir()
	// Put an unreadable file inside a denied dir. If the walker descends,
	// os.Lstat on the child will fail. If it correctly SkipDirs, we never
	// see it.
	deniedFile := filepath.Join(root, "repos", "unreadable.md")
	if err := os.MkdirAll(filepath.Dir(deniedFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deniedFile, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	// Make the parent unreadable too — a walker that descends will error.
	if err := os.Chmod(filepath.Dir(deniedFile), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(deniedFile), 0o755) })

	// One legit file so we get a non-empty result.
	okFile := filepath.Join(root, "raw", "x.md")
	_ = os.MkdirAll(filepath.Dir(okFile), 0o755)
	_ = os.WriteFile(okFile, []byte("ok"), 0o644)

	got, err := WalkUploadSet(root)
	if err != nil {
		t.Fatalf("WalkUploadSet must NOT error on unreadable denied dir: %v", err)
	}
	want := []string{"raw/x.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WalkUploadSet mismatch\n got=%v\nwant=%v", got, want)
	}
}

// TestWalkUploadSet_SkipsSymlinks pins the symlink defence: a symlink
// living inside an include root (raw/, scripts/, skills/, tickets/,
// wiki/) MUST NOT enter the upload set — even though it's superficially
// under a legitimate path. The concern is not just secret-bearing
// targets (gitleaks would catch those) but *any* private non-secret
// target the symlink points at (e.g. ../.env, but also arbitrary
// out-of-tree files).
//
// Also verifies the same rule for a symlink INSIDE a nested include
// dir, where a symlink -> outside-of-workspace could otherwise sneak
// through. The regular file is included as a positive control.
func TestWalkUploadSet_SkipsSymlinks(t *testing.T) {
	root := t.TempDir()

	// Regular file: must be in the set.
	okFile := filepath.Join(root, "raw", "ok.md")
	if err := os.MkdirAll(filepath.Dir(okFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(okFile, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The out-of-tree target — placed OUTSIDE root so the walker never
	// stat's its parent. This is the file we do NOT want to leak.
	outside := filepath.Join(t.TempDir(), "target-outside-workspace.md")
	if err := os.WriteFile(outside, []byte("PRIVATE"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlink inside raw/ pointing at the outside file. If the walker
	// follows the link (bug), the target's contents (or its path) end
	// up in the upload set.
	linkA := filepath.Join(root, "raw", "evil-outside")
	if err := os.Symlink(outside, linkA); err != nil {
		t.Skipf("symlink not supported on this filesystem: %v", err)
	}

	// Symlink pointing at a *denied* dir (.env at root). This is the
	// specific attack vector called out in the security review.
	envFile := filepath.Join(root, ".env")
	if err := os.WriteFile(envFile, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkB := filepath.Join(root, "raw", "evil-env")
	if err := os.Symlink("../.env", linkB); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// Symlink inside a nested include dir, pointing back at the mirror
	// — canary that the guard is not just a top-level check.
	linkC := filepath.Join(root, "wiki", "sneaky")
	if err := os.MkdirAll(filepath.Dir(linkC), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../repos/whatever", linkC); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got, err := WalkUploadSet(root)
	if err != nil {
		t.Fatalf("WalkUploadSet: %v", err)
	}
	want := []string{"raw/ok.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WalkUploadSet must skip symlinks; got=%v want=%v", got, want)
	}
}
