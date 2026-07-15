package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// TestValidationTemplates_SyntheticFS proves enumeration is data-driven and pairs
// each worksheet with its adversarial companion, ignoring non-.md files and never
// listing a companion as a template of its own.
func TestValidationTemplates_SyntheticFS(t *testing.T) {
	fsys := fstest.MapFS{
		"validation/problem-hypothesis.md":             {Data: []byte("PH")},
		"validation/problem-hypothesis.adversarial.md": {Data: []byte("PH-ADV")},
		"validation/scope.md":                          {Data: []byte("SCOPE")}, // no companion
		"validation/README.txt":                        {Data: []byte("ignored")},
		"skills/x/SKILL.md":                            {Data: []byte("not validation")},
	}
	ts, err := ValidationTemplates(fsys)
	if err != nil {
		t.Fatalf("ValidationTemplates: %v", err)
	}
	if len(ts) != 2 {
		t.Fatalf("got %d templates, want 2: %+v", len(ts), ts)
	}
	// Ordered by name.
	if ts[0].Name != "problem-hypothesis" || ts[1].Name != "scope" {
		t.Errorf("names/order = %q,%q", ts[0].Name, ts[1].Name)
	}
	if string(ts[0].Content) != "PH" || string(ts[0].Adversarial) != "PH-ADV" {
		t.Errorf("problem-hypothesis content/adversarial not paired: %+v", ts[0])
	}
	if ts[1].Adversarial != nil {
		t.Errorf("scope should have no adversarial companion, got %q", ts[1].Adversarial)
	}
	for _, v := range ts {
		if strings.HasSuffix(v.FileName, adversarialSuffix) {
			t.Errorf("adversarial companion listed as a template: %s", v.FileName)
		}
	}
}

// TestValidationTemplates_MissingRoot tolerates an absent validation/ tree.
func TestValidationTemplates_MissingRoot(t *testing.T) {
	ts, err := ValidationTemplates(fstest.MapFS{})
	if err != nil || len(ts) != 0 {
		t.Errorf("missing root = (%+v, %v), want ([], nil)", ts, err)
	}
}

// TestValidationTemplates_RealEmbed asserts the 9 authored worksheets ship, each
// with a companion adversarial prompt, and that sean-ellis-survey aligns with the
// MVP->Launch gate evidence filename.
func TestValidationTemplates_RealEmbed(t *testing.T) {
	ts, err := ValidationTemplates(claude.FS)
	if err != nil {
		t.Fatalf("ValidationTemplates(claude.FS): %v", err)
	}
	want := []string{
		"evidence-ledger", "false-positive-registry", "interview-framework",
		"measurement-framework", "problem-hypothesis", "problem-statement",
		"scope", "sean-ellis-survey", "target-customer",
	}
	got := map[string]ValidationTemplate{}
	for _, v := range ts {
		got[v.Name] = v
	}
	if len(ts) != len(want) {
		t.Errorf("got %d validation templates, want %d: %v", len(ts), len(want), keysOf(got))
	}
	for _, name := range want {
		v, ok := got[name]
		if !ok {
			t.Errorf("missing validation template %q", name)
			continue
		}
		if len(v.Content) == 0 {
			t.Errorf("%q worksheet is empty", name)
		}
		if len(v.Adversarial) == 0 {
			t.Errorf("%q is missing its companion adversarial prompt", name)
		}
	}
	// Gate-evidence alignment (issue #104 AC).
	if got["sean-ellis-survey"].FileName != "sean-ellis-survey.md" {
		t.Errorf("sean-ellis-survey filename = %q, want sean-ellis-survey.md", got["sean-ellis-survey"].FileName)
	}
}

// TestValidationPack_SatisfiesIdeaMVPGate is the drift-guard for #149: scaffolding
// the real validation pack into an initiative's evidence dir must produce every
// deterministic artifact the Idea→MVP gate requires. Before this, `adb initiative
// scaffold-evidence` wrote problem-hypothesis.md / scope.md / … while the gate
// demanded problem-statement.md + target-customer.md — so the built-in scaffold
// could never satisfy the built-in gate, and the only way past was --override.
// Reading the required list from DeterministicGateArtifacts (not a hard-coded
// slice) means the two can never silently drift apart again.
func TestValidationPack_SatisfiesIdeaMVPGate(t *testing.T) {
	required := DeterministicGateArtifacts(models.StageIdea)
	if len(required) == 0 {
		t.Fatal("Idea→MVP gate has no deterministic artifacts; expected at least problem-statement.md + target-customer.md")
	}

	dir := t.TempDir()
	if _, err := ScaffoldValidationTemplates(claude.FS, dir, HarnessInstallOptions{}); err != nil {
		t.Fatalf("scaffold real pack: %v", err)
	}

	for _, artifact := range required {
		// artifactSatisfied is the exact check the gate runs: the file must
		// exist and be a non-empty regular file under the evidence dir.
		ok, detail := artifactSatisfied(dir, artifact)
		if !ok {
			t.Errorf("gate artifact %q not satisfied by scaffold-evidence: %s", artifact, detail)
		}
	}
}

func keysOf(m map[string]ValidationTemplate) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestScaffoldValidationTemplates covers the scaffold: worksheets are written
// (companions are NOT), idempotent, clobber-safe, dry-run writes nothing.
func TestScaffoldValidationTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"validation/scope.md":             {Data: []byte("SCOPE v1")},
		"validation/scope.adversarial.md": {Data: []byte("ADV")},
		"validation/sean-ellis-survey.md": {Data: []byte("SURVEY v1")},
	}
	dir := t.TempDir()

	res, err := ScaffoldValidationTemplates(fsys, dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if res.Count(HarnessInstalled) != 2 {
		t.Fatalf("want 2 installed: %+v", res.Entries)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "scope.md")); string(b) != "SCOPE v1" {
		t.Errorf("scope.md content = %q", b)
	}
	// The adversarial companion must NOT be scaffolded into the evidence dir.
	if _, err := os.Stat(filepath.Join(dir, "scope.adversarial.md")); !os.IsNotExist(err) {
		t.Error("adversarial companion should not be scaffolded")
	}

	// Idempotent.
	res, _ = ScaffoldValidationTemplates(fsys, dir, HarnessInstallOptions{})
	if res.Count(HarnessUnchanged) != 2 {
		t.Errorf("rerun want 2 unchanged: %+v", res.Entries)
	}

	// Clobber-safe: an edited worksheet is skipped, not overwritten.
	if err := os.WriteFile(filepath.Join(dir, "scope.md"), []byte("MY EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = ScaffoldValidationTemplates(fsys, dir, HarnessInstallOptions{})
	if res.Count(HarnessSkipped) != 1 {
		t.Errorf("want 1 skipped after edit: %+v", res.Entries)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "scope.md")); string(b) != "MY EDIT" {
		t.Errorf("edited worksheet was clobbered: %q", b)
	}

	// Dry-run writes nothing.
	dry := t.TempDir()
	if _, err := ScaffoldValidationTemplates(fsys, dry, HarnessInstallOptions{DryRun: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if entries, _ := os.ReadDir(dry); len(entries) != 0 {
		t.Errorf("dry-run wrote %d files", len(entries))
	}

	// Unresolved dir errors.
	if _, err := ScaffoldValidationTemplates(fsys, "", HarnessInstallOptions{}); err == nil {
		t.Error("empty dest dir should error")
	}
}
