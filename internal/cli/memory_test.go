package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestMemoryCommandsExist verifies the memory subtree wires into the root under
// its new home.
//
// TASK-00039 Q4 moved `adb memory` under `adb context` and pruned it to the two
// verbs that serve the knowledge loop. `store`/`delete`/`list` were manual pokes
// at a *derived* index that `index` rebuilds from ticket knowledge, so editing it
// by hand could only introduce drift; `export`/`import` never worked — both
// returned an error on every invocation.
func TestMemoryCommandsExist(t *testing.T) {
	rootCmd := NewRootCmd()

	contextCmd := findCobraSub(rootCmd, "context")
	if contextCmd == nil {
		t.Fatal("context command not registered on root")
	}
	memCmd := findCobraSub(contextCmd, "memory")
	if memCmd == nil {
		t.Fatal("`adb context memory` not registered")
	}
	for _, sub := range []string{"index", "search"} {
		if findCobraSub(memCmd, sub) == nil {
			t.Errorf("`adb context memory %s` not registered", sub)
		}
	}

	// The retired top-level spelling stays reachable for the two surviving verbs.
	retired := findCobraSub(rootCmd, "memory")
	if retired == nil {
		t.Fatal("retired `adb memory` alias tree is missing")
	}
	if !retired.Hidden {
		t.Error("retired `adb memory` must be hidden")
	}
	for _, sub := range []string{"index", "search"} {
		if findCobraSub(retired, sub) == nil {
			t.Errorf("retired `adb memory %s` alias is missing", sub)
		}
	}
	for _, gone := range []string{"store", "delete", "list", "export", "import"} {
		if findCobraSub(retired, gone) != nil {
			t.Errorf("dropped verb `adb memory %s` is still registered", gone)
		}
	}
}

func findCobraSub(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestMemoryCLI_SearchFindsAnIndexedRecord covers the surviving read path.
//
// It used to store via `adb memory store` and then search; that verb was dropped
// in TASK-00039 because it wrote by hand into an index `adb context memory index`
// rebuilds from ticket knowledge, so the only thing it could add was drift. The
// record is therefore seeded through the store API — the same way the indexer
// writes it — and `search` is still exercised through its cobra command, which is
// the part with user-facing output to get wrong.
func TestMemoryCLI_SearchFindsAnIndexedRecord(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app

	// Reset the package-level memory flags so prior tests don't leak state.
	memoryDBPath = filepath.Join(tmp, "memory.sqlite")
	memoryProvider = "fake"
	memoryDim = 64
	memoryModel = ""
	memoryEndpoint = ""
	memoryAPIKey = ""

	ctx := context.Background()
	const content = "the cli roundtrip record for search to find"

	store, err := openStoreFromFlags(ctx)
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	if err := store.Upsert(ctx, "tickets/T-1", "notes", content, nil); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	cmd := newMemorySearchCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"tickets/T-1", content})
	_ = cmd.Flags().Set("json", "true")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("search Execute: %v\n%s", err, out.String())
	}
	body := out.String()
	if body == "" {
		t.Fatal("search produced no output")
	}
	if !strings.Contains(body, `"Key": "notes"`) {
		t.Errorf("search JSON did not include the seeded key:\n%s", body)
	}
}
