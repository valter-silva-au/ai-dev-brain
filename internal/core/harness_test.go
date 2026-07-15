package core

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// find returns the HarnessFile with the given kind+relpath, or nil.
func find(files []HarnessFile, kind HarnessKind, rel string) *HarnessFile {
	for i := range files {
		if files[i].Kind == kind && files[i].RelPath == rel {
			return &files[i]
		}
	}
	return nil
}

// TestHarnessManifest_SyntheticFS proves the manifest is data-driven over an
// arbitrary fs.FS: it enumerates exactly the files under agents/ and skills/
// (nested included), records the right kind + root-relative path + content, and
// ignores everything outside those two trees.
func TestHarnessManifest_SyntheticFS(t *testing.T) {
	fsys := fstest.MapFS{
		"agents/devils-advocate.md":  {Data: []byte("agent A")},
		"agents/second-agent.md":     {Data: []byte("agent B")},
		"skills/stage-gate/SKILL.md": {Data: []byte("skill body")},
		"skills/other/SKILL.md":      {Data: []byte("other skill")},
		"projectinit/base/CLAUDE.md": {Data: []byte("not harness")},
		"README.md":                  {Data: []byte("not harness")},
	}

	files, err := HarnessManifest(fsys)
	if err != nil {
		t.Fatalf("HarnessManifest: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("got %d harness files, want 4: %+v", len(files), files)
	}

	// Agents come before skills; within a kind, lexical order.
	want := []struct {
		kind HarnessKind
		rel  string
	}{
		{HarnessAgent, "devils-advocate.md"},
		{HarnessAgent, "second-agent.md"},
		{HarnessSkill, "other/SKILL.md"},
		{HarnessSkill, "stage-gate/SKILL.md"},
	}
	for i, w := range want {
		if files[i].Kind != w.kind || files[i].RelPath != w.rel {
			t.Errorf("files[%d] = {%s %q}, want {%s %q}", i, files[i].Kind, files[i].RelPath, w.kind, w.rel)
		}
	}

	// Content is carried through; a nested skill keeps its sub-path.
	if f := find(files, HarnessSkill, "stage-gate/SKILL.md"); f == nil || string(f.Content) != "skill body" {
		t.Errorf("stage-gate skill content not carried through: %+v", f)
	}
	// Files outside agents/ and skills/ are excluded.
	if find(files, HarnessAgent, "../README.md") != nil || len(files) != 4 {
		t.Error("manifest leaked a non-harness file")
	}
}

// TestHarnessManifest_MissingRootsTolerant verifies a kind whose root is absent
// (or an entirely empty FS) contributes no files rather than erroring.
func TestHarnessManifest_MissingRootsTolerant(t *testing.T) {
	// Only agents/, no skills/.
	agentsOnly := fstest.MapFS{"agents/x.md": {Data: []byte("x")}}
	files, err := HarnessManifest(agentsOnly)
	if err != nil {
		t.Fatalf("HarnessManifest (agents only): %v", err)
	}
	if len(files) != 1 || files[0].Kind != HarnessAgent {
		t.Errorf("agents-only manifest = %+v, want one agent", files)
	}

	// Empty FS → no files, no error.
	files, err = HarnessManifest(fstest.MapFS{})
	if err != nil {
		t.Fatalf("HarnessManifest (empty): %v", err)
	}
	if len(files) != 0 {
		t.Errorf("empty FS manifest = %+v, want none", files)
	}
}

// TestHarnessManifest_RealEmbed asserts the authored harness actually ships in
// the embedded template filesystem: the devils-advocate agent and the stage-gate
// skill, with the shape their consumers depend on.
func TestHarnessManifest_RealEmbed(t *testing.T) {
	files, err := HarnessManifest(claude.FS)
	if err != nil {
		t.Fatalf("HarnessManifest(claude.FS): %v", err)
	}

	agent := find(files, HarnessAgent, "devils-advocate.md")
	if agent == nil {
		t.Fatal("devils-advocate agent not found in the embedded harness")
	}
	body := string(agent.Content)
	// Frontmatter name (Claude Code keys the subagent off it) + the machine-readable
	// verdict contract the hybrid gate (issue #102) will parse.
	if !strings.Contains(body, "name: devils-advocate") {
		t.Error("devils-advocate agent missing its frontmatter name")
	}
	if !strings.Contains(body, "VERDICT: pass | fail") {
		t.Error("devils-advocate agent missing the VERDICT output contract")
	}

	skill := find(files, HarnessSkill, "stage-gate/SKILL.md")
	if skill == nil {
		t.Fatal("stage-gate skill not found in the embedded harness")
	}
	if !strings.Contains(string(skill.Content), "name: stage-gate") {
		t.Error("stage-gate skill missing its frontmatter name")
	}

	// Every embedded harness file must be non-empty (an empty SKILL.md/agent is a
	// packaging bug).
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("embedded harness file %s/%s is empty", f.Kind, f.RelPath)
		}
	}
}
