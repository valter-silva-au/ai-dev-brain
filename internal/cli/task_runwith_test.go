package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// fakeCommander records invocations instead of spawning processes.
type fakeCommander struct {
	lastBin  string
	lastArgs []string
	lastCwd  string
	lastEnv  []string
	err      error
}

func (f *fakeCommander) Run(ctx RunContext, name string, args []string) error {
	f.lastBin = name
	f.lastArgs = append([]string(nil), args...)
	f.lastCwd = ctx.Cwd
	f.lastEnv = append([]string(nil), ctx.Env...)
	return f.err
}

// withFakeCommander swaps the package-level commander for the duration
// of the test, restoring the original on cleanup.
func withFakeCommander(t *testing.T, f *fakeCommander) {
	t.Helper()
	orig := taskRunWithRufloCommander
	taskRunWithRufloCommander = f
	t.Cleanup(func() { taskRunWithRufloCommander = orig })
}

// setupRuntimeWorktree creates a real git worktree so the dispatch
// command's worktree-resolution path has something to find. Returns
// the task id that owns the worktree.
func setupRuntimeWorktree(t *testing.T, tmpDir string) string {
	t.Helper()

	// Init a git repo inside tmpDir — resemble a "user's repo" rather
	// than the workspace itself.
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustRun(t, repoDir, "git", "init")
	mustRun(t, repoDir, "git", "config", "user.email", "t@example.com")
	mustRun(t, repoDir, "git", "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mustRun(t, repoDir, "git", "add", "README.md")
	mustRun(t, repoDir, "git", "commit", "-m", "seed")
	mustRun(t, repoDir, "git", "branch", "-M", "main")

	// Create a worktree for a synthetic task id. Use the adb worktree
	// manager directly so GetWorktreeForTask can find it later.
	taskID := "TASK-RUN-001"
	if _, err := App.GitWorktreeManager.CreateWorktree(taskID, repoDir, "main"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	return taskID
}

// mustRun is a tiny helper — test fails on any command error.
func mustRun(t *testing.T, cwd, bin string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", bin, args, err, string(out))
	}
}

func TestRunTaskWithRuflo_Dispatches(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	taskID := setupRuntimeWorktree(t, tmp)

	fc := &fakeCommander{}
	withFakeCommander(t, fc)

	cmd := newTaskRunWithRufloCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader(""))

	if err := runTaskWithRuflo(cmd, taskID, "", []string{"swarm", "init", "--topology", "mesh"}); err != nil {
		t.Fatalf("runTaskWithRuflo: %v", err)
	}

	// Default binary is "claude-flow".
	if fc.lastBin != "claude-flow" {
		t.Errorf("lastBin = %q, want %q", fc.lastBin, "claude-flow")
	}
	wantArgs := []string{"swarm", "init", "--topology", "mesh"}
	if len(fc.lastArgs) != len(wantArgs) {
		t.Fatalf("lastArgs len = %d, want %d: %v", len(fc.lastArgs), len(wantArgs), fc.lastArgs)
	}
	for i, w := range wantArgs {
		if fc.lastArgs[i] != w {
			t.Errorf("lastArgs[%d] = %q, want %q", i, fc.lastArgs[i], w)
		}
	}
	// cwd must be the worktree path.
	if !strings.Contains(fc.lastCwd, filepath.Join("work", taskID)) {
		t.Errorf("lastCwd = %q, expected to contain work/%s", fc.lastCwd, taskID)
	}
	// Env must include the two ADB_* injections.
	envSet := map[string]bool{}
	for _, kv := range fc.lastEnv {
		envSet[kv] = true
	}
	foundTaskID := false
	foundWorktree := false
	for _, kv := range fc.lastEnv {
		if kv == "ADB_TASK_ID="+taskID {
			foundTaskID = true
		}
		if strings.HasPrefix(kv, "ADB_TASK_WORKTREE=") {
			foundWorktree = true
		}
	}
	if !foundTaskID {
		t.Errorf("ADB_TASK_ID=%s not in env", taskID)
	}
	if !foundWorktree {
		t.Errorf("ADB_TASK_WORKTREE=<path> not in env (env had %d entries)", len(envSet))
	}
}

func TestRunTaskWithRuflo_NoWorktreeErrors(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	fc := &fakeCommander{}
	withFakeCommander(t, fc)

	cmd := newTaskRunWithRufloCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err = runTaskWithRuflo(cmd, "TASK-DOES-NOT-EXIST", "", nil)
	if err == nil {
		t.Fatal("expected error for missing worktree, got nil")
	}
	if !strings.Contains(err.Error(), "no worktree") {
		t.Errorf("expected 'no worktree' in error, got %v", err)
	}
	if fc.lastBin != "" {
		t.Errorf("commander should not have been called; got bin=%q", fc.lastBin)
	}
}

func TestRunTaskWithRuflo_BinaryOverride(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	taskID := setupRuntimeWorktree(t, tmp)

	fc := &fakeCommander{}
	withFakeCommander(t, fc)

	cmd := newTaskRunWithRufloCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	// Via explicit --ruflo-bin flag.
	if err := runTaskWithRuflo(cmd, taskID, "my-custom-ruflo", nil); err != nil {
		t.Fatalf("runTaskWithRuflo: %v", err)
	}
	if fc.lastBin != "my-custom-ruflo" {
		t.Errorf("lastBin = %q, want my-custom-ruflo", fc.lastBin)
	}

	// Via env var (no flag).
	t.Setenv("ADB_RUFLO_BIN", "env-configured-ruflo")
	fc.lastBin = ""
	if err := runTaskWithRuflo(cmd, taskID, "", nil); err != nil {
		t.Fatalf("runTaskWithRuflo: %v", err)
	}
	if fc.lastBin != "env-configured-ruflo" {
		t.Errorf("lastBin = %q, want env-configured-ruflo", fc.lastBin)
	}
}

func TestRunTaskWithRuflo_PropagatesCommanderError(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	taskID := setupRuntimeWorktree(t, tmp)

	wantErr := errors.New("ruflo exited with status 7")
	fc := &fakeCommander{err: wantErr}
	withFakeCommander(t, fc)

	cmd := newTaskRunWithRufloCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	got := runTaskWithRuflo(cmd, taskID, "", nil)
	if !errors.Is(got, wantErr) {
		t.Errorf("expected wrapped commander error, got %v", got)
	}
}

// Sanity: the subcommand registers under `adb task`.
func TestRunWithRuflo_CommandRegistered(t *testing.T) {
	rootCmd := NewRootCmd()
	taskCmd := findCobraSub(rootCmd, "task")
	if taskCmd == nil {
		t.Fatal("task command not registered")
	}
	if findCobraSub(taskCmd, "run-with-ruflo") == nil {
		t.Fatal("task run-with-ruflo not registered")
	}
}

// Compile-time check: execCommander satisfies Commander.
var _ Commander = execCommander{}
