package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	memCmd.PersistentFlags().StringVar(&memoryDBPath, "db-path", "", "path to SQLite file (default: <workspace>/.adb/memory.sqlite)")
	memCmd.PersistentFlags().StringVar(&memoryProvider, "provider", "fake", "embedding provider: fake | openai | ollama")
	memCmd.PersistentFlags().StringVar(&memoryModel, "model", "", "embedding model (provider-specific)")
	memCmd.PersistentFlags().StringVar(&memoryEndpoint, "endpoint", "", "provider endpoint URL (openai: full URL incl /v1/embeddings; ollama: base URL)")
	memCmd.PersistentFlags().IntVar(&memoryDim, "dim", 64, "embedding dimensions (must match provider/model)")
	memCmd.PersistentFlags().StringVar(&memoryAPIKey, "api-key", "", "API key (may reference env var: $OPENAI_API_KEY)")

	memCmd.AddCommand(newMemoryStoreCmd())
	memCmd.AddCommand(newMemorySearchCmd())
	memCmd.AddCommand(newMemoryDeleteCmd())
	memCmd.AddCommand(newMemoryListCmd())
	memCmd.AddCommand(newMemoryIndexCmd())
	memCmd.AddCommand(newMemoryExportCmd())
	memCmd.AddCommand(newMemoryImportCmd())
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

func newMemoryStoreCmd() *cobra.Command {
	var contentFlag, fileFlag string
	var metaFlags []string
	cmd := &cobra.Command{
		Use:   "store <namespace> <key>",
		Short: "Upsert a record into the memory store",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, key := args[0], args[1]
			content, err := resolveContent(cmd.InOrStdin(), contentFlag, fileFlag)
			if err != nil {
				return err
			}
			meta := parseMeta(metaFlags)
			ctx := cmd.Context()
			store, err := openStoreFromFlags(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Upsert(ctx, ns, key, content, meta); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ stored %s/%s (%d chars)\n", ns, key, len(content))
			return nil
		},
	}
	cmd.Flags().StringVar(&contentFlag, "content", "", "content string (use --file or pipe stdin for longer input)")
	cmd.Flags().StringVar(&fileFlag, "file", "", "read content from file")
	cmd.Flags().StringSliceVar(&metaFlags, "meta", nil, "key=value metadata entries (repeatable or comma-separated)")
	return cmd
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

func newMemoryDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <namespace> <key>",
		Short: "Delete a record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := openStoreFromFlags(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Delete(ctx, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ deleted %s/%s\n", args[0], args[1])
			return nil
		},
	}
}

func newMemoryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List namespaces in the store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := openStoreFromFlags(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			nss, err := store.ListNamespaces(ctx)
			if err != nil {
				return err
			}
			for _, ns := range nss {
				fmt.Fprintln(cmd.OutOrStdout(), ns)
			}
			return nil
		},
	}
}

func newMemoryExportCmd() *cobra.Command {
	// Export is a thin convenience: run a single broad query per
	// namespace. For backup purposes users should use `sqlite3
	// .adb/memory.sqlite ".backup ..."` which preserves everything including
	// vector BLOBs. This command exposes only the logical content + meta so
	// humans can eyeball / grep.
	return &cobra.Command{
		Use:   "export",
		Short: "Dump entries (content + meta, no vectors) as JSONL to stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("export/import are deferred — use sqlite3 .adb/memory.sqlite \".backup backup.sqlite\" for full-fidelity backup")
		},
	}
}

func newMemoryImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import",
		Short: "Import entries from JSONL (deferred)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("export/import are deferred — see `adb memory export --help`")
		},
	}
}

// resolveContent picks content from --content, --file, or stdin in that
// priority order. Exactly one must be non-empty.
func resolveContent(stdin io.Reader, contentFlag, fileFlag string) (string, error) {
	if contentFlag != "" && fileFlag != "" {
		return "", fmt.Errorf("--content and --file are mutually exclusive")
	}
	if contentFlag != "" {
		return contentFlag, nil
	}
	if fileFlag != "" {
		b, err := os.ReadFile(fileFlag)
		if err != nil {
			return "", fmt.Errorf("read --file %q: %w", fileFlag, err)
		}
		return string(b), nil
	}
	// Stdin.
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if len(b) == 0 {
		return "", fmt.Errorf("no content: use --content, --file, or pipe via stdin")
	}
	return string(b), nil
}

// parseMeta turns ["k=v", "a=b,c=d"] into a map.
func parseMeta(flags []string) map[string]string {
	out := map[string]string{}
	for _, f := range flags {
		for _, pair := range strings.Split(f, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			eq := strings.IndexByte(pair, '=')
			if eq < 0 {
				continue
			}
			out[pair[:eq]] = pair[eq+1:]
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
