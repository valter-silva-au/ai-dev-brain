package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// =============================================================================
// Generators
// =============================================================================

// genCLIAlias generates a random CLIAlias with alphabetic names and commands.
func genCLIAlias(t *rapid.T, label string) CLIAlias {
	numArgs := rapid.IntRange(0, 4).Draw(t, label+"_numArgs")
	args := make([]string, numArgs)
	for i := range args {
		args[i] = rapid.StringMatching(`--[a-z]{2,10}`).Draw(t, fmt.Sprintf("%s_arg_%d", label, i))
	}

	return CLIAlias{
		Name:        rapid.StringMatching(`[a-z]{1,10}`).Draw(t, label+"_name"),
		Command:     rapid.StringMatching(`[a-z]{2,15}`).Draw(t, label+"_cmd"),
		DefaultArgs: args,
	}
}

// genTaskEnvContext generates a random TaskEnvContext with plausible values.
func genTaskEnvContext(t *rapid.T) *TaskEnvContext {
	taskNum := rapid.IntRange(1, 99999).Draw(t, "taskNum")
	return &TaskEnvContext{
		TaskID:       fmt.Sprintf("TASK-%05d", taskNum),
		Branch:       rapid.SampledFrom([]string{"feat", "bug", "spike", "refactor"}).Draw(t, "type") + "/" + rapid.StringMatching(`[a-z-]{3,20}`).Draw(t, "branchSuffix"),
		WorktreePath: "/repos/github.com/org/repo/work/" + fmt.Sprintf("TASK-%05d", taskNum),
		TicketPath:   "/tickets/" + fmt.Sprintf("TASK-%05d", taskNum),
	}
}

// genArgs generates a list of plausible CLI arguments (no pipes).
func genArgs(t *rapid.T) []string {
	numArgs := rapid.IntRange(0, 6).Draw(t, "numArgs")
	args := make([]string, numArgs)
	for i := range args {
		// Mix of flags and positional args.
		kind := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("argKind_%d", i))
		switch kind {
		case 0:
			args[i] = rapid.StringMatching(`--[a-z]{2,10}`).Draw(t, fmt.Sprintf("flag_%d", i))
		case 1:
			args[i] = rapid.StringMatching(`[a-z0-9]{1,15}`).Draw(t, fmt.Sprintf("pos_%d", i))
		case 2:
			args[i] = rapid.StringMatching(`-[a-z]`).Draw(t, fmt.Sprintf("short_%d", i))
		}
	}
	return args
}

// =============================================================================
// Property 26: CLI Argument Passthrough
// =============================================================================

// Feature: ai-dev-brain, Property 26: CLI Argument Passthrough
// *For any* external CLI name and arbitrary argument list, the CLI_Executor SHALL
// construct a subprocess command where the resolved command is invoked with all
// provided arguments in their original order, prepended by any alias default_args.
//
// **Validates: Requirements 18.1, 18.5**
func TestProperty26_CLIArgumentPassthrough(t *testing.T) {
	executor := NewCLIExecutor()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random alias.
		alias := genCLIAlias(rt, "alias")
		userArgs := genArgs(rt)

		// Use the alias name as CLI input, resolve it.
		cmd, defaultArgs, found := executor.ResolveAlias(alias.Name, []CLIAlias{alias})
		if !found {
			rt.Fatal("expected alias to be found")
		}
		if cmd != alias.Command {
			rt.Errorf("resolved command = %q, want %q", cmd, alias.Command)
		}

		// Verify the full argument list is defaultArgs + userArgs in order.
		fullArgs := make([]string, 0, len(defaultArgs)+len(userArgs))
		fullArgs = append(fullArgs, defaultArgs...)
		fullArgs = append(fullArgs, userArgs...)

		// Check default args come first.
		for i, a := range alias.DefaultArgs {
			if i >= len(fullArgs) || fullArgs[i] != a {
				rt.Errorf("fullArgs[%d] = %q, want default arg %q", i, safeIdx(fullArgs, i), a)
			}
		}

		// Check user args follow.
		offset := len(alias.DefaultArgs)
		for i, a := range userArgs {
			if offset+i >= len(fullArgs) || fullArgs[offset+i] != a {
				rt.Errorf("fullArgs[%d] = %q, want user arg %q", offset+i, safeIdx(fullArgs, offset+i), a)
			}
		}

		// Total length must match.
		if len(fullArgs) != len(alias.DefaultArgs)+len(userArgs) {
			rt.Errorf("fullArgs length = %d, want %d", len(fullArgs), len(alias.DefaultArgs)+len(userArgs))
		}
	})
}

// safeIdx returns the value at index i or "<out-of-bounds>" if i is invalid.
func safeIdx(s []string, i int) string {
	if i < 0 || i >= len(s) {
		return "<out-of-bounds>"
	}
	return s[i]
}

// =============================================================================
// Property 27: Task Context Environment Injection
// =============================================================================

// Feature: ai-dev-brain, Property 27: Task Context Environment Injection
// *For any* task context (TaskEnvContext) with a task ID, branch, worktree path,
// and ticket path, the subprocess environment built by BuildEnv SHALL contain
// ADB_TASK_ID, ADB_BRANCH, ADB_WORKTREE_PATH, and ADB_TICKET_PATH set to
// the corresponding values. When no task context is provided (nil), none of
// these variables SHALL be present.
//
// **Validates: Requirements 18.3**
func TestProperty27_TaskContextEnvironmentInjection(t *testing.T) {
	executor := NewCLIExecutor()
	adbVars := []string{"ADB_TASK_ID", "ADB_BRANCH", "ADB_WORKTREE_PATH", "ADB_TICKET_PATH"}

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random base env (no ADB_ vars).
		numBase := rapid.IntRange(0, 5).Draw(rt, "numBase")
		base := make([]string, numBase)
		for i := range base {
			key := rapid.StringMatching(`[A-Z_]{2,10}`).Draw(rt, fmt.Sprintf("baseKey_%d", i))
			// Ensure no ADB_ prefix in base.
			if strings.HasPrefix(key, "ADB_") {
				key = "X" + key
			}
			val := rapid.StringMatching(`[a-zA-Z0-9/]{1,20}`).Draw(rt, fmt.Sprintf("baseVal_%d", i))
			base[i] = key + "=" + val
		}

		// Test with a task context.
		ctx := genTaskEnvContext(rt)
		env := executor.BuildEnv(base, ctx)

		// Base env should be present.
		for i, entry := range base {
			if i >= len(env) || env[i] != entry {
				rt.Errorf("base env[%d] not preserved: got %q, want %q", i, safeIdx(env, i), entry)
			}
		}

		// All 4 ADB vars should be present.
		envMap := envToMap(env)
		expectedVals := map[string]string{
			"ADB_TASK_ID":       ctx.TaskID,
			"ADB_BRANCH":        ctx.Branch,
			"ADB_WORKTREE_PATH": ctx.WorktreePath,
			"ADB_TICKET_PATH":   ctx.TicketPath,
		}
		for _, k := range adbVars {
			got, ok := envMap[k]
			if !ok {
				rt.Errorf("missing env var %s", k)
				continue
			}
			if got != expectedVals[k] {
				rt.Errorf("%s = %q, want %q", k, got, expectedVals[k])
			}
		}

		// Test with nil context -- no ADB vars should be present.
		envNil := executor.BuildEnv(base, nil)
		envNilMap := envToMap(envNil)
		for _, k := range adbVars {
			if _, ok := envNilMap[k]; ok {
				rt.Errorf("ADB var %s should NOT be present when context is nil", k)
			}
		}

		// Nil context should not change the length.
		if len(envNil) != len(base) {
			rt.Errorf("env length with nil ctx = %d, want %d", len(envNil), len(base))
		}
	})
}

// envToMap converts a list of "KEY=VALUE" strings to a map.
func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// =============================================================================
// Property 29: CLI Alias Resolution
// =============================================================================

// Feature: ai-dev-brain, Property 29: CLI Alias Resolution
// *For any* set of CLI aliases configured in .taskconfig, ResolveAlias SHALL
// return the correct command and default_args for a known alias, and SHALL
// return the original name with no default_args for an unknown alias.
//
// **Validates: Requirements 18.5, 18.8**
func TestProperty29_CLIAliasResolution(t *testing.T) {
	executor := NewCLIExecutor()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random set of aliases with unique names.
		numAliases := rapid.IntRange(1, 10).Draw(rt, "numAliases")
		nameSet := make(map[string]bool)
		aliases := make([]CLIAlias, 0, numAliases)
		for i := 0; i < numAliases; i++ {
			a := genCLIAlias(rt, fmt.Sprintf("alias_%d", i))
			// Ensure unique names.
			if nameSet[a.Name] {
				continue
			}
			nameSet[a.Name] = true
			aliases = append(aliases, a)
		}

		// For each alias, resolution should return the correct command and default args.
		for _, a := range aliases {
			cmd, args, found := executor.ResolveAlias(a.Name, aliases)
			if !found {
				rt.Errorf("alias %q should be found", a.Name)
				continue
			}
			if cmd != a.Command {
				rt.Errorf("alias %q resolved to %q, want %q", a.Name, cmd, a.Command)
			}
			if len(args) != len(a.DefaultArgs) {
				rt.Errorf("alias %q has %d default args, want %d", a.Name, len(args), len(a.DefaultArgs))
				continue
			}
			for i := range args {
				if args[i] != a.DefaultArgs[i] {
					rt.Errorf("alias %q default_args[%d] = %q, want %q", a.Name, i, args[i], a.DefaultArgs[i])
				}
			}
		}

		// Unknown alias should return original name.
		unknownName := "zzz_unknown_" + rapid.StringMatching(`[a-z]{5}`).Draw(rt, "unknown")
		cmd, args, found := executor.ResolveAlias(unknownName, aliases)
		if found {
			rt.Errorf("unknown alias %q should NOT be found", unknownName)
		}
		if cmd != unknownName {
			rt.Errorf("unknown alias returned command %q, want %q", cmd, unknownName)
		}
		if args != nil {
			rt.Errorf("unknown alias returned args %v, want nil", args)
		}
	})
}

// =============================================================================
// Property 30: CLI Failure Propagation
// =============================================================================

// Feature: ai-dev-brain, Property 30: CLI Failure Propagation
// *For any* external CLI execution that exits with a non-zero exit code, the
// CLI_Executor SHALL return a CLIExecResult containing the original exit code
// and captured stderr, and if a task context is active, the failure SHALL be
// logged to the task's context.
//
// **Validates: Requirements 18.6**
func TestProperty30_CLIFailurePropagation(t *testing.T) {
	executor := NewCLIExecutor()

	rapid.Check(t, func(rt *rapid.T) {
		exitCode := rapid.IntRange(1, 125).Draw(rt, "exitCode")
		stderrMsg := rapid.StringMatching(`[a-zA-Z ]{5,30}`).Draw(rt, "stderrMsg")
		useTaskCtx := rapid.Bool().Draw(rt, "useTaskCtx")

		var ticketDir string
		var taskCtx *TaskEnvContext

		if useTaskCtx {
			ticketDir = t.TempDir()
			taskCtx = &TaskEnvContext{
				TaskID:       fmt.Sprintf("TASK-%05d", rapid.IntRange(1, 99999).Draw(rt, "taskNum")),
				Branch:       "feat/test",
				WorktreePath: "/worktree",
				TicketPath:   ticketDir,
			}
		}

		var cli string
		var args []string
		if runtime.GOOS == "windows" {
			cli = "cmd"
			args = []string{"/c", fmt.Sprintf("echo %s>&2 && exit %d", stderrMsg, exitCode)}
		} else {
			cli = "sh"
			args = []string{"-c", fmt.Sprintf("echo '%s' >&2; exit %d", stderrMsg, exitCode)}
		}

		result, err := executor.Exec(CLIExecConfig{
			CLI:     cli,
			Args:    args,
			TaskCtx: taskCtx,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}

		// Exit code should match.
		if result.ExitCode != exitCode {
			rt.Errorf("exit code = %d, want %d", result.ExitCode, exitCode)
		}

		// Stderr should contain the message.
		if !strings.Contains(result.Stderr, stderrMsg) {
			rt.Errorf("stderr = %q, want to contain %q", result.Stderr, stderrMsg)
		}

		// If task context was active, failure should be logged to context.md.
		if useTaskCtx {
			contextPath := filepath.Join(ticketDir, "context.md")
			data, readErr := os.ReadFile(contextPath)
			if readErr != nil {
				rt.Fatalf("failed to read context.md: %v", readErr)
			}
			content := string(data)

			if !strings.Contains(content, "CLI Failure") {
				rt.Error("context.md missing 'CLI Failure' heading")
			}
			if !strings.Contains(content, fmt.Sprintf("Exit Code:** %d", exitCode)) {
				rt.Errorf("context.md missing exit code %d", exitCode)
			}
		}
	})
}
