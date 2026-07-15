package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// taskLaunchInfo carries task metadata through the launch workflow
type taskLaunchInfo struct {
	TaskID       string `json:"task_id"`
	TaskType     string `json:"task_type"`
	Priority     string `json:"priority"`
	Status       string `json:"status"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	Resume       bool   `json:"resume"`
	// Here forces an in-place launch (exec claude in the current terminal),
	// bypassing the TERM_PROGRAM=vscode -> launchViaVSCode handoff. The VS Code
	// extension sets this when it has already created a styled terminal and
	// just wants the binary to run claude there.
	Here      bool   `json:"here,omitempty"`
	Timestamp string `json:"timestamp"`
}

// shouldLaunchInPlace decides whether launchWorkflow should exec claude
// directly in the current terminal or write the VS Code launch file and let
// the extension pick it up. It delegates to the pure shouldLaunchInPlaceFor.
func shouldLaunchInPlace(info taskLaunchInfo) bool {
	return shouldLaunchInPlaceFor(info.Here, os.Getenv("TERM_PROGRAM"), runtime.GOOS)
}

// shouldLaunchInPlaceFor is the pure launch-mode decision. Rules:
//   - here == true             -> always in place (caller owns the terminal)
//   - goos == "windows"        -> always in place. The VS Code hand-off has the
//     extension compose a Unix `bash -l -c` login-shell wrapper, which cannot run
//     on Windows — so bouncing there is a silent no-op (it merely rewrites
//     ~/.adb_terminal_launch.json). Launch claude directly instead.
//   - TERM_PROGRAM == "vscode" -> bounce via VS Code (in_place = false)
//   - otherwise                -> in place (plain terminal)
func shouldLaunchInPlaceFor(here bool, termProgram, goos string) bool {
	if here {
		return true
	}
	if goos == "windows" {
		return true
	}
	return termProgram != "vscode"
}

// launchWorkflow launches the workflow for a task.
// In VS Code, it delegates to the extension for styled terminal creation —
// unless info.Here is true, in which case it launches in place.
// In plain terminals, it launches Claude Code directly.
// serenaActivationHint returns the one-line hint adb emits on launch/dispatch so
// a human/agent knows code-nav follows the ticket: this worktree is its own
// Serena project (instance-per-project), activated via the .serena/project.yml
// provisioned in #202. adb only emits the path + hint — it does not manage
// Serena. See docs/spikes/f8-serena-per-worktree-lsp.md (#212). Empty in ⇒
// empty out (a repo-less/worktree-less task gets no hint).
func serenaActivationHint(worktreePath string) string {
	if worktreePath == "" {
		return ""
	}
	return fmt.Sprintf("Serena: this worktree is its own project (instance-per-project); code-nav will activate %s via its .serena/project.yml.", worktreePath)
}

func launchWorkflow(info taskLaunchInfo) error {
	// Update terminal state
	if App != nil && App.TerminalStateWriter != nil {
		termState := integration.TerminalState{
			WorktreePath: info.WorktreePath,
			TaskID:       info.TaskID,
			Status:       "active",
			LastUpdated:  time.Now().UTC().Format(time.RFC3339),
		}
		if err := App.TerminalStateWriter.WriteState(termState); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update terminal state: %v\n", err)
		}
	}

	// VS Code: write launch file for the extension to pick up, unless the
	// caller (typically the extension itself) asked us to launch in place.
	if !shouldLaunchInPlace(info) {
		return launchViaVSCode(info)
	}

	// Plain terminal (or --here): rename tab and launch directly
	title := fmt.Sprintf("%s %s %s", info.TaskID, info.TaskType, info.Priority)
	fmt.Printf("\033]0;%s\007", title)

	fmt.Printf("Opening Claude Code in %s...\n", info.WorktreePath)
	// Emit the Serena activation hint so code-nav follows the ticket (#212). adb
	// only surfaces the path + hint — it does not manage Serena (instance-per-
	// project; the per-worktree .serena/project.yml from #202 does the work).
	if hint := serenaActivationHint(info.WorktreePath); hint != "" {
		fmt.Println(hint)
	}
	if err := launchClaudeCode(info.WorktreePath, info.Resume); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to launch Claude Code: %v\n", err)

		fmt.Println("\nDropping into interactive shell...")
		fmt.Printf("Working directory: %s\n", info.WorktreePath)
		fmt.Println("Type 'exit' to return to the main shell.")

		return launchInteractiveShell(info.TaskID, info.WorktreePath)
	}

	return nil
}

// launchViaVSCode writes a launch request for the VS Code extension
func launchViaVSCode(info taskLaunchInfo) error {
	info.Timestamp = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal launch info: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	launchFile := filepath.Join(homeDir, ".adb_terminal_launch.json")
	if err := os.WriteFile(launchFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write launch file: %w", err)
	}

	fmt.Printf("Launching styled terminal for %s %s %s...\n", info.TaskID, info.TaskType, info.Priority)
	return nil
}

// claudeProjectDir returns the directory Claude Code uses to store a project's
// conversation transcripts: ~/.claude/projects/<munged>, where <munged> is the
// absolute project path flattened into a single path component. Every path
// separator ('/' and, on Windows, '\'), '.', and the Windows drive ':' is
// replaced by '-'. Munging '\' and ':' keeps <munged> a VALID single component
// on Windows (a raw "C:\..." would otherwise nest dirs / carry an illegal ':');
// on POSIX worktree paths there is no '\' or ':' so the result is unchanged.
func claudeProjectDir(home, path string) string {
	munged := strings.NewReplacer("/", "-", `\`, "-", ".", "-", ":", "-").Replace(path)
	return filepath.Join(home, ".claude", "projects", munged)
}

// conversationExistsIn reports whether Claude Code has a prior conversation for
// the given project path — i.e. ~/.claude/projects/<munged>/ holds at least one
// *.jsonl transcript. `home` is injected so this is testable without touching
// the real home dir.
func conversationExistsIn(home, path string) bool {
	matches, err := filepath.Glob(filepath.Join(claudeProjectDir(home, path), "*.jsonl"))
	return err == nil && len(matches) > 0
}

// conversationExists is the production wrapper around conversationExistsIn,
// resolving the real home directory. Returns false on any error (treat an
// unknown home as "no prior conversation" → start fresh, never crash).
func conversationExists(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return conversationExistsIn(home, path)
}

// claudeArgs builds the argument list for launching Claude Code. `--continue`
// is appended ONLY when a resume was requested AND a prior conversation exists
// for the target directory. A freshly-created worktree has no prior
// conversation, so passing `--continue` there makes the real claude CLI exit 1
// ("No conversation found to continue") — this guard starts a NEW session
// instead of crashing/dropping to a bare shell.
func claudeArgs(resume, conversationExists bool) []string {
	args := []string{"--dangerously-skip-permissions"}
	if resume && conversationExists {
		args = append(args, "--continue")
	}
	return args
}

// launchClaudeCode launches Claude Code in the specified directory. When resume
// is requested it continues the prior conversation only if one exists for this
// directory; otherwise it starts a fresh session.
//
// To make the session survive a VS Code reload/quit/crash, claude is hosted
// inside a tmux session whose name is derived deterministically from `path`
// (see tmuxSessionName) — byte-identical to ~/.local/bin/cc-survive, so an
// adb-launched session and the "🌙 claude (tmux)" VS Code terminal profile
// converge on ONE session per folder and either can reattach to the other's
// live claude. The tmux server is an independent daemon, so claude outlives the
// VS Code pty-host that exec'd adb. We degrade gracefully to a direct (non-
// survivable) launch when tmux is unavailable or we're already inside a tmux
// session (nesting tmux is never wanted).
func launchClaudeCode(path string, resume bool) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	args := claudeArgs(resume, conversationExists(path))

	cmd := tmuxLaunchCommand(path, args)
	if cmd == nil {
		// No tmux, or already inside one — run claude directly (the original,
		// non-survivable path).
		cmd = exec.Command(claudePath, args...)
		cmd.Dir = path
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}

	return nil
}

// tmuxLaunchCommand builds the *exec.Cmd that hosts claude inside a survivable
// tmux session for the given working dir, or returns nil when tmux should NOT
// be used (see shouldUseTmux). It relies on the inherited environment: the
// tmux server, whether reused or cold-started here, captures adb's env (which
// carries the Claude/model env vars when adb is launched from a login shell), so
// claude authenticates the same as a direct launch.
//
// Honors the ADB_TMUX env gate (set by the VS Code extension from
// adb.tmux.enabled): "0" or "false" force a direct, non-survivable launch
// even when tmux is available. Unset defaults to enabled (existing behaviour).
func tmuxLaunchCommand(path string, claudeArgs []string) *exec.Cmd {
	enabled := tmuxEnabledFromEnv(os.Getenv("ADB_TMUX"))
	_, lookupErr := exec.LookPath("tmux")
	if lookupErr != nil || !shouldUseTmux(enabled, true, os.Getenv("TMUX") != "", runtime.GOOS) {
		return nil
	}
	return exec.Command("tmux", tmuxArgs(tmuxSessionName(path), path, claudeArgs)...)
}

// shouldUseTmux decides whether to host claude inside tmux. False when tmux
// hosting is disabled by config (ADB_TMUX=0), when tmux is unavailable, when
// we're already inside a tmux session (TMUX set — nesting is never wanted), or
// when goos == "windows": MSYS tmux denies claude a real console pty, so claude
// falls into --print mode and exits 1 ("Input must be provided…"). On Windows we
// run claude directly against the console instead. Pure for testability.
func shouldUseTmux(enabled, tmuxOnPath, insideTmux bool, goos string) bool {
	return enabled && tmuxOnPath && !insideTmux && goos != "windows"
}

// tmuxEnabledFromEnv reads the ADB_TMUX gate (set by the VS Code extension
// from adb.tmux.enabled). Unset → true (default enabled, matches historical
// behaviour). "0" or "false" → false. Any other value → true (fail toward
// the durable path so a stray/unknown value doesn't silently break tmux).
func tmuxEnabledFromEnv(v string) bool {
	return v != "0" && v != "false"
}

// tmuxSessionName derives the deterministic tmux session name for a working
// directory using the configurable prefix (ADB_TMUX_PREFIX, from
// adb.tmux.sessionPrefix; empty → "cc-"). Byte-identical to
// ~/.local/bin/cc-survive when the default prefix is used. This determinism
// is what makes auto-reattach work: the same folder always maps to the same
// session, so adb and the "🌙 claude (tmux)" profile never fork a second one.
func tmuxSessionName(path string) string {
	return tmuxSessionNameWithPrefix(os.Getenv("ADB_TMUX_PREFIX"), path)
}

// tmuxSessionNameWithPrefix builds <prefix><sanitized-basename>, running
// prefix + basename through a SINGLE sanitizing loop so a hostile prefix
// (e.g. "a.b:") cannot inject argv metacharacters and dash-runs collapse
// across the prefix/basename boundary. Sanitize replaces every byte outside
// [A-Za-z0-9_-] with '-', collapses runs to a single '-', and trims
// leading/trailing '-'. Empty prefix falls back to "cc-" (default).
// Pure for testability.
func tmuxSessionNameWithPrefix(prefix, path string) string {
	if prefix == "" {
		prefix = "cc-"
	}
	raw := prefix + filepath.Base(path)
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
			b.WriteByte(c)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(collapseDashes(b.String()), "-")
}

// collapseDashes replaces every run of consecutive '-' with a single '-',
// matching cc-survive's `sed 's/-\{2,\}/-/g'`.
func collapseDashes(s string) string {
	var b strings.Builder
	prevDash := false
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			if prevDash {
				continue
			}
			prevDash = true
		} else {
			prevDash = false
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// tmuxArgs builds the tmux argv that hosts claude in a survivable session:
//
//	tmux new-session -A -D -s <name> -c <dir> <inner>
//
// -A makes new-session attach to an existing session of that name instead of
// erroring (idempotent attach-or-create); -D detaches any stale client on
// attach (e.g. a ghost left by a VS Code crash), mirroring cc-survive's
// `attach -d`. On a fresh create tmux runs <inner>; on reattach <inner> is
// ignored and you reconnect to the still-running claude — so resume's
// --continue is applied exactly once, at first launch. <inner> drops to a login
// shell after claude exits so the tmux window (and the VS Code tab) stays
// usable, matching cc-survive.
func tmuxArgs(name, dir string, claudeArgs []string) []string {
	inner := "claude " + strings.Join(claudeArgs, " ") + `; exec "${SHELL:-/bin/bash}" -l`
	return []string{"new-session", "-A", "-D", "-s", name, "-c", dir, inner}
}

// interactiveShell picks the shell for the drop-to-shell fallback. On Windows,
// $SHELL is usually unset (or points at a POSIX shell that may be absent), so
// prefer $ComSpec (cmd.exe); elsewhere use $SHELL, falling back to /bin/bash.
// Pure for testability.
func interactiveShell(shellEnv, comspec, goos string) string {
	if goos == "windows" {
		if comspec != "" {
			return comspec
		}
		return "cmd.exe"
	}
	if shellEnv != "" {
		return shellEnv
	}
	return "/bin/bash"
}

// launchInteractiveShell launches an interactive shell in the specified directory
func launchInteractiveShell(taskID, path string) error {
	shell := interactiveShell(os.Getenv("SHELL"), os.Getenv("ComSpec"), runtime.GOOS)

	cmd := exec.Command(shell)
	cmd.Dir = path
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	env = append(env, fmt.Sprintf("ADB_TASK_ID=%s", taskID))
	env = append(env, fmt.Sprintf("ADB_WORKTREE_PATH=%s", path))
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shell exited with error: %w", err)
	}

	return nil
}
