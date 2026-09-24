package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Adding a coding agent used to mean five scattered edits: an entry in
// validAgents, a case in agentDisplayName, an args builder, a prior-session
// probe, and a branch in each of launchAgent and priorSessionExists. Five places
// is how an agent ends up half-wired — accepted by --agent, then launching
// claude, which looks like it worked.
//
// So agents are DATA now: one descriptor in the registry, and every dispatch
// reads it. These tests pin that property rather than any one agent's flags.

func TestAgentRegistry_EveryAgentIsFullyDescribed(t *testing.T) {
	if len(agentRegistry) == 0 {
		t.Fatal("agentRegistry is empty")
	}
	for name, agent := range agentRegistry {
		if name == "" {
			t.Error("registry has an empty key")
		}
		if agent.Name != name {
			t.Errorf("registry key %q holds an agent named %q", name, agent.Name)
		}
		if agent.Binary == "" {
			t.Errorf("%s: no Binary, so there is nothing to exec", name)
		}
		if agent.DisplayName == "" {
			t.Errorf("%s: no DisplayName, so launch messages would be blank", name)
		}
		if agent.Args == nil {
			t.Errorf("%s: no Args builder", name)
		}
		if agent.TmuxPrefix == "" {
			t.Errorf("%s: no TmuxPrefix — two agents in one worktree would collide on one tmux session", name)
		}
	}
}

// validAgents is what --agent validates against and what the error message
// lists. Derived from the registry, never hand-maintained beside it.
func TestValidAgents_IsDerivedFromTheRegistry(t *testing.T) {
	got := validAgentNames()
	want := make([]string, 0, len(agentRegistry))
	for name := range agentRegistry {
		want = append(want, name)
	}
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("validAgentNames() = %v, want %v (sorted registry keys)", got, want)
	}
	if len(got) < 2 {
		t.Errorf("only %d agents; the registry should carry at least claude and pi", len(got))
	}
}

// The two agents that predate the registry must keep behaving exactly as before —
// this is a refactor, and their flags are the contract.
func TestAgentRegistry_PreservesClaudeAndPiBehaviour(t *testing.T) {
	claude, ok := agentRegistry["claude"]
	if !ok {
		t.Fatal("claude is missing from the registry")
	}
	if got := claude.Args(false, false); strings.Join(got, " ") != "--dangerously-skip-permissions" {
		t.Errorf("claude fresh args = %v, want [--dangerously-skip-permissions]", got)
	}
	if got := claude.Args(true, true); strings.Join(got, " ") != "--dangerously-skip-permissions --continue" {
		t.Errorf("claude resume args = %v, want the skip-permissions flag plus --continue", got)
	}
	// A resume with NO prior session must not pass --continue: the real claude
	// CLI exits 1 with "No conversation found to continue".
	if got := claude.Args(true, false); strings.Contains(strings.Join(got, " "), "--continue") {
		t.Errorf("claude resume-without-prior-session args = %v, must not carry --continue", got)
	}

	pi, ok := agentRegistry["pi"]
	if !ok {
		t.Fatal("pi is missing from the registry")
	}
	if got := pi.Args(false, false); len(got) != 0 {
		t.Errorf("pi fresh args = %v, want none (pi has no skip-permissions equivalent)", got)
	}
	if got := pi.Args(true, true); strings.Join(got, " ") != "--continue" {
		t.Errorf("pi resume args = %v, want [--continue]", got)
	}
	if got := pi.Args(true, false); len(got) != 0 {
		t.Errorf("pi resume-without-prior-session args = %v, want none", got)
	}
	if pi.TmuxPrefix == claude.TmuxPrefix {
		t.Error("pi and claude share a tmux prefix; sessions in one worktree would collide")
	}
}

// --- Codex (TASK-00039 Q3) --------------------------------------------------

// Codex's resume is a SUBCOMMAND, not a flag, which is why the descriptor owns
// the whole argv rather than just trailing flags.
func TestCodex_ArgsUseTheResumeSubcommand(t *testing.T) {
	codex, ok := agentRegistry["codex"]
	if !ok {
		t.Fatal("codex is missing from the registry")
	}

	if got := codex.Args(false, false); len(got) != 0 {
		t.Errorf("codex fresh args = %v, want none (bare `codex` opens the interactive CLI)", got)
	}
	if got := strings.Join(codex.Args(true, true), " "); got != "resume --last" {
		t.Errorf("codex resume args = %q, want \"resume --last\"", got)
	}
	// `codex resume --last` filters by cwd by DEFAULT (its `--all` flag is
	// documented as "disables cwd filtering"), so it cannot wander into another
	// worktree's conversation. But with no session for this directory at all we
	// still start fresh rather than handing it a resume it cannot satisfy.
	if got := codex.Args(true, false); len(got) != 0 {
		t.Errorf("codex resume-without-prior-session args = %v, want none", got)
	}
}

// Codex stores sessions DATE-partitioned (~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl),
// not keyed by project path the way Claude Code does — so the probe has to read
// each rollout's first line and compare its recorded cwd.
func TestCodexSessionExistsIn_MatchesTheRecordedCwd(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "09", "16")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	worktree := "/ws/work/github.com/acme/widget/TASK-00001-thing"
	other := "/ws/work/github.com/acme/widget/TASK-00002-other"

	write := func(name, cwd string) {
		line := `{"type":"session_meta","payload":{"cwd":"` + cwd + `","id":"abc"}}` + "\n"
		if err := os.WriteFile(filepath.Join(day, name), []byte(line), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("rollout-2026-09-16T09-00-00-aaa.jsonl", other)

	if codexSessionExistsIn(home, worktree) {
		t.Error("reported a prior session when only ANOTHER directory had one — that is the bug that would resume the wrong task's conversation")
	}

	write("rollout-2026-09-16T10-00-00-bbb.jsonl", worktree)
	if !codexSessionExistsIn(home, worktree) {
		t.Error("did not find the session recorded for this directory")
	}
}

func TestCodexSessionExistsIn_ToleratesJunk(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "09", "16")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range map[string]string{
		"rollout-a.jsonl": "not json at all\n",
		"rollout-b.jsonl": "",
		"rollout-c.jsonl": `{"payload":{}}` + "\n",
		"rollout-d.jsonl": `{"payload":{"cwd":123}}` + "\n",
	} {
		if err := os.WriteFile(filepath.Join(day, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Must not panic, and must not claim a match.
	if codexSessionExistsIn(home, "/ws/anything") {
		t.Error("claimed a match from unparseable session files")
	}
}

// A missing home, or no ~/.codex at all, is "no prior session" — never an error
// and never a crash, because this only decides which sentence gets printed.
func TestCodexSessionExistsIn_NoSessionsDir(t *testing.T) {
	if codexSessionExistsIn(t.TempDir(), "/ws/work/x") {
		t.Error("claimed a prior session with no ~/.codex at all")
	}
}

// --- Amazon Q (TASK-00039 Q3) ----------------------------------------------

// Amazon Q's interactive entry point is `q chat`, so the subcommand is part of
// the argv for BOTH a fresh start and a resume.
func TestAmazonQ_ArgsUseChatSubcommand(t *testing.T) {
	q, ok := agentRegistry["amazon-q"]
	if !ok {
		t.Fatal("amazon-q is missing from the registry")
	}
	if q.Binary != "q" {
		t.Errorf("binary = %q, want q", q.Binary)
	}
	if got := strings.Join(q.Args(false, false), " "); got != "chat" {
		t.Errorf("fresh args = %q, want \"chat\"", got)
	}
	if got := strings.Join(q.Args(true, true), " "); got != "chat --resume" {
		t.Errorf("resume args = %q, want \"chat --resume\"", got)
	}
	if got := strings.Join(q.Args(true, false), " "); got != "chat" {
		t.Errorf("resume-without-prior-session args = %q, want \"chat\"", got)
	}
}

// --- dispatch ---------------------------------------------------------------

// resolveAgent must accept every registered agent and reject anything else with
// the full list, so a typo names its alternatives instead of silently launching
// claude.
func TestResolveAgent_AcceptsEveryRegisteredAgent(t *testing.T) {
	for name := range agentRegistry {
		got, err := resolveAgentWithConfig(name, "", "")
		if err != nil {
			t.Errorf("resolveAgent(%q) errored: %v", name, err)
		}
		if got != name {
			t.Errorf("resolveAgent(%q) = %q", name, got)
		}
	}

	_, err := resolveAgentWithConfig("gpt5", "", "")
	if err == nil {
		t.Fatal("an unknown agent was accepted")
	}
	for name := range agentRegistry {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not list the valid agent %q", err, name)
		}
	}
}

// Every registered agent must resolve to a display name and a prior-session
// probe through the registry — no agent may fall back to claude's, which is what
// made a half-wired agent look like it worked.
func TestAgentDispatch_CoversEveryRegisteredAgent(t *testing.T) {
	for name, agent := range agentRegistry {
		if got := agentDisplayName(name); got != agent.DisplayName {
			t.Errorf("agentDisplayName(%q) = %q, want %q", name, got, agent.DisplayName)
		}
		// An empty path is always "no prior session" for every agent; this is the
		// cheap way to prove each one is reachable without touching a real home.
		if priorSessionExists(name, "") {
			t.Errorf("priorSessionExists(%q, \"\") = true, want false", name)
		}
	}
}

// --- flag surface -----------------------------------------------------------

// The --agent usage string used to be hardcoded to "claude or pi" in three
// places. It is documentation shipped inside the binary, so the moment an agent
// is registered it starts lying — and this was found by reading `--help` on a
// real build, not by any test.
func TestAgentFlagUsage_ListsEveryRegisteredAgent(t *testing.T) {
	usage := agentFlagUsage()
	for name := range agentRegistry {
		if !strings.Contains(usage, name) {
			t.Errorf("--agent usage does not mention %q: %s", name, usage)
		}
	}
}

// Every command that takes --agent must carry the derived usage, not a literal.
func TestAgentFlagUsage_IsWiredIntoEveryCommandThatTakesIt(t *testing.T) {
	for name, build := range map[string]func() *cobra.Command{
		"task create": newTaskCreateCmd,
		"task start":  newTaskStartCmd,
		"task resume": newTaskResumeCmd,
	} {
		flag := build().Flags().Lookup("agent")
		if flag == nil {
			t.Errorf("%s has no --agent flag", name)
			continue
		}
		for agent := range agentRegistry {
			if !strings.Contains(flag.Usage, agent) {
				t.Errorf("%s --agent usage omits %q: %s", name, agent, flag.Usage)
			}
		}
	}
}

// An invalid --agent must be rejected UP FRONT, whether or not a launch will
// happen.
//
// The bug: resolveAgent was only called inside the launch block, which a
// repo-less task never reaches (no worktree → nothing to launch). So
// `adb task create "x" --agent gpt5` printed "✓ Task TASK-00001 created" and
// exited 0, having silently ignored the flag. Found by driving the installed
// binary. Same class as the `--filter Done` defect fixed earlier in this ticket:
// a typo earns a success.
func TestAgentFlag_InvalidIsRejectedEvenWithNothingToLaunch(t *testing.T) {
	newTaskWorkspace(t)

	// A repo-less task: no worktree, so no launch — the path that used to skip
	// validation entirely.
	_, _, err := runCmd(t, newTaskCreateCmd(), "no worktree here", "--type", "chore", "--agent", "gpt5")
	if err == nil {
		t.Fatal("`task create --agent gpt5` succeeded; an invalid agent must be rejected")
	}
	if !strings.Contains(err.Error(), "gpt5") {
		t.Errorf("error does not name the bad value: %v", err)
	}

	// And nothing should have been created.
	backlog, loadErr := App.BacklogManager.Load()
	if loadErr != nil {
		t.Fatalf("load backlog: %v", loadErr)
	}
	if len(backlog.Tasks) != 0 {
		t.Errorf("a task was created despite the rejection: %d tasks", len(backlog.Tasks))
	}
}

func TestAgentFlag_InvalidIsRejectedOnStartAndResume(t *testing.T) {
	newTaskWorkspace(t)
	task := seedTask(t, "for start and resume", models.TaskTypeChore)

	for name, build := range map[string]func() *cobra.Command{
		"start":  newTaskStartCmd,
		"resume": newTaskResumeCmd,
	} {
		if _, _, err := runCmd(t, build(), task.ID, "--agent", "gpt5"); err == nil {
			t.Errorf("`task %s --agent gpt5` succeeded; an invalid agent must be rejected", name)
		}
	}
}
