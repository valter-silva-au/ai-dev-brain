package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// hookOptionsFromConfig reads MergedConfig.Global.Hooks on App and turns it
// into a core.HookEngineOptions value suitable for NewHookEngineWithOptions.
// The three advanced features (EvidenceGate, OperatorControls, Memory) are
// wired from .taskconfig here — that's the bridge that was missing in PR #51
// and #53 so their features were dormant until the user called the Go API
// directly.
//
// Returns an empty options value (zero-valued, all features off) and a
// nil error when Hooks config is absent, so legacy callers keep working.
// Logs warnings on sub-feature setup failures (memory store open, etc.)
// and returns the caller a partially-configured Options value so the rest
// of hook processing still runs.
func hookOptionsFromConfig() core.HookEngineOptions {
	opts := core.HookEngineOptions{}
	if App == nil || App.MergedConfig == nil {
		return opts
	}
	cfg := resolvedHookConfig(App.MergedConfig)

	// --- Evidence gate
	if cfg.EvidenceGate.Enabled {
		opts.Evidence = core.EvidenceGateConfig{
			Enabled:      true,
			WritePaths:   append([]string(nil), cfg.EvidenceGate.WritePaths...),
			ReadPatterns: append([]string(nil), cfg.EvidenceGate.ReadPatterns...),
		}
	}

	// --- Spec gate (block guarded writes until an accepted ADR exists)
	if cfg.SpecGate.Enabled {
		opts.SpecGate = core.SpecGateConfig{
			Enabled:    true,
			WritePaths: append([]string(nil), cfg.SpecGate.WritePaths...),
			HasSpec:    hasAcceptedADR,
		}
	}

	// --- Operator controls (kill-switch + steer)
	if cfg.OperatorControls.KillSwitchEnabled || cfg.OperatorControls.SteerEnabled {
		opts.Operator = core.OperatorConfig{
			KillSwitchEnabled: cfg.OperatorControls.KillSwitchEnabled,
			KillSwitchFile:    cfg.OperatorControls.KillSwitchFile,
			SteerEnabled:      cfg.OperatorControls.SteerEnabled,
			SteerFile:         cfg.OperatorControls.SteerFile,
		}
	}

	// --- Memory
	if cfg.Memory.Enabled {
		indexer, err := openMemoryIndexerFromConfig(cfg.Memory)
		if err != nil {
			// Non-fatal: the rest of the hook pipeline still runs.
			fmt.Fprintf(os.Stderr, "Warning: memory hook indexer disabled: %v\n", err)
		} else {
			opts.Memory = core.MemoryHookConfig{Enabled: true, Indexer: indexer}
		}
	}

	return opts
}

// hasAcceptedADR reports whether the workspace has at least one accepted ADR.
// It is the spec-gate precondition, injected into the HookEngine so core needn't
// reach into storage. A nil App/ADRManager (or a load error) returns (false, err)
// so the gate fails safe-and-loud rather than silently allowing writes.
func hasAcceptedADR() (bool, error) {
	if App == nil || App.ADRManager == nil {
		// Surface the misconfiguration explicitly (the spec-gate then blocks with
		// "spec-gate check failed: …") rather than looking like "no accepted ADR".
		return false, fmt.Errorf("ADR manager not initialized")
	}
	adrs, err := App.ADRManager.List()
	if err != nil {
		return false, err
	}
	for _, a := range adrs {
		if a.Status == models.ADRAccepted {
			return true, nil
		}
	}
	return false, nil
}

// resolvedHookConfig merges the hook config across the global, org, and repo
// tiers with most-specific-wins precedence (Repo > Org > Global). It delegates
// to models.MergedConfig.ResolvedHooks so the shallow, per-sub-struct merge
// semantics live in exactly one place; the org tier is now honoured here (a
// business can enable evidence-gate / operator-controls / memory once per org).
func resolvedHookConfig(mc *models.MergedConfig) models.HookConfig {
	return mc.ResolvedHooks()
}

// openMemoryIndexerFromConfig constructs a memory.SQLiteStore from the
// MemoryHookConfig schema. The embedder is resolved by
// buildEmbedderFromConfig so .taskconfig can pick fake / openai /
// ollama without special-casing in each hook command.
func openMemoryIndexerFromConfig(mc models.MemoryHookConfig) (core.MemoryIndexer, error) {
	emb, err := buildEmbedderFromConfig(mc.Embedder)
	if err != nil {
		return nil, fmt.Errorf("build embedder: %w", err)
	}
	dbPath := mc.DBPath
	if dbPath == "" {
		dbPath = App.StatePath(statedir.FileMemoryDB)
	}
	// Short-lived ctx just for Open; hook bodies pass their own.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := memory.OpenSQLiteStore(ctx, dbPath, emb)
	if err != nil {
		return nil, fmt.Errorf("open memory store at %q: %w", dbPath, err)
	}
	return store, nil
}

// buildEmbedderFromConfig translates the embedder block in .taskconfig
// into a memory.EmbeddingProvider. It is the config-derived sibling of
// memory.go's flag-derived buildEmbedder(); both, along with the MCP
// search_knowledge tool, delegate to memory.NewEmbedder so the
// provider→embedder mapping lives in exactly one place.
func buildEmbedderFromConfig(ec models.MemoryEmbedderConf) (memory.EmbeddingProvider, error) {
	return memory.NewEmbedder(memory.EmbedderConfig{
		Provider: ec.Provider,
		Model:    ec.Model,
		Endpoint: ec.Endpoint,
		APIKey:   ec.APIKey,
		Dim:      ec.Dim,
	})
}
