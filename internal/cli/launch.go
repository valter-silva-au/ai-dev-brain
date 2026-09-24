package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
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
	// Agent is the coding-agent CLI to launch: "claude" (default) or "pi".
	// Empty means claude, so existing launch files and callers keep working.
	Agent     string `json:"agent,omitempty"`
	Timestamp string `json:"timestamp"`
}

// launchWorkflow launches the coding agent for a task, in the caller's terminal.
//
// There is exactly one launch mode. adb used to bounce through a launch file that
// a VS Code extension picked up; the extension is gone (adb is terminal-native),
// so launching in place is unconditional and `--here` had nothing left to bypass.
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

// Coding-agent CLIs the launch workflow knows how to host. claude is the
// default so existing workflows, launch files and tests are unaffected; pi
// (@earendil-works/pi-coding-agent) is the second first-class option.
// validAgents is derived from the agent registry (see agents.go) so the accepted
// set and the descriptors cannot disagree.
var validAgents = validAgentNames()

// validAgent reports whether s is a launchable agent name.
func validAgent(s string) bool {
	for _, a := range validAgents {
		if a == s {
			return true
		}
	}
	return false
}

// resolveAgent picks the agent for a launch: the --agent flag wins, then the
// ADB_AGENT env gate (the same pattern as ADB_TMUX / ADB_NO_LAUNCH), then the
// layered config's `launch_agent` custom setting (repo .taskrc > org config >
// ~/.taskconfig), then the historical default "claude". Unknown values are
// rejected up front so a typo fails with a clear message instead of falling
// back silently.
// validateAgentFlag rejects an invalid --agent value UP FRONT, before any work.
//
// The bug it closes: resolveAgent was only called inside each command's launch
// block, and a repo-less task never reaches that block (no worktree → nothing to
// launch). So `adb task create "x" --agent gpt5` created the task, printed a
// checkmark, exited 0, and silently ignored the flag. A typo earning a success is
// the same defect class as `--filter Done` answering "No tasks found".
//
// It resolves through the same precedence chain the launch path uses, so a bad
// value in ADB_AGENT or in config is rejected here too rather than at launch.
func validateAgentFlag(flagValue string) error {
	_, err := resolveAgent(flagValue)
	return err
}

func resolveAgent(flagValue string) (string, error) {
	return resolveAgentWithConfig(flagValue, os.Getenv("ADB_AGENT"), configuredAgent())
}

// resolveAgentWithConfig is the pure agent decision. Precedence, most specific
// first: flag → env → config → "claude".
func resolveAgentWithConfig(flagValue, envValue, configValue string) (string, error) {
	agent := flagValue
	if agent == "" {
		agent = envValue
	}
	if agent == "" {
		agent = configValue
	}
	if agent == "" {
		agent = "claude"
	}
	if !validAgent(agent) {
		return "", fmt.Errorf("invalid agent: %s (must be one of %s)", agent, strings.Join(validAgentNames(), ", "))
	}
	return agent, nil
}

// configuredAgent reads the `launch_agent` custom setting from the layered
// config (repo > org > global), e.g. in ~/.taskconfig:
//
//	custom_settings:
//	  launch_agent: pi
//
// Empty (the default) when App/MergedConfig are unavailable or the key is not
// set anywhere — callers then fall through to the claude default.
func configuredAgent() string {
	if App == nil || App.MergedConfig == nil {
		return ""
	}
	v, _, ok := App.MergedConfig.SettingSource("launch_agent")
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// agentDisplayName returns the human-facing name used in launch messages.
//
// An unknown agent — including the empty value carried by pre-agent launch files
// — reads as Claude Code, which is the historical default.
func agentDisplayName(agent string) string {
	if described, ok := lookupAgent(agent); ok {
		return described.DisplayName
	}
	return agentRegistry["claude"].DisplayName
}

// agentLauncher and interactiveShell are the two spawn points of the launch
// workflow, held as vars so a test can drive launchWorkflow without starting a
// real agent or blocking on a real shell. This is the same swap idiom the
// removed run-with-ruflo Commander used.
var (
	agentLauncher       = launchAgent
	shellFallbackLaunch = launchInteractiveShell
)

func swapAgentLauncher(fake func(agent, path string, resume bool) error) func() {
	previous := agentLauncher
	agentLauncher = fake
	return func() { agentLauncher = previous }
}

func swapInteractiveShell(fake func(taskID, path string) error) func() {
	previous := shellFallbackLaunch
	shellFallbackLaunch = fake
	return func() { shellFallbackLaunch = previous }
}

func launchWorkflow(info taskLaunchInfo) error {
	// Rename the terminal tab, then launch in this terminal.
	title := fmt.Sprintf("%s %s %s", info.TaskID, info.TaskType, info.Priority)
	fmt.Printf("\033]0;%s\007", title)

	fmt.Printf("Opening %s in %s...\n", agentDisplayName(info.Agent), info.WorktreePath)
	// Emit the Serena activation hint so code-nav follows the ticket (#212). adb
	// only surfaces the path + hint — it does not manage Serena (instance-per-
	// project; the per-worktree .serena/project.yml from #202 does the work).
	if hint := serenaActivationHint(info.WorktreePath); hint != "" {
		fmt.Println(hint)
	}

	// This is the one seam every `adb task create`/`start`/`resume` launch goes
	// through, so it is where an agent session demonstrably begins and ends.
	// Before TASK-00039 the only producer of these events was the ruflo-specific
	// `task run-with-ruflo`; removing it would have left `adb events digest` and
	// `metrics.AgentSessions` permanently empty while the schema still claimed
	// they were emitted.
	emitAgentSessionEvent(observability.EventAgentSessionStarted, info, nil)
	launchErr := agentLauncher(info.Agent, info.WorktreePath, info.Resume)
	// Emitted for a failed launch too: a session that could not start is exactly
	// the case worth finding in the log later.
	emitAgentSessionEvent(observability.EventAgentSessionEnded, info, launchErr)

	if launchErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to launch %s: %v\n", agentDisplayName(info.Agent), launchErr)

		fmt.Println("\nDropping into interactive shell...")
		fmt.Printf("Working directory: %s\n", info.WorktreePath)
		fmt.Println("Type 'exit' to return to the main shell.")

		return shellFallbackLaunch(info.TaskID, info.WorktreePath)
	}

	return nil
}

// emitAgentSessionEvent records an agent session boundary. The payload keys are
// the ones `internal/observability/schema.go` documents and that
// `metrics.AgentSessions` and `adb events digest` read, kept byte-identical to
// what run-with-ruflo wrote so an existing event log stays readable.
//
// Failures are swallowed: observability must never be the reason a launch fails.
func emitAgentSessionEvent(
	eventType observability.EventType,
	info taskLaunchInfo,
	launchErr error,
) {
	if App == nil || App.EventLog == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":  info.TaskID,
		"worktree": info.WorktreePath,
		"bin":      resolvedAgentName(info.Agent),
	}
	if launchErr != nil {
		data["error"] = launchErr.Error()
	}
	App.EventLog.Log(eventType, data)
}

// resolvedAgentName reports the agent actually launched. An empty Agent means
// claude (see taskLaunchInfo), and recording "" would make the event useless
// for telling the two launchers apart.
func resolvedAgentName(agent string) string {
	if agent == "" {
		return "claude"
	}
	return agent
}

// piSessionDir returns the directory pi uses to store a project's session
// transcripts: ~/.pi/agent/sessions/--<munged>--, where <munged> is the absolute
// cwd with any leading '/' or '\\' stripped and every '/', '\\' and ':' replaced
// by '-', wrapped in leading/trailing '--'. This mirrors pi's own
// getDefaultSessionDirPath (session-manager.js) byte for byte, so adb's
// prior-session probe looks in exactly the directory pi writes. `home` is
// injected so this is testable without touching the real home dir.
func piSessionDir(home, path string) string {
	abs := path
	if resolved, err := filepath.Abs(path); err == nil {
		abs = resolved
	}
	return filepath.Join(home, ".pi", "agent", "sessions", "--"+piSessionDirName(abs)+"--")
}

// piSessionDirName is the pure munge pi applies to an (already absolute) cwd:
// strip any leading '/' or '\\', then replace every '/', '\\' and ':' with '-'.
// Note pi does NOT munge '.', unlike Claude Code's project-dir scheme.
func piSessionDirName(abs string) string {
	munged := strings.TrimLeft(abs, "/\\")
	return strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(munged)
}

// piConversationExistsIn reports whether pi has a prior session for the given
// project path — i.e. the project's session dir holds at least one *.jsonl
// transcript. The dir name encodes the cwd, so any transcript in it belongs to
// this path (pi's own discovery additionally re-checks the recorded cwd header;
// the directory scoping alone is a sufficient proxy for "a session exists").
func piConversationExistsIn(home, path string) bool {
	matches, err := filepath.Glob(filepath.Join(piSessionDir(home, path), "*.jsonl"))
	return err == nil && len(matches) > 0
}

// piArgs builds the argument list for launching pi. `--continue` is appended
// ONLY when a resume was requested AND a prior session exists for the target
// directory (pi's `-c`/`--continue` continues the most recent session for the
// cwd). A freshly-created worktree has no session dir, so the guard starts a
// NEW session instead of pi erroring or silently resuming the wrong project's
// session. pi has no --dangerously-skip-permissions equivalent — its own
// project-trust model governs approvals — so no blanket flag is passed.
func piArgs(resume, conversationExists bool) []string {
	if resume && conversationExists {
		return []string{"--continue"}
	}
	return nil
}

// launchAgent launches the requested coding-agent CLI in the specified
// directory, entirely from its registry descriptor. Unknown agents fall back to
// claude (resolveAgent validates up front, so this only guards direct calls).
//
// One path for every agent is the point: an agent cannot be half-wired into
// `--agent` while actually launching something else.
func launchAgent(agent, path string, resume bool) error {
	described, ok := lookupAgent(agent)
	if !ok {
		described = agentRegistry["claude"]
	}
	return launchHostedAgent(
		described.Binary,
		path,
		described.Args(resume, priorSessionExists(described.Name, path)),
		tmuxSessionNameWithPrefix(agentTmuxPrefix(described), path),
	)
}

// agentTmuxPrefix returns the tmux session-name prefix for an agent: the
// ADB_TMUX_PREFIX override when set (an operator who renamed one namespace
// probably wants the same control everywhere), else the descriptor's own.
func agentTmuxPrefix(described codingAgent) string {
	if p := os.Getenv("ADB_TMUX_PREFIX"); p != "" {
		return p
	}
	return described.TmuxPrefix
}

// priorSessionExists reports whether the given agent already has a conversation
// for this directory. Each agent stores its transcripts in its own place, so
// this dispatches per agent rather than probing one well-known path.
//
// It exists so `adb task resume` can SAY which of the two things it did.
// Continuing a prior session and starting a fresh one are very different
// situations for whoever is about to type into it, and the launchers pick
// between them silently.
func priorSessionExists(agent, path string) bool {
	if path == "" {
		return false
	}
	described, ok := lookupAgent(agent)
	if !ok {
		described = agentRegistry["claude"]
	}
	if described.HasPriorSession == nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// An unknown home is "no prior session" → start fresh, never crash.
		return false
	}
	return described.HasPriorSession(home, path)
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

// piTmuxPrefix returns the tmux session-name prefix for pi-hosted sessions:
// the ADB_TMUX_PREFIX override when set (operators who renamed the claude
// namespace probably want the same control here), else the "pi-" default.
func piTmuxPrefix() string {
	if p := os.Getenv("ADB_TMUX_PREFIX"); p != "" {
		return p
	}
	return "pi-"
}

// launchHostedAgent is the shared hosting path for every agent: look the
// binary up on PATH (clear error if missing), then host it in a survivable
// tmux session when available, degrading to a direct exec otherwise.
func launchHostedAgent(binary, path string, args []string, sessionName string) error {
	agentPath, err := exec.LookPath(binary)
	if err != nil {
		return fmt.Errorf("%s CLI not found in PATH: %w", binary, err)
	}

	cmd := tmuxLaunchCommand(path, sessionName, binary, args)
	if cmd == nil {
		// No tmux, or already inside one — run the agent directly (the original,
		// non-survivable path).
		cmd = exec.Command(agentPath, args...)
		cmd.Dir = path
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited with error: %w", binary, err)
	}

	return nil
}

// tmuxLaunchCommand builds the *exec.Cmd that hosts claude inside a survivable
// tmux session for the given working dir, or returns nil when tmux should NOT
// be used (see shouldUseTmux). It relies on the inherited environment: the
// tmux server, whether reused or cold-started here, captures adb's env (which
// carries the Bedrock/Claude vars when adb is launched from a login shell), so
// claude authenticates the same as a direct launch.
//
// Honors the ADB_TMUX env gate (set by the VS Code extension from
// adb.tmux.enabled): "0" or "false" force a direct, non-survivable launch
// even when tmux is available. Unset defaults to enabled (existing behaviour).
func tmuxLaunchCommand(path, sessionName, binary string, agentArgs []string) *exec.Cmd {
	enabled := tmuxEnabledFromEnv(os.Getenv("ADB_TMUX"))
	_, lookupErr := exec.LookPath("tmux")
	if lookupErr != nil || !shouldUseTmux(enabled, true, os.Getenv("TMUX") != "", runtime.GOOS) {
		return nil
	}
	// gosec sees a tainted argv. Every part of it is adb-constructed: the program
	// is the literal "tmux"; `binary` and `agentArgs` come from the agent registry
	// (a closed set that resolveAgent validates before anything launches);
	// `sessionName` has been through tmuxSessionNameWithPrefix's [A-Za-z0-9_-]
	// sanitizer, which is exactly what keeps the shell-interpreted inner command
	// in tmuxArgs safe; and `path` is the workspace's own worktree path, passed as
	// its own -c argv element rather than interpolated.
	//nolint:gosec // argv is registry-derived; sessionName is sanitized to [A-Za-z0-9_-]
	return exec.Command("tmux", tmuxArgs(sessionName, path, binary, agentArgs)...)
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
func tmuxArgs(name, dir, binary string, agentArgs []string) []string {
	inner := binary + " " + strings.Join(agentArgs, " ") + `; exec "${SHELL:-/bin/bash}" -l`
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

	// The "tainted" command is the invoking user's own login shell, taken from
	// their own $SHELL/$ComSpec and run with their own privileges — the whole
	// point of the drop-to-shell fallback. No arguments are passed and no shell
	// interpretation is added, so there is no boundary here to inject across.
	cmd := exec.Command(shell) //nolint:gosec // the caller's own $SHELL, run as the caller, with no args
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
