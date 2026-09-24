package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ===== Isolation in this package ============================================
//
// Every App built in internal/cli comes from internal.NewAppIsolated, never
// internal.NewApp. A t.TempDir() basePath alone pins only the REPO tier
// (<basePath>/.taskrc); the GLOBAL tier still defaults to $HOME/.taskconfig, the
// terminal-state file to $HOME/.adb_terminal_state.json, and the active org to
// $ADB_ORG. On this repo's own workstation that global file enables hooks.memory
// with an ollama embedder — which is exactly how
// TestHookOptionsFromConfig_AllDisabledByDefault came to fail with
// "Memory should default off, got Enabled=true" while passing in CI.
//
// NewAppIsolated resolves all three under basePath instead, so the fixture a test
// writes is the only input. That replaces the old t.Setenv("HOME", …) +
// USERPROFILE + ADB_ORG="" helper this file used to carry: the constructor cannot
// be forgotten, and it does not forbid t.Parallel the way t.Setenv does.
//
// Note the leak was never via ADB_HOME: that is read only by cmd/adb/main.go when
// it resolves a base path to hand to NewApp, so it cannot reach an in-process
// test that passes basePath directly.
//
// t.Setenv survives here only where a test needs an env var for a DIFFERENT
// reason than config tiers — CLAUDE_CONFIG_DIR (sync_test.go: a write target),
// $HOME for the Claude *desktop* config lookup (team_test.go), ADB_HOME for
// base-path resolution, ADB_NO_LAUNCH, and so on. Each such site says why.
//
// ---- Why almost nothing here is t.Parallel ----
// Two process-wide barriers, both independent of the environment:
//   - `App` (vars.go) is a package-level singleton that nearly every test in this
//     package swaps for its own; and
//   - captureStdout (hook_test.go) reassigns os.Stdout for the duration of fn.
// Either one makes two concurrent tests interfere, so t.Parallel appears only on
// tests that touch neither.

// TestHookOptionsFromConfig_AllDisabledByDefault — with no Hooks block
// in .taskconfig, all three advanced features stay off (zero-valued
// options).
func TestHookOptionsFromConfig_AllDisabledByDefault(t *testing.T) {
	tmp := t.TempDir()
	// Isolated: no <tmp>/.taskconfig exists, so the global tier is
	// models.DefaultGlobalConfig() — which leaves
	// Memory/EvidenceGate/SpecGate/OperatorControls zero-valued, i.e. genuinely
	// off. That default is the whole assertion below.
	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app

	opts := hookOptionsFromConfig()
	if opts.Evidence.Enabled {
		t.Errorf("EvidenceGate should default off, got Enabled=true")
	}
	if opts.Operator.KillSwitchEnabled || opts.Operator.SteerEnabled {
		t.Errorf("OperatorControls should default off, got %+v", opts.Operator)
	}
	if opts.Memory.Enabled {
		t.Errorf("Memory should default off, got Enabled=true")
	}
}

// TestHookOptionsFromConfig_MemoryEnabled — write a .taskrc with
// memory.enabled=true, confirm the resulting options contain a live
// Indexer that can Upsert.
func TestHookOptionsFromConfig_MemoryEnabled(t *testing.T) {
	// The repo tier below overrides the global Memory block outright
	// (MergedConfig.ResolvedHooks), so this case does not depend on the
	// developer's ~/.taskconfig for its assertions — but an un-isolated App would
	// still MERGE it. NewAppIsolated keeps the .taskrc fixture the only input.
	tmp := t.TempDir()

	// Repo-level config (.taskrc) is read by Viper via ViperConfigManager.
	// It merges into MergedConfig.Global.Hooks.
	taskrc := `
name: e2e
hooks:
  memory:
    enabled: true
    db_path: ` + filepath.Join(tmp, "memory-e2e.sqlite") + `
    embedder:
      provider: fake
      dim: 32
`
	if err := os.WriteFile(filepath.Join(tmp, ".taskrc"), []byte(taskrc), 0o644); err != nil {
		t.Fatalf("write .taskrc: %v", err)
	}

	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app

	opts := hookOptionsFromConfig()
	if !opts.Memory.Enabled {
		t.Fatalf("Memory.Enabled should be true from config, got false")
	}
	if opts.Memory.Indexer == nil {
		t.Fatal("Memory.Indexer should be non-nil when Enabled is true")
	}

	// Confirm the indexer actually works (construct a call that would
	// hit SQLite + HNSW end-to-end).
	ctx := context.Background()
	if err := opts.Memory.Indexer.Upsert(ctx, "tickets/T-1", "notes", "config-wired memory roundtrip", nil); err != nil {
		t.Fatalf("Upsert via config-wired indexer: %v", err)
	}

	// Tear down: the indexer holds a SQLite handle; close it so the
	// tempdir can be removed cleanly on Windows.
	if closer, ok := opts.Memory.Indexer.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

// TestHookOptionsFromConfig_EvidenceGate — .taskrc toggles the evidence
// gate; options surface the right paths.
func TestHookOptionsFromConfig_EvidenceGate(t *testing.T) {
	// Isolated: the .taskrc below must be the only tier in play (no global
	// ~/.taskconfig merge, no $ADB_ORG-selected org tier).
	tmp := t.TempDir()
	taskrc := `
name: e2e
hooks:
  evidence_gate:
    enabled: true
    write_paths:
      - test-results.json
    read_patterns:
      - "*.png"
      - "*.txt"
`
	if err := os.WriteFile(filepath.Join(tmp, ".taskrc"), []byte(taskrc), 0o644); err != nil {
		t.Fatalf("write .taskrc: %v", err)
	}
	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app

	opts := hookOptionsFromConfig()
	if !opts.Evidence.Enabled {
		t.Fatal("Evidence should be enabled from config")
	}
	if len(opts.Evidence.WritePaths) != 1 || opts.Evidence.WritePaths[0] != "test-results.json" {
		t.Errorf("WritePaths = %v, want [test-results.json]", opts.Evidence.WritePaths)
	}
	if len(opts.Evidence.ReadPatterns) != 2 {
		t.Errorf("ReadPatterns len = %d, want 2", len(opts.Evidence.ReadPatterns))
	}
}

// TestHookOptionsFromConfig_OperatorControls — kill-switch + steer flip
// with default file names when unspecified.
func TestHookOptionsFromConfig_OperatorControls(t *testing.T) {
	// Isolated: the .taskrc below must be the only tier in play (no global
	// ~/.taskconfig merge, no $ADB_ORG-selected org tier).
	tmp := t.TempDir()
	taskrc := `
name: e2e
hooks:
  operator_controls:
    kill_switch_enabled: true
    steer_enabled: true
`
	if err := os.WriteFile(filepath.Join(tmp, ".taskrc"), []byte(taskrc), 0o644); err != nil {
		t.Fatalf("write .taskrc: %v", err)
	}
	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app

	opts := hookOptionsFromConfig()
	if !opts.Operator.KillSwitchEnabled {
		t.Error("KillSwitch should be enabled")
	}
	if !opts.Operator.SteerEnabled {
		t.Error("Steer should be enabled")
	}
}

// TestBuildEmbedderFromConfig_Providers — each of the four supported
// providers maps to a concrete EmbeddingProvider with the expected
// Name().
func TestBuildEmbedderFromConfig_Providers(t *testing.T) {
	// Safe to parallelise: this is the one test in this file that touches neither
	// barrier — no App, no captureStdout, no env. buildEmbedderFromConfig is a
	// pure constructor over its argument.
	t.Parallel()
	cases := []struct {
		name     string
		provider string
		model    string
		wantName string
	}{
		{"fake default", "", "", "fake"},
		{"fake explicit", "fake", "", "fake"},
		{"openai with model", "openai", "text-embedding-3-small", "openai/text-embedding-3-small"},
		{"ollama with model", "ollama", "nomic-embed-text", "ollama/nomic-embed-text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			emb, err := buildEmbedderFromConfig(models.MemoryEmbedderConf{Provider: c.provider, Model: c.model})
			if err != nil {
				t.Fatalf("buildEmbedderFromConfig: %v", err)
			}
			if emb.Name() != c.wantName {
				t.Errorf("Name() = %q, want %q", emb.Name(), c.wantName)
			}
		})
	}

	// Unknown provider → error.
	if _, err := buildEmbedderFromConfig(models.MemoryEmbedderConf{Provider: "bogus"}); err == nil {
		t.Error("expected error for unknown provider")
	}
}

// Compile-time assertion: the returned indexer implements the memory
// Store contract (SQLiteStore does).
var _ = memory.NewFakeEmbedder
