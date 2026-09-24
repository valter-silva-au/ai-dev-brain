package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
)

// Shared flags for commands that open the store.
var (
	memoryDBPath   string
	memoryProvider string
	memoryModel    string
	memoryEndpoint string
	memoryDim      int
	memoryAPIKey   string
)

// NewMemoryCmd builds the `adb memory` command tree.
func NewMemoryCmd() *cobra.Command {
	memCmd := &cobra.Command{
		Use:   "memory",
		Short: "Namespaced vector-memory store",
		Long: `Vector-memory commands for adb.

Stores and searches semantically-embedded records keyed by (namespace, key).
Default-off; enable by passing --db-path or via hooks.memory.enabled in
.taskconfig. Embeddings come from a pluggable provider (fake for tests,
OpenAI-compatible HTTP, or Ollama).`,
	}

	addMemoryConnectionFlags(memCmd)

	// Only the two verbs that serve the knowledge loop (TASK-00039 Q4).
	// `store`/`delete`/`list` were manual pokes at a *derived* index that `index`
	// rebuilds from ticket knowledge, so hand-editing could only cause drift;
	// `export`/`import` returned an error on every invocation. The underlying
	// store methods survive — MCP's search_knowledge reads the same store.
	memCmd.AddCommand(newMemoryIndexCmd())
	memCmd.AddCommand(newMemorySearchCmd())
	return memCmd
}

// newMemoryIndexCmd builds `adb memory index` — the explicit connection from the
// knowledge/graph pipeline to the vector store (issue #121). It indexes every
// ticket's knowledge files + the graph's typed edges so the MCP search_knowledge
// tool (#113) surfaces real workspace content.
//
// Opt-in model: running this command IS the opt-in — it uses the same
// --db-path / --provider flags as the rest of `adb memory` and writes to the same
// default db (<workspace>/.adb/memory.sqlite) that search_knowledge reads. This
// is the MANUAL, full-workspace counterpart to the AUTOMATIC per-completion
// indexing done by the memory hook, which is gated by hooks.memory.enabled.
func newMemoryIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Index ticket knowledge + graph edges into the store (feeds search_knowledge)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialised")
			}
			ctx := cmd.Context()
			store, err := openStoreFromFlags(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			ki := core.NewKnowledgeIndexer(store, App.BacklogManager, App.GraphManager, filepath.Join(App.BasePath, "tickets"))
			stats, err := ki.IndexWorkspace(ctx)
			if err != nil {
				return fmt.Errorf("index workspace: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"✓ Indexed %d knowledge file(s) across %d ticket(s) + %d graph edge(s) into the vector store.\n",
				stats.Files, stats.Tickets, stats.Edges)
			return nil
		},
	}
}

// openStoreFromFlags constructs a memory.SQLiteStore from the current
// flag values. It is called from each subcommand's RunE so that flag
// parsing happens first (some flags only take effect after Cobra has
// walked the tree).
func openStoreFromFlags(ctx context.Context) (*memory.SQLiteStore, error) {
	if App == nil {
		return nil, fmt.Errorf("app not initialised")
	}
	dbPath := memoryDBPath
	if dbPath == "" {
		dbPath = App.StatePath(statedir.FileMemoryDB)
	}
	emb, err := buildEmbedder()
	if err != nil {
		return nil, fmt.Errorf("build embedder: %w", err)
	}
	return memory.OpenSQLiteStore(ctx, dbPath, emb)
}

// buildEmbedder converts the --provider / --model / --endpoint / --dim /
// --api-key flags into a concrete memory.EmbeddingProvider. Like the
// config-derived buildEmbedderFromConfig, it delegates to memory.NewEmbedder so
// the provider→embedder mapping lives in exactly one place.
func buildEmbedder() (memory.EmbeddingProvider, error) {
	return memory.NewEmbedder(memory.EmbedderConfig{
		Provider: memoryProvider,
		Model:    memoryModel,
		Endpoint: memoryEndpoint,
		APIKey:   memoryAPIKey,
		Dim:      memoryDim,
	})
}

func newMemorySearchCmd() *cobra.Command {
	var k int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "search <namespace> <query>",
		Short: "Semantic search within a namespace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, query := args[0], args[1]
			ctx := cmd.Context()
			store, err := openStoreFromFlags(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			hits, err := store.Search(ctx, ns, query, k)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			for i, h := range hits {
				fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s/%s  score=%.4f\n    %s\n",
					i+1, h.Namespace, h.Key, h.Score, truncate(h.Content, 120))
			}
			if len(hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no hits)")
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&k, "k", 5, "number of results to return")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON array")
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// addMemoryConnectionFlags installs the embedding-store connection flags as
// PERSISTENT flags on a memory command tree, so every child inherits them.
//
// Extracted so the retired `adb memory` alias tree carries them too: they are
// persistent on the parent rather than declared per child, so an alias tree
// without them would make `adb memory search --provider ollama` fail to parse
// while the new spelling accepted it.
func addMemoryConnectionFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&memoryDBPath, "db-path", "", "path to SQLite file (default: <workspace>/.adb/memory.sqlite)")
	cmd.PersistentFlags().StringVar(&memoryProvider, "provider", "fake", "embedding provider: fake | openai | ollama")
	cmd.PersistentFlags().StringVar(&memoryModel, "model", "", "embedding model (provider-specific)")
	cmd.PersistentFlags().StringVar(&memoryEndpoint, "endpoint", "", "provider endpoint URL (openai: full URL incl /v1/embeddings; ollama: base URL)")
	cmd.PersistentFlags().IntVar(&memoryDim, "dim", 64, "embedding dimensions (must match provider/model)")
	cmd.PersistentFlags().StringVar(&memoryAPIKey, "api-key", "", "API key (may reference env var: $OPENAI_API_KEY)")
}
