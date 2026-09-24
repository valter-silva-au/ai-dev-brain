// Package e2e drives the REAL built `adb` binary against throwaway workspaces.
//
// Every other test in this repo is in-process: it swaps `cli.App` for one rooted
// at a t.TempDir() and calls cobra handlers directly. That is fast and isolated,
// but it cannot catch anything that only goes wrong when a separate process
// resolves its own workspace, config, and embedded template FS — which is exactly
// where document programs had their shipped path defect.
//
// Two properties this package exists to hold:
//
//  1. ISOLATION, of BOTH config tiers. `ADB_HOME` is commonly exported
//     session-wide, so a child `adb` with an inherited environment resolves the
//     DEVELOPER's real workspace and the gate then looks broken when it is fine.
//     `HOME` matters just as much and is easier to miss: the GLOBAL config tier
//     is `~/.taskconfig`, resolved from `$HOME` independently of `ADB_HOME`, so
//     leaving it inherited lets a real config switch on hooks or repoint the
//     memory embedder underneath a test. Every command here runs with an
//     explicitly overridden environment (see adbEnv) — no test may read or write
//     the real workspace or the real config.
//
//  2. A REAL BINARY. The binary is built ONCE per package run, straight to its
//     destination. On macOS Apple Silicon, `cp`-ing a Go binary invalidates the
//     linker's ad-hoc code signature and the copy is SIGKILL'd on exec (exit 137,
//     with no message) — so this never copies a binary, and re-signs if it must.
package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// adbBin is the absolute path of the binary built by TestMain.
var adbBin string

// cleanHome is an EMPTY directory used as $HOME for every child `adb`, so the
// global config tier (~/.taskconfig) resolves to defaults instead of the
// developer's real configuration. See adbEnv for why this is load-bearing.
//
// It is ONE directory shared by the whole package, which is only safe while it
// contains no configuration files: a test that wrote a .taskconfig into it would
// silently hand every other test in the package a global config tier, and the
// tests it broke would be the ones that never mentioned $HOME. macOS may create
// empty standard directories while resolving user paths, and the mandatory Code
// Defender Git hook writes audit logs under Library/Logs/CodeDefender. TestMain
// permits those non-configuring paths but makes every other file or symlink LOUD.
// If a test must populate $HOME, give it its own directory and pass it through
// adbEnv instead.
var cleanHome string

// TestMain builds ./cmd/adb once for the whole package. Building per-test would
// dominate the runtime; building once means every test below is just process
// spawns against a fixed binary.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "adb-e2e-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: mkdtemp: %v\n", err)
		os.Exit(1)
	}
	// A dedicated empty HOME, deliberately NOT the bin dir: nothing may ever
	// write a .taskconfig here, or every test silently gains a global config.
	cleanHome = filepath.Join(dir, "home")
	if err := os.MkdirAll(cleanHome, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: mkdir clean home: %v\n", err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code, err := buildADB(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	adbBin = code
	result := m.Run()
	if leaked := assertCleanHomeUnconfigured(); leaked != "" {
		fmt.Fprint(os.Stderr, leaked)
		result = 1
	}
	_ = os.RemoveAll(dir)
	os.Exit(result)
}

// assertCleanHomeUnconfigured reports a diagnostic if the shared $HOME contains
// any configuring file or symlink, and "" when it contains only directories or
// mandatory Code Defender audit logs.
//
// The whole package trusts cleanHome to contribute no configuration: the global
// config tier is ~/.taskconfig, so one stray configuration file reconfigures every
// subsequent test — and does it invisibly, because a test that asserts on the repo
// tier has no reason to look at $HOME. This runs after m.Run rather than per-test
// because the damage is cumulative and the guarantee is package-level; per-test it
// would only ever catch the test that happened to run next.
//
// It is reported through TestMain's exit code rather than t.Errorf because by this
// point no *testing.T is live — a silently-green run with a polluted home is the
// exact outcome this exists to prevent.
func assertCleanHomeUnconfigured() string {
	var paths []string
	err := filepath.WalkDir(
		cleanHome,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == cleanHome || entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(cleanHome, path)
			if err != nil {
				return err
			}
			codeDefenderLogRoot := filepath.Join(
				"Library",
				"Logs",
				"CodeDefender",
			) + string(filepath.Separator)
			if strings.HasPrefix(relative, codeDefenderLogRoot) {
				return nil
			}
			paths = append(paths, relative)
			return nil
		},
	)
	if err != nil {
		return fmt.Sprintf("e2e: shared clean HOME %s is unreadable: %v\n", cleanHome, err)
	}
	if len(paths) == 0 {
		return ""
	}
	return fmt.Sprintf("e2e: a test wrote into the SHARED clean HOME (%s): %s\n"+
		"      $HOME is the global config tier (~/.taskconfig) and CLAUDE_CONFIG_DIR's parent, "+
		"so files left here can configure every other test in this package.\n"+
		"      Give that test its own home directory and pass it to adbEnv instead.\n",
		cleanHome, strings.Join(paths, ", "))
}

// buildADB compiles the CLI to dir/adb and returns its path.
//
// The output path is passed to `go build -o` DIRECTLY rather than building
// somewhere and copying: on darwin/arm64 a copied Go binary loses its valid
// ad-hoc signature and is killed on exec. The codesign step below is belt and
// braces for the same failure mode (it is what `make install-local` does).
func buildADB(dir string) (string, error) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	bin := filepath.Join(dir, "adb")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/adb")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ./cmd/adb: %w\n%s", err, out)
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("codesign"); err == nil {
			// Ignore failure: an unsigned-but-freshly-built binary runs fine; this
			// only restores a signature if the toolchain produced none.
			_ = exec.Command("codesign", "--force", "--sign", "-", bin).Run()
		}
	}
	// Prove it actually executes before any test blames its own setup for a
	// SIGKILL. `adb version` uses cobra Run (not RunE) and needs no workspace.
	probe := exec.Command(bin, "version")
	probe.Env = adbEnv(dir, cleanHome)
	if out, err := probe.CombinedOutput(); err != nil {
		return "", fmt.Errorf("built binary is not executable (%w)\n%s", err, out)
	}
	return bin, nil
}

// adbEnv builds the child environment for one `adb` invocation.
//
// It starts from the parent environment (the Go toolchain and PATH are needed)
// but PINS every variable adb resolves configuration through, so a developer's
// own setup can never leak into a test. Anything ADB_*-prefixed that is not
// explicitly set here is dropped rather than inherited.
//
// THREE configuration inputs have to be pinned, and missing any one of them makes
// a test read (or write) real data:
//
//   - ADB_HOME selects the workspace, which is the REPO tier (<ws>/.taskrc).
//   - HOME selects the GLOBAL tier. NewApp passes an empty global-config path,
//     and core.NewViperConfigManager fills it in from os.UserHomeDir() as
//     ~/.taskconfig. So the global tier follows $HOME regardless of ADB_HOME —
//     on a developer machine that is a real config which can enable hooks,
//     point the memory store at an Ollama embedder, and so on.
//   - CLAUDE_CONFIG_DIR selects the Claude Code config directory that
//     `adb sync claude-user` reads and installs the harness into
//     (internal/cli/sync.go:resolveClaudeConfigDir — $CLAUDE_CONFIG_DIR, else
//     ~/.claude). It is a real config input, exported on plenty of developer
//     machines, and unlike the two above it is a WRITE target: left inherited, a
//     test of that command would install into the developer's live Claude config.
//     Pinning it under homeDir rather than merely unsetting it keeps the sandbox
//     explicit instead of relying on the ~/.claude fallback staying put.
//
// homeDir must therefore be a directory with no .taskconfig in it, so the global
// tier resolves to models.DefaultGlobalConfig() and the only configuration in
// play is what a test wrote itself.
func adbEnv(workspace, homeDir string) []string {
	out := make([]string, 0, len(os.Environ())+8)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(key, "ADB_"):
			continue // never inherit adb configuration
		case key == "HOME" || key == "USERPROFILE" || key == "CLAUDE_CONFIG_DIR":
			continue // replaced below — see the doc comment
		}
		out = append(out, kv)
	}
	return append(out,
		// Base-path resolution is ADB_HOME → walk up for .taskconfig → cwd. A
		// scaffolded project writes .taskrc, NOT .taskconfig, so the cwd fallback
		// alone is fragile; ADB_HOME is set explicitly on every call.
		"ADB_HOME="+workspace,
		"ADB_ORG=", // no org tier, so config is the plain two-tier merge
		"ADB_NO_LAUNCH=1",
		"ADB_TMUX=0",
		// The global config tier. USERPROFILE is the Windows equivalent.
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		// The Claude Code config dir, kept inside the sandboxed home.
		"CLAUDE_CONFIG_DIR="+filepath.Join(homeDir, ".claude"),
	)
}

// adbResult is the full outcome of one `adb` invocation.
type adbResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// combined returns stdout+stderr, for assertions that do not care which stream
// carried the message.
func (r adbResult) combined() string { return r.stdout + r.stderr }

// runADB executes the built binary in workspace with a pinned environment. It
// does NOT fail the test on a non-zero exit — several cases assert on failure —
// so callers use mustRunADB when success is required.
func runADB(t *testing.T, workspace string, args ...string) adbResult {
	t.Helper()
	cmd := exec.Command(adbBin, args...)
	cmd.Dir = workspace
	cmd.Env = adbEnv(workspace, cleanHome)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	res := adbResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if err != nil {
		res.exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.exitCode = exitErr.ExitCode()
			// 137 is SIGKILL — on darwin/arm64 that is the broken-code-signature
			// symptom, not a CLI error. Say so rather than letting a test report a
			// confusing empty-output failure.
			if res.exitCode == 137 {
				t.Fatalf("adb was SIGKILL'd (exit 137) — the binary's code signature is invalid "+
					"(a Go binary must not be copied on macOS Apple Silicon). args=%v", args)
			}
		}
	}
	return res
}

// mustRunADB runs a command that is required to succeed.
func mustRunADB(t *testing.T, workspace string, args ...string) adbResult {
	t.Helper()
	res := runADB(t, workspace, args...)
	if res.err != nil {
		t.Fatalf("adb %s failed (exit %d): %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), res.exitCode, res.err, res.stdout, res.stderr)
	}
	return res
}
