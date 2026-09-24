package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TASK-00039 removed the `adb serena` command — the #203 effectiveness telemetry,
// which had no store, one emitter, one consumer and no non-human reader.
//
// It did NOT remove Serena PROVISIONING (#201/#202), and that distinction is the
// whole point of this test. The two features shared a name and nothing else: the
// telemetry was a CLI surface over the event log, while provisioning sits on the
// worktree-bootstrap seam inside TaskManager.Create/Resume and has never had a
// command. A future cleanup grepping for "serena" would find the provisioner and
// could reasonably conclude it is leftover telemetry — so the guard belongs at
// the binary level, where "the shipped adb still writes this file" is the claim.
//
// It is deliberately an e2e test rather than a unit one: internal/core already
// covers the seam with a mock provisioner, which would keep passing if the
// wiring line in internal/app.go were deleted.
func TestE2E_SerenaProvisioningSurvivesTheCommandRemoval(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	mustRunADB(t, workspace, "init", "workspace", workspace)

	// A real upstream to clone, so the task gets a genuine git worktree.
	upstream := t.TempDir()
	runGit(t, upstream, "init", "-q", "-b", "main", ".")
	runGit(t, upstream, "config", "user.email", "e2e@example.invalid")
	runGit(t, upstream, "config", "user.name", "e2e")
	if err := os.WriteFile(filepath.Join(upstream, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed upstream: %v", err)
	}
	runGit(t, upstream, "add", "-A")
	runGit(t, upstream, "commit", "-qm", "init")

	clone := filepath.Join(workspace, "repos", "github.com", "acme", "widget")
	if err := os.MkdirAll(filepath.Dir(clone), 0o755); err != nil {
		t.Fatalf("mkdir repos: %v", err)
	}
	runGit(t, workspace, "clone", "-q", upstream, clone)

	mustRunADB(
		t, workspace,
		"task", "create", "provisioning survives", "--type", "chore",
		"--repo", "github.com/acme/widget",
	)

	worktree := filepath.Join(
		workspace, "work", "github.com", "acme", "widget",
		"TASK-00001-provisioning-survives",
	)
	config := filepath.Join(worktree, ".serena", "project.yml")
	body, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("Serena provisioning did not write %s: %v\n"+
			"If `adb serena` was removed and this file stopped appearing, the "+
			"provisioner wiring in internal/app.go was deleted with the telemetry.",
			config, err)
	}

	// The detector should have found Go from main.go. Asserting the language
	// rather than merely the file's existence is what distinguishes "provisioning
	// ran" from "something wrote an empty stub".
	if !strings.Contains(string(body), "go") {
		t.Errorf(".serena/project.yml does not mention the detected language:\n%s", body)
	}

	// And the command really is gone.
	if res := runADB(t, workspace, "serena", "report"); res.err == nil {
		t.Error("`adb serena report` still runs; the telemetry command was removed")
	}
}

// runGit runs a git command in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// TASK-00039 Q5: the worktree's task context is agent-agnostic, and adb's own
// generated files no longer make a fresh worktree dirty.
//
// The dirty half is the part that needs a real repo and a real binary. Before this,
// a brand-new worktree in any repo that did not happen to gitignore
// `.claude/rules/task-context.md` was immediately dirty, `adb task list --git`
// reported `dirty` for every ticket, and adb REFUSED to tear down its own worktree
// via the #207 guard. adb's own repo hid it, because its .gitignore lists the file.
func TestE2E_WorktreeContextIsAgentAgnosticAndDoesNotDirtyTheRepo(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	mustRunADB(t, workspace, "init", "workspace", workspace)

	// An upstream with NO .gitignore — the condition that exposed the bug.
	upstream := t.TempDir()
	runGit(t, upstream, "init", "-q", "-b", "main", ".")
	runGit(t, upstream, "config", "user.email", "e2e@example.invalid")
	runGit(t, upstream, "config", "user.name", "e2e")
	if err := os.WriteFile(filepath.Join(upstream, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed upstream: %v", err)
	}
	runGit(t, upstream, "add", "-A")
	runGit(t, upstream, "commit", "-qm", "init")

	clone := filepath.Join(workspace, "repos", "github.com", "acme", "widget")
	if err := os.MkdirAll(filepath.Dir(clone), 0o755); err != nil {
		t.Fatalf("mkdir repos: %v", err)
	}
	runGit(t, workspace, "clone", "-q", upstream, clone)

	mustRunADB(
		t, workspace,
		"task", "create", "agnostic context", "--type", "chore",
		"--repo", "github.com/acme/widget",
	)
	worktree := filepath.Join(
		workspace, "work", "github.com", "acme", "widget", "TASK-00001-agnostic-context",
	)

	// The canonical context is agent-agnostic and names the task.
	canonical := filepath.Join(worktree, ".adb", "task-context.md")
	body, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("canonical task context missing at %s: %v", canonical, err)
	}
	if !strings.Contains(string(body), "TASK-00001") {
		t.Errorf("canonical context does not name the task:\n%s", body)
	}

	// Each harness pointer resolves to it through an @import.
	for _, pointer := range []string{
		filepath.Join(".claude", "rules", "task-context.md"),
		"AGENTS.md",
	} {
		data, err := os.ReadFile(filepath.Join(worktree, pointer))
		if err != nil {
			t.Errorf("pointer %s missing: %v", pointer, err)
			continue
		}
		if !strings.Contains(string(data), "@.adb/task-context.md") {
			t.Errorf("pointer %s does not @import the canonical file:\n%s", pointer, data)
		}
	}

	// THE REGRESSION: the worktree must be clean despite all of the above.
	status := gitOutput(t, worktree, "status", "--short")
	if strings.TrimSpace(status) != "" {
		t.Errorf("a brand-new worktree is dirty from adb's own generated files:\n%s", status)
	}

	// Which means adb can tear down its own worktree without --force.
	if res := runADB(t, workspace, "task", "worktree", "remove", "TASK-00001"); res.err != nil {
		t.Errorf("adb refused to remove its own clean worktree: %v\n%s", res.err, res.combined())
	}
}

// gitOutput runs a git command in dir and returns its stdout.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v", strings.Join(args, " "), dir, err)
	}
	return string(out)
}
