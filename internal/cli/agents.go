package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The coding agents adb can launch, as DATA.
//
// Adding an agent used to mean five scattered edits — an entry in validAgents, a
// case in agentDisplayName, an args builder, a prior-session probe, and a branch
// in each of launchAgent and priorSessionExists. Five places is how an agent ends
// up half-wired: accepted by `--agent`, then launching claude, which looks like
// it worked. With a descriptor, adding one is a single registry entry and every
// dispatch reads it.
//
// adb manages tickets, worktrees, and written-down context; the model, the
// harness rules, and the session store belong to whichever agent you launch. So a
// descriptor carries only what adb genuinely needs to know: what to exec, what to
// call it, how to ask for a resume, where its transcripts live, and which tmux
// namespace it owns.

// codingAgent describes one launchable coding agent.
type codingAgent struct {
	// Name is the `--agent` value and the registry key.
	Name string
	// Binary is looked up on PATH at launch time. adb never installs an agent —
	// a missing binary is a clear error, the same honest gate tmux/gitleaks/gh get.
	Binary string
	// DisplayName is what launch messages call it.
	DisplayName string
	// Args builds the full argument list. It takes both `resume` (what the user
	// asked for) and `priorSession` (what actually exists), because every agent
	// here errors or misbehaves when told to continue a conversation that is not
	// there — claude exits 1 with "No conversation found to continue".
	//
	// It returns the WHOLE argv after the binary, not just trailing flags, since
	// some agents put the verb first (`codex resume --last`, `q chat`).
	Args func(resume, priorSession bool) []string
	// HasPriorSession reports whether this agent already has a conversation for
	// a directory. home is injected so it is testable without a real home dir;
	// an empty/unknown home means "no prior session" — start fresh, never crash.
	HasPriorSession func(home, path string) bool
	// TmuxPrefix namespaces the tmux session name, so two agents in one worktree
	// never collide on a single session.
	TmuxPrefix string
	// Verified records whether adb's wiring for this agent has been driven
	// against the real CLI. It is documentation, not behaviour: an unverified
	// agent still launches, and this is what stops the docs from implying more
	// confidence than was earned.
	Verified bool
}

// agentRegistry is the set of launchable agents, keyed by `--agent` value.
var agentRegistry = map[string]codingAgent{
	"claude": {
		Name:        "claude",
		Binary:      "claude",
		DisplayName: "Claude Code",
		// --dangerously-skip-permissions is adb's long-standing default for a
		// task worktree; --continue only when a transcript actually exists.
		Args: func(resume, priorSession bool) []string {
			args := []string{"--dangerously-skip-permissions"}
			if resume && priorSession {
				args = append(args, "--continue")
			}
			return args
		},
		HasPriorSession: conversationExistsIn,
		TmuxPrefix:      "cc-",
		Verified:        true,
	},

	"pi": {
		Name:        "pi",
		Binary:      "pi",
		DisplayName: "pi",
		// pi has no --dangerously-skip-permissions equivalent — its own
		// project-trust model governs approvals — so no blanket flag is passed.
		Args: func(resume, priorSession bool) []string {
			if resume && priorSession {
				return []string{"--continue"}
			}
			return nil
		},
		HasPriorSession: piConversationExistsIn,
		TmuxPrefix:      "pi-",
		Verified:        true,
	},

	"codex": {
		Name:        "codex",
		Binary:      "codex",
		DisplayName: "Codex CLI",
		// Bare `codex` opens the interactive CLI. Resume is a SUBCOMMAND, and
		// `--last` is already scoped to the current directory: Codex's own
		// `--all` flag is documented as "disables cwd filtering", so filtering is
		// the default and a resume cannot wander into another worktree's
		// conversation. Verified from `codex resume --help` on a real install.
		Args: func(resume, priorSession bool) []string {
			if resume && priorSession {
				return []string{"resume", "--last"}
			}
			return nil
		},
		HasPriorSession: codexSessionExistsIn,
		TmuxPrefix:      "cx-",
		Verified:        true,
	},

	"amazon-q": {
		Name:        "amazon-q",
		Binary:      "q",
		DisplayName: "Amazon Q Developer CLI",
		// `q chat` is the interactive entry point, so the subcommand is part of
		// the argv either way.
		//
		// UNVERIFIED: the `q` CLI is not installed on the machine this was
		// written on, so the argv and the transcript location come from Amazon Q's
		// documentation rather than from driving it. The failure mode is honest —
		// a missing binary gives "q CLI not found in PATH", and a wrong resume
		// flag surfaces as q's own usage error rather than as adb misbehaving.
		Args: func(resume, priorSession bool) []string {
			args := []string{"chat"}
			if resume && priorSession {
				args = append(args, "--resume")
			}
			return args
		},
		HasPriorSession: amazonQSessionExistsIn,
		TmuxPrefix:      "q-",
		Verified:        false,
	},
}

// validAgentNames returns the registered agent names, sorted. This is what
// `--agent` validates against and what its error message lists — derived from the
// registry so the two cannot disagree.
func validAgentNames() []string {
	names := make([]string, 0, len(agentRegistry))
	for name := range agentRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// agentFlagUsage is the --agent flag's usage string, built from the registry.
//
// It was hardcoded to "claude or pi" in three commands. A usage string is
// documentation that ships inside the binary, so the moment an agent was
// registered it started lying — and nothing caught it, because no test read the
// flag's help. Derived, it cannot go stale.
func agentFlagUsage() string {
	return "Coding agent to launch: " + strings.Join(validAgentNames(), ", ") +
		" (default: ADB_AGENT, else config launch_agent, else claude)"
}

// lookupAgent returns the descriptor for a name.
func lookupAgent(name string) (codingAgent, bool) {
	agent, ok := agentRegistry[name]
	return agent, ok
}

// codexSessionExistsIn reports whether Codex CLI has a prior session for a
// directory.
//
// Codex stores transcripts DATE-partitioned — ~/.codex/sessions/YYYY/MM/DD/
// rollout-<timestamp>-<uuid>.jsonl — rather than keyed by project path the way
// Claude Code does, so there is no directory to glob for. Each rollout's first
// line records the cwd it ran in, so the probe reads that.
//
// Bounded on purpose: rollouts are walked newest-first (the timestamp is in the
// filename, so lexical order is chronological) and the scan stops at the first
// match or after codexProbeLimit files. This decides ONE printed sentence —
// whether `adb task resume` says it continued or started fresh — and is not worth
// reading thousands of files for.
func codexSessionExistsIn(home, path string) bool {
	if home == "" || path == "" {
		return false
	}
	matches, err := filepath.Glob(
		filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*.jsonl"),
	)
	if err != nil || len(matches) == 0 {
		return false
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	if len(matches) > codexProbeLimit {
		matches = matches[:codexProbeLimit]
	}

	want := filepath.Clean(path)
	for _, file := range matches {
		if codexRolloutCwd(file) == want {
			return true
		}
	}
	return false
}

// codexProbeLimit caps how many rollout files the Codex probe reads.
const codexProbeLimit = 200

// codexRolloutCwd reads the cwd recorded in a rollout file's first line,
// returning "" for anything it cannot parse. Every failure mode here — junk, an
// empty file, a non-string cwd — means "not a match", never a crash.
func codexRolloutCwd(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	var record struct {
		Payload struct {
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return ""
	}
	if record.Payload.Cwd == "" {
		return ""
	}
	return filepath.Clean(record.Payload.Cwd)
}

// amazonQSessionExistsIn reports whether Amazon Q has a prior conversation for a
// directory.
//
// UNVERIFIED, like the rest of the amazon-q descriptor: `q` is not installed
// here, so this checks the documented conversation store
// (~/.aws/amazonq/) for any transcript naming the directory. When it is wrong the
// consequence is bounded — adb says "starting a new one" and `q chat` runs
// without --resume, which is the safe direction.
func amazonQSessionExistsIn(home, path string) bool {
	if home == "" || path == "" {
		return false
	}
	matches, err := filepath.Glob(filepath.Join(home, ".aws", "amazonq", "history", "*.json"))
	if err != nil || len(matches) == 0 {
		return false
	}
	want := filepath.Clean(path)
	for _, file := range matches {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), want) {
			return true
		}
	}
	return false
}
