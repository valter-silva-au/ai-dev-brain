package core

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// assertOwnerRWFile mirrors the repo's portable perm contract (see
// storage/perm_helpers_test.go): a regular file readable+writable by the owner,
// rather than an exact 0o644 match which Windows never reports.
func assertOwnerRWFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("%q should be a regular file, got %v", path, info.Mode())
	}
	if info.Mode().Perm()&0o600 != 0o600 {
		t.Errorf("%q must be owner read+write, got %o", path, info.Mode().Perm())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(b)
}

// TestInstallHarness_FreshIdempotentAndClobberSafe covers the whole install
// contract over a synthetic harness FS + a temp Claude dir: fresh install lands
// each file under agents/ or skills/, a re-run is "unchanged", a user-edited file
// is "skipped" (never clobbered) without --force and overwritten with it.
func TestInstallHarness_FreshIdempotentAndClobberSafe(t *testing.T) {
	harnessFS := fstest.MapFS{
		"agents/devils-advocate.md":  {Data: []byte("AGENT v1")},
		"skills/stage-gate/SKILL.md": {Data: []byte("SKILL v1")},
	}
	dir := t.TempDir()
	agentDest := filepath.Join(dir, "agents", "devils-advocate.md")
	skillDest := filepath.Join(dir, "skills", "stage-gate", "SKILL.md")

	// Fresh install → both installed, with content + owner-rw perms.
	res, err := InstallHarness(harnessFS, dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("InstallHarness (fresh): %v", err)
	}
	if res.Count(HarnessInstalled) != 2 || len(res.Entries) != 2 {
		t.Fatalf("fresh install = %+v, want 2 installed", res.Entries)
	}
	if got := readFile(t, agentDest); got != "AGENT v1" {
		t.Errorf("agent content = %q", got)
	}
	if got := readFile(t, skillDest); got != "SKILL v1" {
		t.Errorf("skill content = %q", got)
	}
	assertOwnerRWFile(t, agentDest)
	assertOwnerRWFile(t, skillDest)

	// Re-run → both unchanged (idempotent).
	res, err = InstallHarness(harnessFS, dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("InstallHarness (rerun): %v", err)
	}
	if res.Count(HarnessUnchanged) != 2 {
		t.Errorf("rerun = %+v, want 2 unchanged", res.Entries)
	}

	// A user edits the installed agent → a plain re-run must NOT clobber it.
	if err := os.WriteFile(agentDest, []byte("USER EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = InstallHarness(harnessFS, dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("InstallHarness (after edit): %v", err)
	}
	if a := actionFor(res, agentDest); a != HarnessSkipped {
		t.Errorf("edited agent action = %q, want skipped", a)
	}
	if got := readFile(t, agentDest); got != "USER EDIT" {
		t.Errorf("edited agent was clobbered without --force: %q", got)
	}

	// --force overwrites the edited file back to the embedded content.
	res, err = InstallHarness(harnessFS, dir, HarnessInstallOptions{Force: true})
	if err != nil {
		t.Fatalf("InstallHarness (force): %v", err)
	}
	if a := actionFor(res, agentDest); a != HarnessInstalled {
		t.Errorf("forced agent action = %q, want installed", a)
	}
	if got := readFile(t, agentDest); got != "AGENT v1" {
		t.Errorf("agent after --force = %q, want embedded content", got)
	}
}

// actionFor returns the action recorded for the entry whose Dest ends with the
// same base path as want (compared on the full Dest).
func actionFor(res HarnessInstallResult, dest string) HarnessInstallAction {
	for _, e := range res.Entries {
		if e.Dest == dest {
			return e.Action
		}
	}
	return ""
}

// TestInstallHarness_DryRunWritesNothing verifies a dry run plans installs but
// touches no disk.
func TestInstallHarness_DryRunWritesNothing(t *testing.T) {
	harnessFS := fstest.MapFS{
		"agents/devils-advocate.md":  {Data: []byte("A")},
		"skills/stage-gate/SKILL.md": {Data: []byte("S")},
	}
	dir := t.TempDir()

	res, err := InstallHarness(harnessFS, dir, HarnessInstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("InstallHarness (dry-run): %v", err)
	}
	if !res.DryRun || res.Count(HarnessInstalled) != 2 {
		t.Errorf("dry-run result = %+v, want 2 planned installs", res)
	}
	// Nothing was written.
	if _, err := os.Stat(filepath.Join(dir, "agents")); !os.IsNotExist(err) {
		t.Errorf("dry-run created the agents dir (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills")); !os.IsNotExist(err) {
		t.Errorf("dry-run created the skills dir (err=%v)", err)
	}
}

// TestInstallHarness_UnresolvedDir rejects an empty Claude dir.
func TestInstallHarness_UnresolvedDir(t *testing.T) {
	if _, err := InstallHarness(fstest.MapFS{}, "", HarnessInstallOptions{}); err == nil {
		t.Error("InstallHarness with an empty claude dir should error")
	}
}

// TestInstallHarness_RealEmbed installs the real embedded harness and asserts the
// devils-advocate agent and stage-gate skill land at their expected paths.
func TestInstallHarness_RealEmbed(t *testing.T) {
	dir := t.TempDir()
	res, err := InstallHarness(claude.FS, dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("InstallHarness(claude.FS): %v", err)
	}
	if res.Count(HarnessInstalled) == 0 {
		t.Fatal("real embed installed nothing")
	}
	for _, rel := range []string{
		filepath.Join("agents", "devils-advocate.md"),
		filepath.Join("skills", "stage-gate", "SKILL.md"),
	} {
		p := filepath.Join(dir, rel)
		if info, err := os.Stat(p); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty installed %s (err=%v)", rel, err)
		}
		assertOwnerRWFile(t, p)
	}
}
