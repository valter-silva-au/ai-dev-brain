package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

func TestGTMPacks_FromEmbed(t *testing.T) {
	packs, err := GTMPacks(claude.FS)
	if err != nil {
		t.Fatalf("GTMPacks: %v", err)
	}
	want := map[string]bool{"positioning": false, "moat": false}
	for _, p := range packs {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("expected GTM pack %q in %v", p, packs)
		}
	}
}

func TestScaffoldGTMPack(t *testing.T) {
	dir := t.TempDir()
	res, err := ScaffoldGTMPack(claude.FS, "moat", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected at least one scaffolded doc")
	}
	data, err := os.ReadFile(filepath.Join(dir, "moat-narrative.md"))
	if err != nil || len(data) == 0 {
		t.Fatalf("moat-narrative.md not written: %v (len %d)", err, len(data))
	}

	// Unknown pack errors.
	if _, err := ScaffoldGTMPack(claude.FS, "nope", dir, HarnessInstallOptions{}); err == nil {
		t.Error("expected error for unknown pack")
	}
}
