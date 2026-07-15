package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

func TestComplianceFrameworks_FromEmbed(t *testing.T) {
	frameworks, err := ComplianceFrameworks(claude.FS)
	if err != nil {
		t.Fatalf("ComplianceFrameworks: %v", err)
	}
	want := map[string]bool{"soc2": false, "gdpr": false, "hipaa": false}
	for _, f := range frameworks {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for f, seen := range want {
		if !seen {
			t.Errorf("expected framework %q in %v", f, frameworks)
		}
	}
}

func TestScaffoldComplianceFramework(t *testing.T) {
	dir := t.TempDir()
	res, err := ScaffoldComplianceFramework(claude.FS, "soc2", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected at least one scaffolded doc")
	}
	// controls.md landed and is non-empty.
	data, err := os.ReadFile(filepath.Join(dir, "controls.md"))
	if err != nil || len(data) == 0 {
		t.Fatalf("controls.md not written: %v (len %d)", err, len(data))
	}

	// Re-scaffold is idempotent: every entry unchanged.
	res2, err := ScaffoldComplianceFramework(claude.FS, "soc2", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("re-scaffold: %v", err)
	}
	for _, e := range res2.Entries {
		if e.Action != HarnessUnchanged {
			t.Errorf("re-scaffold %s = %s, want unchanged", e.Name, e.Action)
		}
	}

	// Unknown framework errors.
	if _, err := ScaffoldComplianceFramework(claude.FS, "pci", dir, HarnessInstallOptions{}); err == nil {
		t.Error("expected error for unknown framework")
	}
}
