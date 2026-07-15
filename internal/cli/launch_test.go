package cli

import (
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
		{"/Users/v", "/Users/v/Code/workspace", "/Users/v/.claude/projects/-Users-v-Code-workspace"},
		{"/home/u", "/home/u/Code/valter/work/TASK-1", "/home/u/.claude/projects/-home-u-Code-valter-work-TASK-1"},
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

// TestShouldLaunchInPlaceFor verifies the pure launch-mode decision across OSes:
//   - here == true              -> always in-place (skip the VS Code launch-file bounce)
//   - goos == "windows"         -> always in-place; the extension's `bash -l -c`
//     hand-off can't run on Windows, so bouncing is a silent no-op (TASK-00006)
//   - here == false in vscode   -> bounce via VS Code (launchViaVSCode) — POSIX only
//   - here == false in plain    -> in-place (launchClaudeCode directly)
func TestShouldLaunchInPlaceFor(t *testing.T) {
	tests := []struct {
		name        string
		here        bool
		termProgram string
		goos        string
		want        bool
	}{
		{"here=true in vscode bypasses the bounce", true, "vscode", "linux", true},
		{"here=true in a plain terminal launches in place", true, "", "linux", true},
		{"posix: here=false in vscode bounces via VS Code", false, "vscode", "linux", false},
		{"posix: here=false in a plain terminal launches in place", false, "", "linux", true},
		{"posix: here=false in iTerm.app launches in place", false, "iTerm.app", "darwin", true},
		{"windows: vscode still launches in place (TASK-00006)", false, "vscode", "windows", true},
		{"windows: plain terminal launches in place", false, "", "windows", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldLaunchInPlaceFor(tt.here, tt.termProgram, tt.goos)
			if got != tt.want {
				t.Errorf("shouldLaunchInPlaceFor(here=%v, term=%q, goos=%q) = %v, want %v",
					tt.here, tt.termProgram, tt.goos, got, tt.want)
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
		{"/home/user/Code/project", "cc-project"},
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
		{"adb-", "/home/u/valter", "adb-valter"},
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
	args := tmuxArgs("cc-project", "/home/user/Code/project",
		[]string{"--dangerously-skip-permissions", "--continue"})

	// Leading flags, in order.
	want := []string{"new-session", "-A", "-D", "-s", "cc-project", "-c", "/home/user/Code/project"}
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

// TestTaskResumeHasHereFlag verifies that 'adb task resume' exposes a --here flag
// with the documented help text describing the in-place behavior.
func TestTaskResumeHasHereFlag(t *testing.T) {
	cmd := newTaskResumeCmd()

	flag := cmd.Flags().Lookup("here")
	if flag == nil {
		t.Fatal("Expected --here flag to be defined on 'task resume'")
	}

	if flag.Value.Type() != "bool" {
		t.Errorf("Expected --here to be a bool flag, got %q", flag.Value.Type())
	}

	if flag.DefValue != "false" {
		t.Errorf("Expected --here default to be false, got %q", flag.DefValue)
	}

	// Help text must mention the in-place / current-terminal intent so users
	// understand what skipping the VS Code bounce means.
	if flag.Usage == "" {
		t.Error("Expected --here flag to have a usage description")
	}
}
