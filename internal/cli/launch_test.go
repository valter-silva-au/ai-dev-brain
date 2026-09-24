package cli

import (
	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSerenaActivationHint covers the F8 (#212) launch hint: a worktree path
// yields a hint naming it (instance-per-project); an empty path yields nothing.
func TestSerenaActivationHint(t *testing.T) {
	if got := serenaActivationHint(""); got != "" {
		t.Errorf("empty worktree should yield no hint, got %q", got)
	}
	got := serenaActivationHint("/work/TASK-1")
	if !strings.Contains(got, "/work/TASK-1") {
		t.Errorf("hint should name the worktree path, got %q", got)
	}
	if !strings.Contains(got, "instance-per-project") || !strings.Contains(got, ".serena/project.yml") {
		t.Errorf("hint should describe instance-per-project activation, got %q", got)
	}
}

// TestClaudeProjectDir verifies the path munge Claude Code uses to key its
// per-project conversation store: every '/' and '.' in the absolute worktree
// path becomes '-', under ~/.claude/projects/.
func TestClaudeProjectDir(t *testing.T) {
	tests := []struct {
		home string
		path string
		want string
	}{
		{"/Users/v", "/Users/v/Code/myproject", "/Users/v/.claude/projects/-Users-v-Code-myproject"},
		{"/home/u", "/home/u/Code/myproject/work/TASK-1", "/home/u/.claude/projects/-home-u-Code-myproject-work-TASK-1"},
		{"/h", "/h/x/.obsidian", "/h/.claude/projects/-h-x--obsidian"},
		// A Windows-style path: the drive ':' and '\' separators must munge to
		// '-' so <munged> stays a single valid path component on every OS.
		{"/h", `C:\proj\x`, "/h/.claude/projects/C--proj-x"},
	}
	for _, tt := range tests {
		// want is written with '/' separators for readability; the production
		// path is filepath.Join-ed, so compare against the OS-native form.
		want := filepath.FromSlash(tt.want)
		got := claudeProjectDir(tt.home, tt.path)
		if got != want {
			t.Errorf("claudeProjectDir(%q, %q) = %q, want %q", tt.home, tt.path, got, want)
		}
	}
}

// TestConversationExists verifies detection of a prior Claude conversation for a
// worktree: true iff ~/.claude/projects/<munged>/ holds at least one *.jsonl.
func TestConversationExists(t *testing.T) {
	home := t.TempDir()
	wt := t.TempDir()

	// no project dir yet -> no conversation
	if conversationExistsIn(home, wt) {
		t.Fatalf("expected no conversation before any jsonl exists")
	}

	// create the munged project dir but with a non-jsonl file -> still none
	pdir := claudeProjectDir(home, wt)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if conversationExistsIn(home, wt) {
		t.Fatalf("expected no conversation when only non-jsonl files exist")
	}

	// add a .jsonl transcript -> conversation exists
	if err := os.WriteFile(filepath.Join(pdir, "abc.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !conversationExistsIn(home, wt) {
		t.Fatalf("expected a conversation once a *.jsonl transcript exists")
	}
}

// TestClaudeArgs verifies the launch argument decision: --continue is passed
// ONLY when resume is requested AND a prior conversation exists for the cwd.
// A fresh worktree (resume=true, no conversation) must start a NEW session, not
// crash on `claude --continue` -> "No conversation found to continue".
func TestClaudeArgs(t *testing.T) {
	base := []string{"--dangerously-skip-permissions"}
	tests := []struct {
		name     string
		resume   bool
		hasConvo bool
		wantCont bool
	}{
		{"create (resume=false) never continues", false, false, false},
		{"resume with existing conversation continues", true, true, true},
		{"resume with NO conversation starts fresh (the bug)", true, false, false},
		{"resume=false ignores an existing conversation", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := claudeArgs(tt.resume, tt.hasConvo)
			if len(args) < len(base) || args[0] != base[0] {
				t.Fatalf("claudeArgs must always include %v, got %v", base, args)
			}
			gotCont := false
			for _, a := range args {
				if a == "--continue" {
					gotCont = true
				}
			}
			if gotCont != tt.wantCont {
				t.Errorf("claudeArgs(resume=%v, hasConvo=%v) --continue=%v, want %v",
					tt.resume, tt.hasConvo, gotCont, tt.wantCont)
			}
		})
	}
}

// TestInteractiveShell verifies the drop-to-shell fallback selection: Windows
// prefers $ComSpec (then cmd.exe); POSIX uses $SHELL (then /bin/bash). This is
// the TASK-00006 fix for the hardcoded /bin/bash that failed on Windows with
// "exec: /bin/bash: executable file not found in %PATH%".
func TestInteractiveShell(t *testing.T) {
	tests := []struct {
		name     string
		shellEnv string
		comspec  string
		goos     string
		want     string
	}{
		{"windows prefers ComSpec", "/usr/bin/bash", `C:\Windows\System32\cmd.exe`, "windows", `C:\Windows\System32\cmd.exe`},
		{"windows with no ComSpec falls back to cmd.exe", "", "", "windows", "cmd.exe"},
		{"posix uses $SHELL", "/bin/zsh", "", "linux", "/bin/zsh"},
		{"posix with no $SHELL falls back to /bin/bash", "", "", "linux", "/bin/bash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interactiveShell(tt.shellEnv, tt.comspec, tt.goos); got != tt.want {
				t.Errorf("interactiveShell(%q, %q, %q) = %q, want %q",
					tt.shellEnv, tt.comspec, tt.goos, got, tt.want)
			}
		})
	}
}

// TestTmuxSessionName verifies the deterministic session-name derivation is
// byte-identical to ~/.local/bin/cc-survive: "cc-" + basename, with every byte
// outside [A-Za-z0-9_-] mapped to '-', runs of '-' collapsed, and leading/
// trailing '-' trimmed. The same folder MUST always map to the same name —
// that's what lets adb and the "🌙 claude (tmux)" profile share one session.
// tmuxSessionName reads ADB_TMUX_PREFIX; unset the env so this test isolates
// the default-prefix path regardless of the caller's shell.
func TestTmuxSessionName(t *testing.T) {
	t.Setenv("ADB_TMUX_PREFIX", "")
	tests := []struct {
		path string
		want string
	}{
		// plain basename
		{"/home/u/work/myproject", "cc-myproject"},
		// dots (tmux forbids '.' in names) become dashes
		{"/home/u/my.project", "cc-my-project"},
		// a worktree leaf with the canonical TASK id survives intact
		{"/home/u/work/TASK-00002-some-slug", "cc-TASK-00002-some-slug"},
		// runs of illegal chars collapse to a single dash
		{"/tmp/a..  b", "cc-a-b"},
		// leading/trailing illegal chars are trimmed, not left as edge dashes
		{"/tmp/.hidden.", "cc-hidden"},
		// trailing slash → basename is the last real component
		{"/home/u/proj/", "cc-proj"},
	}
	for _, tt := range tests {
		if got := tmuxSessionName(tt.path); got != tt.want {
			t.Errorf("tmuxSessionName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestTmuxSessionNameWithPrefix verifies the tmux session-name derivation
// with a configurable prefix (adb.tmux.sessionPrefix, threaded through
// ADB_TMUX_PREFIX). The prefix + basename go through a SINGLE sanitizing
// loop so illegal chars in either — including on the boundary — collapse
// consistently. Empty prefix falls back to "cc-" (existing behaviour).
func TestTmuxSessionNameWithPrefix(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		want   string
	}{
		// default (empty) → "cc-"
		{"", "/home/u/work/TASK-00002-slug", "cc-TASK-00002-slug"},
		// explicit default is a no-op
		{"cc-", "/home/u/work/TASK-00002-slug", "cc-TASK-00002-slug"},
		// custom legal prefix
		{"adb-", "/home/u/myproj", "adb-myproj"},
		// illegal prefix chars → dashes; runs collapse across the boundary
		{"a.b:", "/home/u/proj", "a-b-proj"},
		// illegal chars in the basename still sanitized
		{"cc-", "/home/u/my.project", "cc-my-project"},
		// canonical TASK id survives
		{"cc-", "/home/u/work/TASK-00002-some-slug", "cc-TASK-00002-some-slug"},
		// leading/trailing illegal chars trimmed
		{"cc-", "/tmp/.hidden.", "cc-hidden"},
		// trailing slash → basename is the last real component
		{"cc-", "/home/u/proj/", "cc-proj"},
	}
	for _, tt := range tests {
		if got := tmuxSessionNameWithPrefix(tt.prefix, tt.path); got != tt.want {
			t.Errorf("tmuxSessionNameWithPrefix(%q,%q) = %q, want %q", tt.prefix, tt.path, got, tt.want)
		}
	}
}

// TestShouldUseTmux verifies the host-in-tmux decision: only when tmux is
// enabled by config (ADB_TMUX!=0), tmux is on PATH, and we're not already
// inside a tmux session (no nesting). The enabled gate is what lets a user
// set adb.tmux.enabled=false in VS Code to opt out of the durable-session
// path.
func TestShouldUseTmux(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		onPath     bool
		insideTmux bool
		goos       string
		want       bool
	}{
		{"enabled + available + not nested (posix) → use it", true, true, false, "linux", true},
		{"disabled by config → never use tmux", false, true, false, "linux", false},
		{"enabled but already inside tmux → don't nest", true, true, true, "linux", false},
		{"enabled but no tmux on PATH → direct launch", true, false, false, "linux", false},
		{"disabled + no tmux → false", false, false, false, "linux", false},
		{"disabled + inside tmux → false", false, true, true, "linux", false},
		{"windows: never use tmux even when available (TASK-00006)", true, true, false, "windows", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseTmux(tt.enabled, tt.onPath, tt.insideTmux, tt.goos); got != tt.want {
				t.Errorf("shouldUseTmux(enabled=%v, onPath=%v, insideTmux=%v, goos=%q) = %v, want %v",
					tt.enabled, tt.onPath, tt.insideTmux, tt.goos, got, tt.want)
			}
		})
	}
}

// TestTmuxEnabledFromEnv verifies the ADB_TMUX gate parser: unset defaults
// to enabled (durable session, existing behaviour), "0" or "false" disable,
// any other value defaults to enabled (fail toward the durable path).
func TestTmuxEnabledFromEnv(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"", true},     // unset → default enabled
		{"1", true},    // explicit on
		{"true", true}, // explicit on (canonical form)
		{"0", false},   // explicit off
		{"false", false},
		{"yes", true}, // any other truthy-ish string → default enabled
	}
	for _, tt := range tests {
		if got := tmuxEnabledFromEnv(tt.val); got != tt.want {
			t.Errorf("tmuxEnabledFromEnv(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

// TestTmuxArgs verifies the attach-or-create argv: idempotent (-A), detaches a
// stale client (-D), names the session deterministically (-s), starts in the
// task dir (-c), and the inner command runs claude with the given args then
// drops to a login shell so the window/tab stays usable after claude exits.
func TestTmuxArgs(t *testing.T) {
	args := tmuxArgs("cc-myproject", "/home/u/work/myproject", "claude",
		[]string{"--dangerously-skip-permissions", "--continue"})

	// Leading flags, in order.
	want := []string{"new-session", "-A", "-D", "-s", "cc-myproject", "-c", "/home/u/work/myproject"}
	for i, w := range want {
		if i >= len(args) || args[i] != w {
			t.Fatalf("tmuxArgs prefix = %v, want prefix %v", args, want)
		}
	}

	// The inner command is the final arg.
	inner := args[len(args)-1]
	if !strings.HasPrefix(inner, "claude --dangerously-skip-permissions --continue") {
		t.Errorf("inner command must launch claude with its args, got %q", inner)
	}
	if !strings.Contains(inner, "exec") || !strings.Contains(inner, "SHELL") {
		t.Errorf("inner command must fall back to a login shell after claude exits, got %q", inner)
	}
}

// TestPiSessionDir verifies that adb computes the same session directory pi
// itself does (mirroring getDefaultSessionDirPath in pi's session-manager.js):
// ~/.pi/agent/sessions/--<cwd with the leading slash stripped and every '/',
// '\' and ':' replaced by '-'>--.
func TestPiSessionDir(t *testing.T) {
	tests := []struct {
		home string
		path string
		want string
	}{
		{"/Users/v", "/Users/v/Code/myproject", "/Users/v/.pi/agent/sessions/--Users-v-Code-myproject--"},
		{"/home/u", "/home/u/work/TASK-1", "/home/u/.pi/agent/sessions/--home-u-work-TASK-1--"},
		// pi keeps '.' verbatim (unlike Claude Code's project-dir munge).
		{"/h", "/h/x/.obsidian", "/h/.pi/agent/sessions/--h-x-.obsidian--"},
	}
	for _, tt := range tests {
		want := filepath.FromSlash(tt.want)
		got := piSessionDir(tt.home, tt.path)
		if got != want {
			t.Errorf("piSessionDir(%q, %q) = %q, want %q", tt.home, tt.path, got, want)
		}
	}
}

// TestPiSessionDirName verifies the pure name munge on an already-absolute
// path, including a Windows-style path: leading drive + '\' separators must
// munge so <munged> stays a single valid path component on every OS.
func TestPiSessionDirName(t *testing.T) {
	if got, want := piSessionDirName(`C:\proj\x`), "C--proj-x"; got != want {
		t.Errorf("piSessionDirName = %q, want %q", got, want)
	}
}

// TestPiConversationExistsIn verifies detection of a prior pi session for a
// project: true iff the project's session dir holds at least one *.jsonl.
func TestPiConversationExists(t *testing.T) {
	home := t.TempDir()
	wt := t.TempDir()

	if piConversationExistsIn(home, wt) {
		t.Fatal("expected no session before any jsonl exists")
	}

	dir := piSessionDir(home, wt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Non-session files must not count.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if piConversationExistsIn(home, wt) {
		t.Fatal("non-jsonl files must not count as a session")
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-01-01T00-00-00-000Z.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !piConversationExistsIn(home, wt) {
		t.Fatal("expected session to be detected once a jsonl transcript exists")
	}
}

// TestPiArgs verifies the guarded --continue: appended only when a resume was
// requested AND a prior session exists — pi's -c continues the most recent
// session for the cwd, and a fresh worktree has none.
func TestPiArgs(t *testing.T) {
	tests := []struct {
		resume bool
		exists bool
		want   []string
	}{
		{false, false, nil},
		{true, false, nil},
		{false, true, nil},
		{true, true, []string{"--continue"}},
	}
	for _, tt := range tests {
		got := piArgs(tt.resume, tt.exists)
		if len(got) != len(tt.want) {
			t.Fatalf("piArgs(resume=%v, exists=%v) = %v, want %v", tt.resume, tt.exists, got, tt.want)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("piArgs(resume=%v, exists=%v) = %v, want %v", tt.resume, tt.exists, got, tt.want)
			}
		}
	}
}

// TestResolveAgent verifies the layered agent selection: flag > ADB_AGENT env >
// config > claude default, with unknown values rejected rather than silently
// falling back.
func TestResolveAgent(t *testing.T) {
	t.Setenv("ADB_AGENT", "")
	got, err := resolveAgentWithConfig("", "", "")
	if err != nil || got != "claude" {
		t.Fatalf("default agent = (%q, %v), want (\"claude\", nil)", got, err)
	}

	got, err = resolveAgentWithConfig("", "pi", "")
	if err != nil || got != "pi" {
		t.Fatalf("env=pi, no flag/config → (%q, %v), want (\"pi\", nil)", got, err)
	}

	got, err = resolveAgentWithConfig("claude", "pi", "pi")
	if err != nil || got != "claude" {
		t.Fatalf("flag wins over env and config → (%q, %v), want (\"claude\", nil)", got, err)
	}

	got, err = resolveAgentWithConfig("", "", "pi")
	if err != nil || got != "pi" {
		t.Fatalf("config wins over default → (%q, %v), want (\"pi\", nil)", got, err)
	}

	got, err = resolveAgentWithConfig("", "claude", "pi")
	if err != nil || got != "claude" {
		t.Fatalf("env wins over config → (%q, %v), want (\"claude\", nil)", got, err)
	}

	// `codex` used to be the stand-in for "unknown" here. It is a REGISTERED
	// agent as of TASK-00039 Q3, so the negative case needs a name that is
	// genuinely not an agent — otherwise this test would pass by asserting the
	// opposite of the truth.
	if _, err := resolveAgentWithConfig("gpt5", "", ""); err == nil {
		t.Fatal("unknown agent must be rejected")
	} else if !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "codex") {
		t.Errorf("error should list the valid agents, got %v", err)
	}

	// A bad config value must also be rejected, not silently ignored.
	if _, err := resolveAgentWithConfig("", "", "gpt5"); err == nil {
		t.Fatal("unknown agent from config must be rejected")
	}
}

// TestConfiguredAgent verifies the launch_agent custom setting is read from
// the layered config (repo > org > global) and empty when unset/unavailable.
func TestConfiguredAgent(t *testing.T) {
	saved := App
	defer func() { App = saved }()

	App = nil
	if got := configuredAgent(); got != "" {
		t.Fatalf("App nil → %q, want empty", got)
	}

	// Isolated App, global tier (repointed at <dir>/.taskconfig by isolation),
	// key not set → empty.
	dir := t.TempDir()
	app, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app
	if got := configuredAgent(); got != "" {
		t.Fatalf("no custom setting → %q, want empty", got)
	}

	// Set the global tier's custom_settings and rebuild the App so the config
	// manager re-reads it.
	if err := os.WriteFile(filepath.Join(dir, ".taskconfig"),
		[]byte("custom_settings:\n  launch_agent: pi\n"), 0o644); err != nil {
		t.Fatalf("write .taskconfig: %v", err)
	}
	app2, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("NewAppIsolated (2nd): %v", err)
	}
	defer app2.Cleanup()
	App = app2
	if got := configuredAgent(); got != "pi" {
		t.Fatalf("global custom_settings launch_agent → %q, want pi", got)
	}
}

// TestAgentDisplayName verifies the launch-message naming: claude keeps its
// historical "Claude Code" label; other agents print as their binary name.
func TestAgentDisplayName(t *testing.T) {
	if got := agentDisplayName("claude"); got != "Claude Code" {
		t.Errorf("claude display name = %q, want %q", got, "Claude Code")
	}
	if got := agentDisplayName("pi"); got != "pi" {
		t.Errorf("pi display name = %q, want %q", got, "pi")
	}
	// Empty (a pre-agent launch file) must behave as claude, never print "".
	if got := agentDisplayName(""); got != "Claude Code" {
		t.Errorf("empty agent display name = %q, want %q", got, "Claude Code")
	}
}

// TestPiTmuxPrefix verifies pi's tmux namespace: default "pi-" (so a pi
// session never collides with a claude "cc-" session in the same folder),
// overridable by ADB_TMUX_PREFIX like the claude namespace.
func TestPiTmuxPrefix(t *testing.T) {
	t.Setenv("ADB_TMUX_PREFIX", "")
	if got := piTmuxPrefix(); got != "pi-" {
		t.Errorf("default pi tmux prefix = %q, want %q", got, "pi-")
	}
	t.Setenv("ADB_TMUX_PREFIX", "adb-")
	if got := piTmuxPrefix(); got != "adb-" {
		t.Errorf("ADB_TMUX_PREFIX override = %q, want %q", got, "adb-")
	}
}

// TestLaunchAgentRouting verifies launchAgent dispatches to the right agent
// launcher (observable via the PATH-lookup error naming the right binary).
func TestLaunchAgentRouting(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no agent binaries on PATH
	if err := launchAgent("pi", t.TempDir(), false); err == nil || !strings.Contains(err.Error(), "pi CLI not found in PATH") {
		t.Errorf("pi launch should fail with a pi PATH error, got %v", err)
	}
	if err := launchAgent("claude", t.TempDir(), false); err == nil || !strings.Contains(err.Error(), "claude CLI not found in PATH") {
		t.Errorf("claude launch should fail with a claude PATH error, got %v", err)
	}
}

// TestTaskFlagsHaveAgent verifies 'task create' and 'task resume' expose the
// --agent flag with claude|pi documented.
func TestTaskFlagsHaveAgent(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"task create", newTaskCreateCmd()},
		{"task resume", newTaskResumeCmd()},
	} {
		flag := tc.cmd.Flags().Lookup("agent")
		if flag == nil {
			t.Fatalf("expected --agent flag on %s", tc.name)
		}
		if !strings.Contains(flag.Usage, "claude") || !strings.Contains(flag.Usage, "pi") {
			t.Errorf("%s --agent usage should document claude|pi, got %q", tc.name, flag.Usage)
		}
	}
}
