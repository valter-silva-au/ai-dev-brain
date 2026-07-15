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

// TestMemoryCommandsExist verifies the memory subtree wires into the root.
func TestMemoryCommandsExist(t *testing.T) {
	rootCmd := NewRootCmd()
	memCmd := findCobraSub(rootCmd, "memory")
	if memCmd == nil {
		t.Fatal("memory command not registered on root")
	}
	for _, sub := range []string{"store", "search", "delete", "list", "export", "import"} {
		if findCobraSub(memCmd, sub) == nil {
			t.Errorf("memory subcommand %q not registered", sub)
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

// TestMemoryCLI_StoreSearchRoundtrip is a small end-to-end: store a
// record via the CLI, then search and assert a hit, both using the fake
// embedder so the test doesn't need network.
func TestMemoryCLI_StoreSearchRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	// Reset the package-level memory flags so prior tests don't leak state.
	memoryDBPath = filepath.Join(tmp, ".adb_memory.sqlite")
	memoryProvider = "fake"
	memoryDim = 64
	memoryModel = ""
	memoryEndpoint = ""
	memoryAPIKey = ""

	ctx := context.Background()
	{
		cmd := newMemoryStoreCmd()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"tickets/T-1", "notes"})
		_ = cmd.Flags().Set("content", "the cli roundtrip record for search to find")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("store Execute: %v\n%s", err, out.String())
		}
		if got := out.String(); got == "" {
			t.Errorf("store produced no output")
		}
	}
	{
		cmd := newMemorySearchCmd()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"tickets/T-1", "the cli roundtrip record for search to find"})
		_ = cmd.Flags().Set("json", "true")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("search Execute: %v\n%s", err, out.String())
		}
		body := out.String()
		if body == "" {
			t.Errorf("search produced no output")
		}
		// Crude assertion: the stored key must appear in the JSON.
		if !bytesContains(body, "\"Key\": \"notes\"") {
			t.Errorf("search JSON did not include the stored key:\n%s", body)
		}
	}
}

func bytesContains(s, substr string) bool { return strings.Contains(s, substr) }
