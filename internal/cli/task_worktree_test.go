package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// mustWorktreeGit runs git in dir, failing the test on a non-zero exit. Identity
// is passed per-invocation rather than written into a config file, so the test
// never depends on (or touches) the developer's git configuration.
func mustWorktreeGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.name=ADB Test",
		"-c", "user.email=adb-test@example.invalid",
		"-c", "commit.gpgsign=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestTaskWorktreePrune_LogsRemovalAndCountsInMetrics is the follow-up to #206
// driven through the CLI over a REAL git worktree and the App's real TaskManager
// and event log — the wiring is the thing under test, so nothing here is faked.
//
// Before the fix, `task worktree prune --apply` called
// App.GitWorktreeManager.RemoveWorktree directly and emitted nothing, so
// `adb metrics` reported worktrees_removed = 0 after a sweep and #206's
// created/removed balance silently broke.
func TestTaskWorktreePrune_LogsRemovalAndCountsInMetrics(t *testing.T) {
	tmp := t.TempDir()
	app := withAppAt(t, tmp)

	// A real clone where findOrphanedWorktrees looks for it: repos/<platform>/<org>/<repo>.
	clone := filepath.Join(tmp, "repos", "github.com", "acme", "thing")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWorktreeGit(t, clone, "init")
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWorktreeGit(t, clone, "add", "--", "README.md")
	mustWorktreeGit(t, clone, "commit", "-m", "seed")

	// A worktree on disk that no ticket owns — the orphan.
	orphan := filepath.Join(tmp, "work", "github.com", "acme", "thing", "TASK-00099-orphan")
	mustWorktreeGit(t, clone, "worktree", "add", "-b", "chore/orphan", orphan)

	// One live ticket in that repo, owning no worktree. It is what makes the repo
	// visible to the orphan scan; the worktree above belongs to nobody.
	task := models.NewTask("TASK-00001", "live ticket", models.TaskTypeFeat)
	task.Repo = "github.com/acme/thing"
	task.Status = models.TaskStatusInProgress
	if err := app.BacklogManager.AddTask(*task); err != nil {
		t.Fatal(err)
	}

	// Preview: the safety rule says a sweep changes nothing without --apply, so
	// it must also log nothing.
	out, err := runADBOut(t, "task", "worktree", "prune")
	if err != nil {
		t.Fatalf("prune (preview): %v\n%s", err, out)
	}
	if !strings.Contains(out, "would prune") {
		t.Errorf("preview did not name the orphan:\n%s", out)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("preview removed the worktree: %v", err)
	}
	if m, err := app.MetricsCalculator.ComputeMetrics(); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	} else if m.WorktreesRemoved != 0 {
		t.Errorf("preview logged a removal: worktrees_removed = %d, want 0", m.WorktreesRemoved)
	}

	out, err = runADBOut(t, "task", "worktree", "prune", "--apply")
	if err != nil {
		t.Fatalf("prune --apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pruned:") {
		t.Fatalf("prune --apply did not report a removal:\n%s", out)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan worktree survived the prune (stat err = %v)", err)
	}

	// The point of the change: the sweep is now counted.
	m, err := app.MetricsCalculator.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if m.WorktreesRemoved != 1 {
		t.Errorf("worktrees_removed = %d after a prune, want 1", m.WorktreesRemoved)
	}

	// …and it is attributed honestly: no task_id, a reason naming why.
	events, err := app.EventLog.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	found := 0
	for _, e := range events {
		if string(e.Type) != "worktree.removed" {
			continue
		}
		found++
		if id, ok := e.Data["task_id"].(string); !ok || id != "" {
			t.Errorf("orphan sweep task_id = %#v, want an empty string", e.Data["task_id"])
		}
		if e.Data["reason"] != "orphaned" {
			t.Errorf("orphan sweep reason = %#v, want \"orphaned\"", e.Data["reason"])
		}
		if p, ok := e.Data["path"].(string); !ok || !strings.HasSuffix(p, "TASK-00099-orphan") {
			t.Errorf("orphan sweep path = %#v, want the orphan worktree path", e.Data["path"])
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly 1 worktree.removed event, got %d", found)
	}

	// An empty task_id must not leak into some task's timeline — `task timeline`
	// filters on data.task_id, and an orphan belongs to no task's history.
	timeline, err := runADBOut(t, "task", "timeline", "TASK-00001")
	if err != nil {
		t.Fatalf("task timeline: %v\n%s", err, timeline)
	}
	if strings.Contains(timeline, "worktree.removed") {
		t.Errorf("orphan sweep leaked into TASK-00001's timeline:\n%s", timeline)
	}
}
