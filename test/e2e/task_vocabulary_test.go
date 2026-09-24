package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// The task vocabulary settled in TASK-00039 (Q7), driven against the real
// binary. In-process cli tests build a root directly; only the shipped binary
// composes the root a user actually gets, and only a separate process resolves
// its own workspace and config tiers. Both of Batch 2b's real defects were found
// this way and by nothing else.

// newTaskWorkspace scaffolds a task workspace and returns its path.
func newVocabWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	mustRunADB(t, workspace, "init", "workspace", workspace)
	return workspace
}

// seedVocabTask creates a repo-less task (so no worktree, no agent launch) and
// returns its id.
func seedVocabTask(t *testing.T, workspace, title string) string {
	t.Helper()
	mustRunADB(t, workspace, "task", "create", title, "--type", "chore", "--no-launch")
	list := mustRunADB(t, workspace, "task", "list", "--json")
	var rows []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(list.stdout), &rows); err != nil {
		t.Fatalf("decode task list: %v\nstdout:\n%s", err, list.stdout)
	}
	for _, row := range rows {
		if row.Title == title {
			return row.ID
		}
	}
	t.Fatalf("task %q not found after create; list:\n%s", title, list.stdout)
	return ""
}

// TestE2E_TaskVocabularyIsVisibleAndOldSpellingsAreNot pins both halves of the
// rename on the composed root: the new verbs are advertised, the old ones are
// reachable but absent from help. A rename that leaves the old name in help has
// renamed nothing.
func TestE2E_TaskVocabularyIsVisibleAndOldSpellingsAreNot(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)

	taskHelp := mustRunADB(t, workspace, "task", "--help").combined()
	for _, verb := range []string{"list", "show", "close", "remove", "timeline", "worktree"} {
		if !strings.Contains(taskHelp, "\n  "+verb+" ") {
			t.Errorf("`adb task --help` does not advertise %q:\n%s", verb, taskHelp)
		}
	}
	for _, retired := range []string{"status", "delete", "priority", "unarchive", "cleanup"} {
		if strings.Contains(taskHelp, "\n  "+retired+" ") {
			t.Errorf("`adb task --help` still advertises the retired %q:\n%s", retired, taskHelp)
		}
	}

	rootHelp := mustRunADB(t, workspace, "--help").combined()
	for _, retired := range []string{"\n  status ", "\n  work "} {
		if strings.Contains(rootHelp, retired) {
			t.Errorf("`adb --help` still advertises %q — it moved onto the task noun", strings.TrimSpace(retired))
		}
	}

	worktreeHelp := mustRunADB(t, workspace, "task", "worktree", "--help").combined()
	for _, verb := range []string{"list", "switch", "remove", "prune", "reconcile"} {
		if !strings.Contains(worktreeHelp, "\n  "+verb+" ") {
			t.Errorf("`adb task worktree --help` does not advertise %q:\n%s", verb, worktreeHelp)
		}
	}
}

// TestE2E_TaskListJSONShapeIsStable is the contract decision from spec §3: the
// document is an array with or without --git, and --git adds keys per element
// rather than switching the shape. A consumer must be able to decode without
// knowing which flags the caller passed.
func TestE2E_TaskListJSONShapeIsStable(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)

	empty := mustRunADB(t, workspace, "task", "list", "--json")
	if empty.stdout != "[]\n" {
		t.Fatalf("empty `task list --json` = %q, want %q", empty.stdout, "[]\n")
	}

	seedVocabTask(t, workspace, "shape stability")

	plain := mustRunADB(t, workspace, "task", "list", "--json")
	withGit := mustRunADB(t, workspace, "task", "list", "--json", "--git")

	var plainRows, gitRows []map[string]any
	if err := json.Unmarshal([]byte(plain.stdout), &plainRows); err != nil {
		t.Fatalf("`task list --json` is not an array: %v\n%s", err, plain.stdout)
	}
	if err := json.Unmarshal([]byte(withGit.stdout), &gitRows); err != nil {
		t.Fatalf("`task list --json --git` is not an array: %v\n%s", err, withGit.stdout)
	}
	if len(plainRows) != len(gitRows) {
		t.Fatalf("--git changed the row count: %d without, %d with — a flag must not filter", len(plainRows), len(gitRows))
	}
	if len(gitRows) != 1 {
		t.Fatalf("rows = %d, want 1", len(gitRows))
	}
	for _, key := range []string{"worktree_exists", "dirty", "ahead", "behind", "worktree_missing"} {
		if _, ok := gitRows[0][key]; !ok {
			t.Errorf("--git row is missing %q: %v", key, gitRows[0])
		}
	}
	if _, ok := plainRows[0]["dirty"]; ok {
		t.Error("plain row carries git keys; they belong to --git only")
	}
}

// `task worktree list --json` is the one task listing with an envelope, because
// an orphaned worktree has no task row to be a key on.
func TestE2E_TaskWorktreeListJSONHasAnEnvelope(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)

	out := mustRunADB(t, workspace, "task", "worktree", "list", "--json").stdout
	var report struct {
		Worktrees []map[string]any `json:"worktrees"`
		Orphaned  []string         `json:"orphaned"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, out)
	}
	if report.Worktrees == nil || report.Orphaned == nil {
		t.Errorf("both keys must be arrays, never null, so `jq '.orphaned | length'` works: %s", out)
	}
}

// `task show --json` is a single object, not a one-element array.
func TestE2E_TaskShowJSONIsASingleObject(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)
	id := seedVocabTask(t, workspace, "show one")

	out := mustRunADB(t, workspace, "task", "show", id, "--json").stdout
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("`task show --json` is not a JSON object: %v\n%s", err, out)
	}
	if row["id"] != id {
		t.Errorf("id = %v, want %s", row["id"], id)
	}

	// A typo must fail rather than print an empty object.
	if res := runADB(t, workspace, "task", "show", "TASK-99999", "--json"); res.err == nil {
		t.Error("`task show` on an unknown id succeeded, want a failure")
	}
}

// close → done, and timeline reads it back out of the event log. Together they
// prove the two new verbs are wired to real state rather than printing.
func TestE2E_TaskCloseAndTimeline(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)
	id := seedVocabTask(t, workspace, "close and replay")

	mustRunADB(t, workspace, "task", "close", id)

	show := mustRunADB(t, workspace, "task", "show", id, "--json").stdout
	var row map[string]any
	if err := json.Unmarshal([]byte(show), &row); err != nil {
		t.Fatalf("decode show: %v\n%s", err, show)
	}
	if row["status"] != "done" {
		t.Errorf("status = %v, want done", row["status"])
	}

	timeline := mustRunADB(t, workspace, "task", "timeline", id, "--json").stdout
	var events []struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(timeline), &events); err != nil {
		t.Fatalf("`task timeline --json` is not an array: %v\n%s", err, timeline)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
		if e.Data["task_id"] != id {
			t.Errorf("timeline leaked an event for %v, want only %s", e.Data["task_id"], id)
		}
	}
	for _, want := range []string{"task.created", "task.status_changed"} {
		if !seen[want] {
			t.Errorf("timeline is missing %q; got %v", want, seen)
		}
	}
}

// Spec §4: archiving is a directory move, not a field write. `--status archived`
// must be refused (naming the command that does it properly) and `--status
// backlog` on an archived task must genuinely unarchive.
func TestE2E_TaskUpdateArchiveSemantics(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)
	id := seedVocabTask(t, workspace, "archive semantics")

	reject := runADB(t, workspace, "task", "update", id, "--status", "archived")
	if reject.err == nil {
		t.Fatal("`task update --status archived` succeeded, want a rejection")
	}
	if !strings.Contains(reject.combined(), "adb task archive") {
		t.Errorf("rejection does not name `adb task archive`:\n%s", reject.combined())
	}

	mustRunADB(t, workspace, "task", "archive", id, "--force")
	archived := mustRunADB(t, workspace, "task", "show", id, "--json").stdout
	var before map[string]any
	if err := json.Unmarshal([]byte(archived), &before); err != nil {
		t.Fatalf("decode: %v\n%s", err, archived)
	}
	ticketPath, _ := before["ticket_path"].(string)
	if !strings.Contains(ticketPath, "_archived") {
		t.Fatalf("ticket_path = %q, want it under _archived/", ticketPath)
	}

	mustRunADB(t, workspace, "task", "update", id, "--status", "backlog")
	restored := mustRunADB(t, workspace, "task", "show", id, "--json").stdout
	var after map[string]any
	if err := json.Unmarshal([]byte(restored), &after); err != nil {
		t.Fatalf("decode: %v\n%s", err, restored)
	}
	if after["status"] != "backlog" {
		t.Errorf("status = %v, want backlog", after["status"])
	}
	if path, _ := after["ticket_path"].(string); strings.Contains(path, "_archived") {
		t.Errorf("ticket_path = %q — still under _archived/, so the directory was not moved back", path)
	}
}

// Spec §5: a sweeping worktree verb previews unless --apply. Asserted through
// the retired `adb work prune` spelling as well, because that is the invocation
// whose behaviour changed.
func TestE2E_SweepingWorktreeVerbsPreviewByDefault(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)
	seedVocabTask(t, workspace, "sweep preview")

	for _, args := range [][]string{
		{"task", "worktree", "prune"},
		{"task", "worktree", "reconcile"},
		{"work", "prune"},
		{"work", "prune", "--dry-run"},
	} {
		res := mustRunADB(t, workspace, args...)
		if strings.Contains(res.stdout, "pruned:") || strings.Contains(res.stdout, "restored:") {
			t.Errorf("`adb %s` mutated without --apply:\n%s", strings.Join(args, " "), res.stdout)
		}
	}

	// --dry-run must still PARSE on the retired spelling; a flag that stopped
	// existing would break the invocation the alias exists to preserve.
	if res := runADB(t, workspace, "work", "prune", "--dry-run"); res.err != nil {
		t.Errorf("`adb work prune --dry-run` no longer parses: %v\n%s", res.err, res.combined())
	}
}

// Every retired spelling must still run, warn on STDERR, and leave stdout a
// clean pipeline. A notice on stdout is a silently broken script, which is why
// cobra's Deprecated field is not used.
func TestE2E_RetiredSpellingsWarnOnStderrOnly(t *testing.T) {
	t.Parallel()
	workspace := newVocabWorkspace(t)
	id := seedVocabTask(t, workspace, "aliases")

	for _, tc := range []struct {
		args        []string
		replacement string
	}{
		{[]string{"task", "status", "--json"}, "adb task list"},
		{[]string{"status", "--json"}, "adb task list --git"},
		{[]string{"work", "list", "--json"}, "adb task worktree list"},
	} {
		name := strings.Join(tc.args, " ")
		res := mustRunADB(t, workspace, tc.args...)
		if !strings.Contains(res.stderr, "deprecated") {
			t.Errorf("`adb %s` printed no deprecation notice on stderr (stderr %q)", name, res.stderr)
		}
		if !strings.Contains(res.stderr, tc.replacement) {
			t.Errorf("`adb %s` notice does not name %q: %q", name, tc.replacement, res.stderr)
		}
		if strings.Contains(res.stdout, "deprecated") {
			t.Errorf("`adb %s` leaked the notice onto stdout: %q", name, res.stdout)
		}
		var decoded any
		if err := json.Unmarshal([]byte(res.stdout), &decoded); err != nil {
			t.Errorf("`adb %s` stdout is not clean JSON: %v\n%s", name, err, res.stdout)
		}
	}

	// The merges keep their own retired implementation (their shapes differ from
	// the flag they merged into), so they need their own check that they still
	// do the work.
	mustRunADB(t, workspace, "task", "priority", id, "--priority", "P0")
	show := mustRunADB(t, workspace, "task", "show", id, "--json").stdout
	var row map[string]any
	if err := json.Unmarshal([]byte(show), &row); err != nil {
		t.Fatalf("decode: %v\n%s", err, show)
	}
	if row["priority"] != "P0" {
		t.Errorf("priority = %v, want P0 — the retired `task priority` alias did not act", row["priority"])
	}
}
