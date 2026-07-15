package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fixedTime makes the publisher deterministic in tests.
func fixedTime() time.Time { return time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC) }

// seedKnowledge writes a decisions.yaml for taskID under basePath.
func seedKnowledge(t *testing.T, basePath, taskID string, k *models.ExtractedKnowledge) {
	t.Helper()
	ke := NewKnowledgeExtractor(basePath)
	if err := ke.SaveKnowledge(taskID, k); err != nil {
		t.Fatalf("SaveKnowledge(%s): %v", taskID, err)
	}
}

func newTestPublisher(basePath string) *WikiPublisher {
	p := NewWikiPublisher(basePath)
	p.now = fixedTime
	return p
}

func TestWikiPublisher_PublishAll(t *testing.T) {
	base := t.TempDir()

	seedKnowledge(t, base, "TASK-00001", &models.ExtractedKnowledge{
		TaskID:  "TASK-00001",
		Summary: "Auth rework.",
		Decisions: []models.Decision{{
			ID: "D1", Title: "Use JWT", Description: "Stateless sessions.",
			Status: "accepted", Rationale: "Scales horizontally.",
			Alternatives: []string{"Server sessions"}, Consequences: []string{"Token revocation is harder"},
		}},
		Learnings: []models.Learning{{Title: "pgx beats lib/pq", Description: "3x throughput.", Category: "technical"}},
		Gotchas:   []models.Gotcha{{Title: "Clock skew", Description: "JWT exp fails.", Solution: "NTP", Severity: "high"}},
	})
	// A task with no knowledge entries — must be skipped, not written empty.
	seedKnowledge(t, base, "TASK-00002", &models.ExtractedKnowledge{TaskID: "TASK-00002"})

	outDir := filepath.Join(base, "wiki-out")
	res, err := newTestPublisher(base).PublishAll(outDir)
	if err != nil {
		t.Fatalf("PublishAll: %v", err)
	}

	if res.TasksScanned != 2 {
		t.Errorf("TasksScanned = %d, want 2", res.TasksScanned)
	}
	if len(res.PagesWritten) != 1 || res.PagesWritten[0] != "task-00001-knowledge.md" {
		t.Errorf("PagesWritten = %v, want [task-00001-knowledge.md]", res.PagesWritten)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "TASK-00002" {
		t.Errorf("Skipped = %v, want [TASK-00002]", res.Skipped)
	}

	page, err := os.ReadFile(filepath.Join(outDir, "task-00001-knowledge.md"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	content := string(page)

	// Frontmatter assertions.
	for _, want := range []string{
		"title: TASK-00001 Knowledge",
		"created: 2026-06-07",
		"updated: 2026-06-07",
		"tags: [adb, knowledge, task-knowledge]",
		"source: tickets/TASK-00001/knowledge/decisions.yaml",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("page missing frontmatter line %q\n---\n%s", want, content)
		}
	}

	// Body assertions: each knowledge kind and its detail fields rendered.
	for _, want := range []string{
		"## Decisions",
		"### Use JWT `accepted`",
		"**Rationale:** Scales horizontally.",
		"- Server sessions",
		"- Token revocation is harder",
		"## Learnings",
		"### pgx beats lib/pq `technical`",
		"## Gotchas",
		"### Clock skew `high`",
		"**Solution:** NTP",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("page missing %q\n---\n%s", want, content)
		}
	}
}

func TestWikiPublisher_EmptyWorkspace(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "out")
	res, err := newTestPublisher(base).PublishAll(outDir)
	if err != nil {
		t.Fatalf("PublishAll on empty workspace: %v", err)
	}
	if res.TasksScanned != 0 || len(res.PagesWritten) != 0 {
		t.Errorf("expected nothing published, got %+v", res)
	}
}

func TestWikiPublisher_RequiresOutDir(t *testing.T) {
	if _, err := newTestPublisher(t.TempDir()).PublishAll(""); err == nil {
		t.Error("expected error for empty outDir")
	}
}

func TestWikiPublisher_OutputIsValidFrontmatter(t *testing.T) {
	base := t.TempDir()
	seedKnowledge(t, base, "TASK-00009", &models.ExtractedKnowledge{
		TaskID:    "TASK-00009",
		Decisions: []models.Decision{{ID: "D", Title: "Minimal", Status: "proposed"}},
	})
	outDir := filepath.Join(base, "out")
	if _, err := newTestPublisher(base).PublishAll(outDir); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(outDir, "task-00009-knowledge.md"))
	content := string(page)
	// Frontmatter must open and close before any heading.
	if !strings.HasPrefix(content, "---\n") {
		t.Error("page must start with frontmatter delimiter")
	}
	if strings.Count(content, "---\n") < 2 {
		t.Error("frontmatter must be closed with a second delimiter")
	}
	if idx := strings.Index(content, "# TASK-00009"); idx != -1 {
		if fmEnd := strings.Index(content[4:], "---\n"); fmEnd == -1 || (fmEnd+4) > idx {
			t.Error("heading must come after closed frontmatter")
		}
	}
}
