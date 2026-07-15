package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestInitiativeCLI_ScaffoldEvidence drives `adb initiative scaffold-evidence`
// end-to-end: it drops the validation worksheets into the initiative's evidence
// dir, is clobber-safe, and rejects an unknown initiative.
func TestInitiativeCLI_ScaffoldEvidence(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	oldApp := App
	App = app
	defer func() { App = oldApp }()

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}

	// Unknown initiative is rejected.
	if err := runADB(t, "initiative", "scaffold-evidence", "ghost"); err == nil {
		t.Error("scaffold-evidence on an unknown initiative should error")
	}

	// Scaffold into the real evidence dir.
	if err := runADB(t, "initiative", "scaffold-evidence", "widget"); err != nil {
		t.Fatalf("scaffold-evidence: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	for _, name := range []string{"problem-hypothesis.md", "sean-ellis-survey.md", "false-positive-registry.md"} {
		if info, err := os.Stat(filepath.Join(evDir, name)); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty scaffolded %s (err=%v)", name, err)
		}
	}
	// The adversarial companion is NOT scaffolded into the evidence dir.
	if _, err := os.Stat(filepath.Join(evDir, "problem-hypothesis.adversarial.md")); !os.IsNotExist(err) {
		t.Error("adversarial companion should not be scaffolded into the evidence dir")
	}

	// A local edit survives a plain re-run (clobber-safe).
	edited := filepath.Join(evDir, "scope.md")
	if err := os.WriteFile(edited, []byte("MY SCOPE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runADB(t, "initiative", "scaffold-evidence", "widget"); err != nil {
		t.Fatalf("scaffold-evidence re-run: %v", err)
	}
	if b, _ := os.ReadFile(edited); string(b) != "MY SCOPE" {
		t.Errorf("re-run clobbered a local edit: %q", b)
	}
}

// TestInitiativeCLI_LintInterview drives `adb initiative lint-interview`: a file
// with a Mom Test violation exits non-zero; a clean file passes.
func TestInitiativeCLI_LintInterview(t *testing.T) {
	dir := t.TempDir()

	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("Would you pay for this?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runADB(t, "initiative", "lint-interview", bad); err == nil {
		t.Error("lint-interview should fail on a file with Mom Test violations")
	}

	good := filepath.Join(dir, "good.md")
	if err := os.WriteFile(good, []byte("How do you currently handle this?\nWhat did you do last time?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runADB(t, "initiative", "lint-interview", good); err != nil {
		t.Errorf("lint-interview should pass a clean file, got: %v", err)
	}
}
