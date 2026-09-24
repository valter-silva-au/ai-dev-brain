package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_InitProjectIsDiscoverable pins `init project` into public help. It is
// the only way to provision a document-program pack, so hiding it made the
// whole `adb program` surface look like it had no entry point.
func TestE2E_InitProjectIsDiscoverable(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	help := mustRunADB(t, workspace, "init", "--help").combined()
	for _, want := range []string{"project", "workspace"} {
		if !strings.Contains(help, want) {
			t.Fatalf("adb init --help omits %q:\n%s", want, help)
		}
	}
}

// TestE2E_InitBoundaryAndBareFormAreEquivalent pins the Q2 compatibility
// contract against the real binary: the named spelling is the blessed one, the
// bare positional still works, and the deprecation warning stays on stderr so
// `--format json` remains machine-readable.
func TestE2E_InitBoundaryAndBareFormAreEquivalent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	// Named spelling.
	named := mustRunADB(
		t, parent,
		"init", "boundary", filepath.Join(parent, "named"),
		"--apply", "--format", "json",
	)
	if !strings.Contains(named.stdout, `"outcome": "applied"`) {
		t.Fatalf("init boundary did not apply:\n%s", named.combined())
	}

	// Bare form: same outcome, warning on stderr, stdout still parses as JSON.
	bare := mustRunADB(
		t, parent,
		"init", filepath.Join(parent, "bare"),
		"--apply", "--format", "json",
	)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(bare.stdout), &envelope); err != nil {
		t.Fatalf(
			"bare init stdout is not clean JSON (warning leaked?): %v\nstdout:\n%s",
			err,
			bare.stdout,
		)
	}
	if envelope["outcome"] != "applied" {
		t.Fatalf("bare init outcome = %v, want applied", envelope["outcome"])
	}
	if !strings.Contains(bare.stderr, "init boundary") {
		t.Fatalf("bare init did not point at `init boundary` on stderr:\n%s", bare.stderr)
	}

	// Bare `adb init` prints help instead of initializing.
	help := mustRunADB(t, parent, "init")
	for _, want := range []string{"boundary", "workspace", "project"} {
		if !strings.Contains(help.combined(), want) {
			t.Fatalf("bare `adb init` help omits %q:\n%s", want, help.combined())
		}
	}
}
