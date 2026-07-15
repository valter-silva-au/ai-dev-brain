package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type fakeWikiClassifier struct{ org, initiative string }

func (f fakeWikiClassifier) Classify(string) (string, string) { return f.org, f.initiative }

// TestWikiPublisher_Upgrade covers issue #127: graph cross-links, org/initiative
// namespacing, index/home + per-tag + per-initiative pages, llms.txt + AGENTS.md,
// and semantic-search indexing.
func TestWikiPublisher_Upgrade(t *testing.T) {
	base := t.TempDir()
	seedKnowledge(t, base, "TASK-00001", &models.ExtractedKnowledge{
		TaskID:  "TASK-00001",
		Summary: "Auth rework.",
		Decisions: []models.Decision{{
			ID: "D1", Title: "Use JWT", Status: "accepted", Tags: []string{"security"},
		}},
	})

	p := newTestPublisher(base)
	p.SetGraph(&fakeGraph{neighbors: map[string][]models.GraphEdge{
		"TASK-00001": {{From: "TASK-00001", Type: models.EdgeRelatesTo, To: "TASK-00002"}},
	}})
	p.SetClassifier(fakeWikiClassifier{org: "acme", initiative: "onboarding"})
	idx := &fakeMemIndexer{}
	p.SetIndexer(idx)

	outDir := filepath.Join(base, "out")
	res, err := p.PublishAll(outDir)
	if err != nil {
		t.Fatalf("PublishAll: %v", err)
	}

	// The ticket page is namespaced under <org>/<initiative>/.
	wantRel := "acme/onboarding/task-00001-knowledge.md"
	if len(res.PagesWritten) != 1 || res.PagesWritten[0] != wantRel {
		t.Fatalf("PagesWritten = %v, want [%s]", res.PagesWritten, wantRel)
	}
	page := readFile(t, filepath.Join(outDir, filepath.FromSlash(wantRel)))
	for _, want := range []string{
		"org: acme", "initiative: onboarding",
		"## Related (graph)", "relates_to", "`TASK-00002`",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("ticket page missing %q\n---\n%s", want, page)
		}
	}

	// Navigation artifacts exist.
	for _, nav := range []string{"index.md", "llms.txt", "AGENTS.md", "tags/security.md", "initiatives/onboarding.md"} {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(nav))); err != nil {
			t.Errorf("expected nav artifact %s: %v", nav, err)
		}
	}
	// index.md links to the tag + initiative index pages.
	index := readFile(t, filepath.Join(outDir, "index.md"))
	for _, want := range []string{"tags/security.md", "initiatives/onboarding.md", wantRel} {
		if !strings.Contains(index, want) {
			t.Errorf("index.md missing link %q\n---\n%s", want, index)
		}
	}
	// A tag page links back UP to the namespaced ticket page.
	tagPage := readFile(t, filepath.Join(outDir, "tags", "security.md"))
	if !strings.Contains(tagPage, "../"+wantRel) {
		t.Errorf("tags/security.md should link ../%s\n---\n%s", wantRel, tagPage)
	}

	// The page was indexed for semantic search under ns wiki/<task-id>.
	if res.Indexed != 1 {
		t.Fatalf("Indexed = %d, want 1", res.Indexed)
	}
	found := false
	for _, c := range idx.calls {
		if c.ns == "wiki/TASK-00001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an upsert to ns wiki/TASK-00001, got %+v", idx.calls)
	}
}

// TestWikiPublisher_NilSeamsStayFlat confirms the additive design: with no
// graph/classifier/indexer, ticket pages stay flat and no cross-links appear,
// but the tag index + entrypoints are still emitted.
func TestWikiPublisher_NilSeamsStayFlat(t *testing.T) {
	base := t.TempDir()
	seedKnowledge(t, base, "TASK-00042", &models.ExtractedKnowledge{
		TaskID:    "TASK-00042",
		Decisions: []models.Decision{{ID: "D", Title: "X", Status: "accepted"}},
	})
	outDir := filepath.Join(base, "out")
	res, err := newTestPublisher(base).PublishAll(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PagesWritten) != 1 || res.PagesWritten[0] != "task-00042-knowledge.md" {
		t.Fatalf("PagesWritten = %v, want flat [task-00042-knowledge.md]", res.PagesWritten)
	}
	if res.Indexed != 0 {
		t.Fatalf("Indexed = %d, want 0 (no indexer wired)", res.Indexed)
	}
	page := readFile(t, filepath.Join(outDir, "task-00042-knowledge.md"))
	if strings.Contains(page, "## Related (graph)") {
		t.Error("no graph wired → no Related section expected")
	}
	// Entrypoints still emitted.
	for _, nav := range []string{"index.md", "llms.txt", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(outDir, nav)); err != nil {
			t.Errorf("expected %s even with nil seams: %v", nav, err)
		}
	}
}
