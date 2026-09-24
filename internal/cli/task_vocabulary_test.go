package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// newTaskWorkspace gives each test an isolated App over a temp workspace. Same
// shape as newStartWorkspace; kept separate so the two files' fixtures can
// diverge without one breaking the other.
func newTaskWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("write backlog: %v", err)
	}
	app, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	oldApp := App
	App = app
	t.Cleanup(func() { App = oldApp })
	return dir
}

// runCmd drives one constructed command with captured streams.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut strings.Builder
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

// runTree drives a path through the real root, which is the only way to prove a
// hidden alias is actually reachable.
func runTree(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCmd(t, NewRootCmd(), args...)
}

func seedTask(t *testing.T, title string, taskType models.TaskType) *models.Task {
	t.Helper()
	task, err := App.TaskManager.Create(core.CreateTaskOpts{Title: title, TaskType: taskType})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

// --- 1. the tree shape -------------------------------------------------------

// The new vocabulary is the visible surface; the old spellings survive only as
// hidden aliases. Both halves are asserted, because a rename that leaves the old
// name visible has not renamed anything.
func TestTaskVocabulary_VisibleAndHiddenSubcommands(t *testing.T) {
	visible := map[string]bool{
		"create": true, "list": true, "show": true, "start": true, "resume": true,
		"update": true, "close": true, "archive": true, "remove": true,
		"validate": true, "timeline": true, "worktree": true,
	}
	hidden := map[string]bool{
		"status": true, "delete": true, "priority": true, "unarchive": true,
		"cleanup": true, "migrate-blocked-by": true,
	}

	found := map[string]*cobra.Command{}
	for _, sub := range NewTaskCmd().Commands() {
		found[sub.Name()] = sub
	}

	for name := range visible {
		sub, ok := found[name]
		if !ok {
			t.Errorf("adb task %s is missing", name)
			continue
		}
		if sub.Hidden {
			t.Errorf("adb task %s is hidden, want visible", name)
		}
	}
	for name := range hidden {
		sub, ok := found[name]
		if !ok {
			t.Errorf("adb task %s must survive as a hidden alias, but is gone", name)
			continue
		}
		if !sub.Hidden {
			t.Errorf("adb task %s is visible, want hidden (it was renamed)", name)
		}
	}
	for name := range found {
		if !visible[name] && !hidden[name] && name != "help" && name != "completion" {
			t.Errorf("unexpected `adb task %s` — add it to the visible or hidden set deliberately", name)
		}
	}
}

func TestTaskVocabulary_WorktreeNamespace(t *testing.T) {
	want := []string{"list", "switch", "remove", "prune", "reconcile"}
	found := map[string]bool{}
	for _, sub := range newTaskWorktreeCmd().Commands() {
		found[sub.Name()] = true
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("adb task worktree %s is missing", name)
		}
	}
}

// Top-level `status` and `work` leave the visible surface entirely. They stay
// reachable, so no script breaks, but they no longer advertise themselves.
func TestTaskVocabulary_TopLevelStatusAndWorkAreHiddenAliases(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"status", "work"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Fatalf("adb %s is not reachable: %v", name, err)
		}
		if !cmd.Hidden {
			t.Errorf("adb %s is visible, want hidden", name)
		}
	}
}

func TestTaskVocabulary_RetiredWorkChildrenReachable(t *testing.T) {
	root := NewRootCmd()
	for _, child := range []string{"list", "switch", "prune", "reconcile"} {
		cmd, _, err := root.Find([]string{"work", child})
		if err != nil || cmd.Name() != child {
			t.Errorf("adb work %s is not reachable: %v", child, err)
		}
	}
}

// --- 2. the JSON contracts (spec §3) ----------------------------------------

// The whole point of the shape-stable decision: a flag must never change the
// top-level JSON shape. `--git` adds keys to each element, it does not switch
// the document from an array to an object.
func TestTaskList_JSONIsAlwaysAnArray(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "first", models.TaskTypeFeat)
	seedTask(t, "second", models.TaskTypeFix)

	for _, args := range [][]string{{"--json"}, {"--json", "--git"}} {
		out, _, err := runCmd(t, newTaskListCmd(), args...)
		if err != nil {
			t.Fatalf("task list %v: %v", args, err)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("task list %v did not emit a JSON array: %v\n%s", args, err, out)
		}
		if len(rows) != 2 {
			t.Errorf("task list %v returned %d rows, want 2", args, len(rows))
		}
	}
}

// --git must not silently filter. The old `--git` path went through
// buildStatusRows, which drops archived tasks and tasks with no worktree — so a
// flag that reads as "show me more" removed rows.
func TestTaskList_GitDoesNotChangeWhichTasksAppear(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "no worktree here", models.TaskTypeFeat)

	plain, _, err := runCmd(t, newTaskListCmd(), "--json")
	if err != nil {
		t.Fatalf("task list --json: %v", err)
	}
	withGit, _, err := runCmd(t, newTaskListCmd(), "--json", "--git")
	if err != nil {
		t.Fatalf("task list --json --git: %v", err)
	}

	var a, b []map[string]any
	if err := json.Unmarshal([]byte(plain), &a); err != nil {
		t.Fatalf("plain: %v", err)
	}
	if err := json.Unmarshal([]byte(withGit), &b); err != nil {
		t.Fatalf("git: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("--git changed the row count: %d without, %d with", len(a), len(b))
	}
	if len(b) == 0 {
		t.Fatal("no rows to inspect")
	}
	// The git keys are present on a repo-less task too, at their zero values —
	// "this task has no worktree" is an answer, not an absence.
	for _, key := range []string{"worktree_exists", "dirty", "ahead", "behind", "worktree_missing"} {
		if _, ok := b[0][key]; !ok {
			t.Errorf("--git row is missing key %q", key)
		}
	}
	if _, ok := a[0]["dirty"]; ok {
		t.Error("plain --json row carries git keys; they belong to --git only")
	}
}

func TestTaskList_EmptyJSONIsAnEmptyArray(t *testing.T) {
	newTaskWorkspace(t)
	out, _, err := runCmd(t, newTaskListCmd(), "--json")
	if err != nil {
		t.Fatalf("task list --json: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("empty workspace task list --json = %q, want []", strings.TrimSpace(out))
	}
}

// `task show` is a per-task read, so its JSON is a single object — not a
// one-element array a caller has to unwrap.
func TestTaskShow_JSONIsASingleObject(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "show me", models.TaskTypeDocs)

	out, _, err := runCmd(t, newTaskShowCmd(), task.ID, "--json")
	if err != nil {
		t.Fatalf("task show --json: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("task show --json is not a JSON object: %v\n%s", err, out)
	}
	if row["id"] != task.ID {
		t.Errorf("id = %v, want %s", row["id"], task.ID)
	}
	if row["type"] != string(models.TaskTypeDocs) {
		t.Errorf("type = %v, want docs", row["type"])
	}
}

func TestTaskShow_UnknownTaskErrors(t *testing.T) {
	newTaskWorkspace(t)
	if _, _, err := runCmd(t, newTaskShowCmd(), "TASK-99999", "--json"); err == nil {
		t.Fatal("task show on an unknown id returned nil, want an error")
	}
}

// `task worktree list` is the one listing that owns data which is not a task —
// orphaned worktrees have no row — so it is the one with an envelope.
func TestTaskWorktreeList_JSONCarriesWorktreesAndOrphans(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "wt", models.TaskTypeFeat)

	out, _, err := runCmd(t, newTaskWorktreeListCmd(), "--json")
	if err != nil {
		t.Fatalf("task worktree list --json: %v", err)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, out)
	}
	for _, key := range []string{"worktrees", "orphaned"} {
		if _, ok := report[key]; !ok {
			t.Errorf("missing key %q (got %v)", key, report)
		}
	}
	// Both must be arrays, never null, so `jq '.orphaned | length'` works on a
	// clean workspace.
	for key, raw := range report {
		if strings.TrimSpace(string(raw)) == "null" {
			t.Errorf("%q is null, want an empty array", key)
		}
	}
}

// --- 3. close and timeline --------------------------------------------------

func TestTaskClose_MarksDone(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "close me", models.TaskTypeFeat)

	if _, _, err := runCmd(t, newTaskCloseCmd(), task.ID); err != nil {
		t.Fatalf("task close: %v", err)
	}
	stored, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != models.TaskStatusDone {
		t.Errorf("status = %s, want done", stored.Status)
	}
}

func TestTaskClose_UnknownTaskErrors(t *testing.T) {
	newTaskWorkspace(t)
	if _, _, err := runCmd(t, newTaskCloseCmd(), "TASK-99999"); err == nil {
		t.Fatal("task close on an unknown id returned nil, want an error")
	}
}

// `timeline` reuses the event log rather than adding a store, so its JSON is
// byte-compatible with `adb events query --json`: an array of events.
func TestTaskTimeline_JSONIsAnEventArray(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "timeline me", models.TaskTypeFeat)
	if err := App.TaskManager.Close(task.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, _, err := runCmd(t, newTaskTimelineCmd(), task.ID, "--json")
	if err != nil {
		t.Fatalf("task timeline --json: %v", err)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(out), &events); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, out)
	}
	if len(events) == 0 {
		t.Fatal("timeline is empty; task.created and task.status_changed should both be there")
	}
	for _, e := range events {
		for _, key := range []string{"timestamp", "type", "data"} {
			if _, ok := e[key]; !ok {
				t.Fatalf("event %v is missing %q — the shape must match `adb events query --json`", e, key)
			}
		}
		data, _ := e["data"].(map[string]any)
		if data["task_id"] != task.ID {
			t.Errorf("timeline leaked an event for %v, want only %s", data["task_id"], task.ID)
		}
	}
}

// A timeline for a task with no events is an empty array, not null and not an
// error — "nothing has happened yet" is a valid answer.
func TestTaskTimeline_EmptyIsAnEmptyArray(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "quiet", models.TaskTypeFeat)

	out, _, err := runCmd(t, newTaskTimelineCmd(), "TASK-00002", "--json")
	if err != nil {
		t.Fatalf("task timeline --json: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("out = %q, want []", strings.TrimSpace(out))
	}
}

// --- 4. the update/archive semantics fix (spec §4) --------------------------

// `task update --status backlog` on an ARCHIVED task must route through
// TaskManager.Unarchive, which physically moves the ticket directory back out of
// _archived/. Writing the field alone left the directory in _archived/ with a
// backlog status — an incoherent workspace the merge would have made routine.
func TestTaskUpdate_StatusBacklogUnarchives(t *testing.T) {
	dir := newTaskWorkspace(t)
	task := seedTask(t, "archived then restored", models.TaskTypeFeat)

	if err := App.TaskManager.Archive(task.ID, core.ArchiveOptions{Force: true}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	archived, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload archived: %v", err)
	}
	if !strings.Contains(archived.TicketPath, "_archived") {
		t.Fatalf("archived ticket path = %q, want it under _archived/", archived.TicketPath)
	}

	if _, _, err := runCmd(t, newTaskUpdateCmd(), task.ID, "--status", "backlog"); err != nil {
		t.Fatalf("task update --status backlog: %v", err)
	}

	restored, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload restored: %v", err)
	}
	if restored.Status != models.TaskStatusBacklog {
		t.Errorf("status = %s, want backlog", restored.Status)
	}
	if strings.Contains(restored.TicketPath, "_archived") {
		t.Errorf("ticket path = %q, still under _archived/ — the directory was not moved back", restored.TicketPath)
	}
	if _, err := os.Stat(restored.TicketPath); err != nil {
		t.Errorf("restored ticket dir %s does not exist: %v", restored.TicketPath, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tickets", "_archived", task.ID)); err == nil {
		t.Error("the archived copy is still on disk")
	}
}

// `--status archived` used to write the field and nothing else: a live worktree,
// a live ticket dir, and an `archived` row. Reject it and name the command that
// owns archiving — it is the one with --force/--keep-worktree/--prune-branch.
func TestTaskUpdate_StatusArchivedIsRejected(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "do not archive me this way", models.TaskTypeFeat)

	_, _, err := runCmd(t, newTaskUpdateCmd(), task.ID, "--status", "archived")
	if err == nil {
		t.Fatal("task update --status archived was accepted, want a rejection")
	}
	if !strings.Contains(err.Error(), "adb task archive") {
		t.Errorf("error %q does not name `adb task archive`", err)
	}

	stored, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status == models.TaskStatusArchived {
		t.Error("the status was written despite the rejection")
	}
}

// A non-archived task keeps the plain field write — the unarchive routing must
// not turn every `--status backlog` into a directory move.
func TestTaskUpdate_StatusBacklogOnLiveTaskIsAPlainWrite(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "live", models.TaskTypeFeat)
	if err := App.TaskManager.UpdateStatus(task.ID, models.TaskStatusReview); err != nil {
		t.Fatalf("seed review: %v", err)
	}

	if _, _, err := runCmd(t, newTaskUpdateCmd(), task.ID, "--status", "backlog"); err != nil {
		t.Fatalf("task update --status backlog: %v", err)
	}
	stored, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != models.TaskStatusBacklog {
		t.Errorf("status = %s, want backlog", stored.Status)
	}
}

// --- 5. the worktree-prune safety rule (spec §5) ----------------------------

// One rule across both nouns: a worktree mutation that NAMES its target acts;
// one that SWEEPS a set previews unless --apply. `prune` and `reconcile` sweep.
func TestTaskWorktree_SweepingVerbsPreviewByDefault(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() *cobra.Command
	}{
		{"prune", newTaskWorktreePruneCmd},
		{"reconcile", newTaskWorktreeReconcileCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.build()
			apply := cmd.Flags().Lookup("apply")
			if apply == nil {
				t.Fatalf("task worktree %s has no --apply flag", tc.name)
			}
			if apply.DefValue != "false" {
				t.Errorf("--apply default = %q, want false (preview is the default)", apply.DefValue)
			}
			// --dry-run survives as an accepted no-op so `adb work prune
			// --dry-run` keeps parsing, but it must not advertise itself.
			dryRun := cmd.Flags().Lookup("dry-run")
			if dryRun == nil {
				t.Fatalf("task worktree %s dropped --dry-run; the retired alias needs it to keep parsing", tc.name)
			}
			if !dryRun.Hidden {
				t.Errorf("--dry-run is visible on task worktree %s, want hidden (preview is the default)", tc.name)
			}
		})
	}
}

// `task worktree remove <id>` names its target, so it acts immediately — and
// keeps the #207 dirty guard, with --force to override.
func TestTaskWorktreeRemove_NamesItsTargetAndActs(t *testing.T) {
	cmd := newTaskWorktreeRemoveCmd()
	if cmd.Flags().Lookup("apply") != nil {
		t.Error("task worktree remove has --apply; a verb that names its target should act")
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Error("task worktree remove lost --force (the #207 dirty-guard override)")
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("task worktree remove accepted zero args; it must name a task")
	}
}

// Found by driving the installed binary, not by any unit test: the CLI wrapper
// and TaskManager.Cleanup both prefixed "failed to remove worktree", so a
// refused removal read
//
//	failed to remove worktree: failed to remove worktree: refusing to remove …
//
// A stutter like that reads as a bug in adb rather than as the dirty guard doing
// its job, which is the one message here that has to be trusted.
// The task must EXIST and its removal must FAIL, or the assertion is vacuous: an
// unknown id fails at the backlog lookup and never reaches the code that had the
// stutter. Pointing a real task at a directory that is not a git worktree is the
// in-process way to make Cleanup's own error surface.
func TestTaskWorktreeRemove_ErrorIsNotDoublyPrefixed(t *testing.T) {
	dir := newTaskWorkspace(t)
	task := seedTask(t, "points at a non-worktree", models.TaskTypeChore)

	bogus := filepath.Join(dir, "not-a-worktree")
	if err := os.MkdirAll(bogus, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	task.WorktreePath = bogus
	if err := App.BacklogManager.UpdateTask(*task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	_, _, err := runCmd(t, newTaskWorktreeRemoveCmd(), task.ID)
	if err == nil {
		t.Fatal("removing a non-worktree directory succeeded, want an error")
	}
	if got := strings.Count(err.Error(), "failed to remove worktree"); got != 1 {
		t.Errorf("prefix appears %d times, want exactly 1: %v", got, err)
	}
}

// `task worktree remove` clears the recorded worktree_path, so reconcile — which
// keys off that path — cannot rebuild it. The help text said it could, which is
// worse than saying nothing: it sends someone to a command that will silently
// report "0 change(s) planned".
func TestTaskWorktreeRemove_HelpDoesNotPromiseReconcileRebuilds(t *testing.T) {
	help := newTaskWorktreeRemoveCmd().Long
	if !strings.Contains(help, "reconcile") {
		t.Fatal("help says nothing about reconcile; the interaction is surprising enough to need a note")
	}
	if !strings.Contains(help, "will not rebuild it") {
		t.Errorf("help does not state that reconcile will NOT rebuild a deliberately-removed worktree:\n%s", help)
	}
}

// Cleanup no-ops on a task with no worktree, so the old `✓ … worktree removed`
// was reporting a removal that never happened — confirmation the caller did not
// earn, and indistinguishable from the real thing.
func TestTaskWorktreeRemove_SaysWhenThereWasNothingToRemove(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "repo-less, so no worktree", models.TaskTypeChore)

	out, _, err := runCmd(t, newTaskWorktreeRemoveCmd(), task.ID)
	if err != nil {
		t.Fatalf("task worktree remove: %v", err)
	}
	if !strings.Contains(out, "nothing to remove") {
		t.Errorf("out = %q, want it to say there was no worktree", out)
	}
	if strings.Contains(out, "worktree removed") {
		t.Errorf("out = %q claims a removal that did not happen", out)
	}
}

// An empty worktree listing must say so rather than printing a bare header,
// which reads as a rendering failure.
func TestTaskWorktreeList_EmptyHumanOutputSaysSo(t *testing.T) {
	newTaskWorkspace(t)

	out, _, err := runCmd(t, newTaskWorktreeListCmd())
	if err != nil {
		t.Fatalf("task worktree list: %v", err)
	}
	if !strings.Contains(out, "No task worktrees.") {
		t.Errorf("out = %q, want a no-worktrees message", out)
	}
	if strings.Contains(out, "TASK\t") || strings.Contains(out, "BRANCH") {
		t.Errorf("out = %q, want no bare table header", out)
	}
}

// --- 6. the aliases (spec §2) ----------------------------------------------

// A rename's alias is built from the TARGET's constructor, so the two spellings
// cannot drift in flags or defaults. Assert the full effective flag set.
func TestTaskAliases_PreserveTheirFlagSets(t *testing.T) {
	root := NewRootCmd()
	for _, tc := range []struct {
		old    []string
		target func() *cobra.Command
	}{
		{[]string{"task", "status"}, newTaskListCmd},
		{[]string{"task", "delete"}, newTaskRemoveCmd},
		{[]string{"task", "cleanup"}, newTaskWorktreeRemoveCmd},
		{[]string{"work", "list"}, newTaskWorktreeListCmd},
		{[]string{"work", "switch"}, newTaskWorktreeSwitchCmd},
		{[]string{"work", "prune"}, newTaskWorktreePruneCmd},
		{[]string{"work", "reconcile"}, newTaskWorktreeReconcileCmd},
	} {
		name := strings.Join(tc.old, " ")
		t.Run(name, func(t *testing.T) {
			alias, _, err := root.Find(tc.old)
			if err != nil {
				t.Fatalf("adb %s is not reachable: %v", name, err)
			}
			want := map[string]string{}
			tc.target().Flags().VisitAll(func(f *pflag.Flag) { want[f.Name] = f.DefValue })
			got := map[string]string{}
			alias.Flags().VisitAll(func(f *pflag.Flag) { got[f.Name] = f.DefValue })
			for flag, def := range want {
				gotDef, ok := got[flag]
				if !ok {
					t.Errorf("alias `adb %s` is missing --%s", name, flag)
					continue
				}
				if gotDef != def {
					t.Errorf("alias `adb %s` --%s default = %q, want %q", name, flag, gotDef, def)
				}
			}
		})
	}
}

// The deprecation notice goes to stderr, so stdout stays a clean pipeline. A
// sentence prepended to --json output is a silently broken script, not a
// visible failure — which is why cobra's Deprecated field is not used.
func TestTaskAliases_NoticeGoesToStderrAndStdoutStaysParseable(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "piped", models.TaskTypeFeat)

	stdout, stderr, err := runTree(t, "task", "status", "--json")
	if err != nil {
		t.Fatalf("adb task status --json: %v", err)
	}
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("stderr = %q, want a deprecation notice", stderr)
	}
	if !strings.Contains(stderr, "adb task list") {
		t.Errorf("stderr = %q, does not name the replacement", stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("stdout is not clean JSON: %v\n%s", err, stdout)
	}
	if len(rows) != 1 {
		t.Errorf("rows = %d, want 1", len(rows))
	}
}

// `adb status` was a top-level command whose whole job was the git-joined view.
// Its alias must therefore land on `task list --git`, not on a plain list.
func TestTopLevelStatusAliasImpliesGit(t *testing.T) {
	newTaskWorkspace(t)
	seedTask(t, "joined", models.TaskTypeFeat)

	stdout, stderr, err := runTree(t, "status", "--json")
	if err != nil {
		t.Fatalf("adb status --json: %v", err)
	}
	if !strings.Contains(stderr, "adb task list --git") {
		t.Errorf("stderr = %q, does not name `adb task list --git`", stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout)
	}
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	if _, ok := rows[0]["dirty"]; !ok {
		t.Error("`adb status --json` row has no git keys; the alias did not imply --git")
	}
}

// The merges keep their own retired implementation, because their shapes differ
// from the flag they merged into — `priority` takes many ids and a required
// flag, `update` takes exactly one id and four optional ones. They must still
// work and still delegate to the same core method.
func TestTaskMergeAliases_StillWork(t *testing.T) {
	newTaskWorkspace(t)
	first := seedTask(t, "one", models.TaskTypeFeat)
	second := seedTask(t, "two", models.TaskTypeFix)

	if _, stderr, err := runTree(t, "task", "priority", first.ID, second.ID, "--priority", "P0"); err != nil {
		t.Fatalf("adb task priority: %v (stderr %q)", err, stderr)
	}
	for _, id := range []string{first.ID, second.ID} {
		stored, err := App.BacklogManager.GetTask(id)
		if err != nil {
			t.Fatalf("reload %s: %v", id, err)
		}
		if stored.Priority != models.PriorityP0 {
			t.Errorf("%s priority = %s, want P0", id, stored.Priority)
		}
	}

	if err := App.TaskManager.Archive(first.ID, core.ArchiveOptions{Force: true}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, stderr, err := runTree(t, "task", "unarchive", first.ID); err != nil {
		t.Fatalf("adb task unarchive: %v (stderr %q)", err, stderr)
	}
	restored, err := App.BacklogManager.GetTask(first.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if restored.Status != models.TaskStatusBacklog {
		t.Errorf("status = %s, want backlog", restored.Status)
	}
}

// --- 7. help-rendering hygiene ----------------------------------------------

// Cobra's UnquoteUsage treats a BACKQUOTED word in a flag's usage string as the
// flag's VALUE PLACEHOLDER. So a bool flag whose usage mentions `adb events query
// --json` renders as
//
//	--json adb events query --json   Output as a JSON array (…)
//
// which tells the reader the flag takes an argument. Batch 2b hit this on
// `--all` and swept for it; this test makes the sweep permanent rather than a
// thing someone has to remember.
//
// The rule is narrow on purpose: a backquoted word on a flag that DOES take a
// value is the documented cobra idiom for naming its placeholder, so only
// zero-value flags are checked.
func TestFlagUsageStringsDoNotBackquoteOnValuelessFlags(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			// NoOptDefVal is set for flags usable without a value (bools).
			if f.Value.Type() != "bool" {
				return
			}
			if strings.Contains(f.Usage, "`") {
				t.Errorf(
					"%s --%s usage contains a backquote, which cobra renders as a value placeholder on a bool flag: %q",
					cmd.CommandPath(), f.Name, f.Usage,
				)
			}
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(NewRootCmd())
}

// --- 8. no stale advertising ------------------------------------------------

// Help text is documentation that ships in the binary. A renamed spelling left
// in a Short/Long/Example is a doc that lies from inside the tool.
func TestTaskVocabulary_HelpTextDoesNotAdvertiseRetiredSpellings(t *testing.T) {
	retired := []string{
		"adb task status", "adb task delete", "adb task cleanup",
		"adb task priority", "adb task unarchive",
		"adb work ", "adb status",
	}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		// A deprecated alias's own Short names the replacement, so skip the
		// aliases themselves — they are the one place the old name belongs.
		if !cmd.Hidden {
			text := cmd.Short + "\n" + cmd.Long + "\n" + cmd.Example
			for _, spelling := range retired {
				if strings.Contains(text, spelling) {
					t.Errorf("%s help still advertises %q", cmd.CommandPath(), spelling)
				}
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(NewRootCmd())
}
