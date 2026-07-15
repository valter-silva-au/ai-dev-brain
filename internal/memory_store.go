package internal

import (
	"context"
	"fmt"
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// OpenMemoryStore opens the workspace's vector-memory store for search,
// resolving the embedder from the merged Hooks.Memory config. It is the shared
// entry point the MCP search_knowledge tool uses so behaviour matches the
// `adb memory` CLI and the memory hook.
//
// It returns configured=false (nil store, nil error) when no knowledge base
// exists yet — the memory db file is absent — so callers can degrade gracefully
// with a clear notice instead of erroring. When configured is true the caller
// owns Close().
func (app *App) OpenMemoryStore(ctx context.Context) (store memory.Store, configured bool, err error) {
	memCfg := app.resolvedMemoryConfig()
	dbPath := memCfg.DBPath
	if dbPath == "" {
		dbPath = app.StatePath(statedir.FileMemoryDB)
	}
	// The db file's presence is the "is there a knowledge base?" signal — a user
	// may have stored records via `adb memory store` without enabling the hook,
	// so we key off the file, not memCfg.Enabled.
	if _, statErr := os.Stat(dbPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat memory db %q: %w", dbPath, statErr)
	}
	emb, err := memory.NewEmbedder(memory.EmbedderConfig{
		Provider: memCfg.Embedder.Provider,
		Model:    memCfg.Embedder.Model,
		Endpoint: memCfg.Embedder.Endpoint,
		APIKey:   memCfg.Embedder.APIKey,
		Dim:      memCfg.Embedder.Dim,
	})
	if err != nil {
		return nil, false, fmt.Errorf("build embedder: %w", err)
	}
	s, err := memory.OpenSQLiteStore(ctx, dbPath, emb)
	if err != nil {
		return nil, false, fmt.Errorf("open memory store at %q: %w", dbPath, err)
	}
	return s, true, nil
}

// resolvedMemoryConfig returns the Memory hook block resolved across all three
// config tiers (Repo > Org > Global) via MergedConfig.ResolvedHooks, so an org
// tier that opts into vector memory is honoured here too.
func (app *App) resolvedMemoryConfig() models.MemoryHookConfig {
	if app.MergedConfig == nil {
		return models.MemoryHookConfig{}
	}
	return app.MergedConfig.ResolvedHooks().Memory
}
