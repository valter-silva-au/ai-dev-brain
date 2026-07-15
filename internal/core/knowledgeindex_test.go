package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type upsertCall struct {
	ns, key, content string
	meta             map[string]string
}

type fakeMemIndexer struct {
	calls []upsertCall
}

func (f *fakeMemIndexer) Upsert(_ context.Context, ns, key, content string, meta map[string]string) error {
	f.calls = append(f.calls, upsertCall{ns: ns, key: key, content: content, meta: meta})
	return nil
}

// fakeBacklogStore implements core.BacklogStore; only Load is exercised here.
type fakeBacklogStore struct {
	backlog *models.Backlog
}

func (f *fakeBacklogStore) Load() (*models.Backlog, error)       { return f.backlog, nil }
func (f *fakeBacklogStore) Save(*models.Backlog) error           { return nil }
func (f *fakeBacklogStore) AddTask(models.Task) error            { return nil }
func (f *fakeBacklogStore) UpdateTask(models.Task) error         { return nil }
func (f *fakeBacklogStore) GetTask(string) (*models.Task, error) { return nil, nil }
func (f *fakeBacklogStore) RemoveTask(string) error              { return nil }

func TestKnowledgeIndexer_IndexWorkspace(t *testing.T) {
	dir := t.TempDir()
	ticketDir := filepath.Join(dir, "tickets", "_local", "TASK-00001-thing")
	if err := os.MkdirAll(filepath.Join(ticketDir, "knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Two non-empty knowledge files; design.md is absent (skipped).
	if err := os.WriteFile(filepath.Join(ticketDir, "context.md"), []byte("the problem"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "notes.md"), []byte("some notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An empty file must be skipped, not indexed.
	if err := os.WriteFile(filepath.Join(ticketDir, "knowledge", "decisions.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	backlog := &models.Backlog{Tasks: []models.Task{{ID: "TASK-00001", TicketPath: ticketDir}}}
	graph := NewGraphManager(&fakeGraphSource{nodes: []GraphNode{
		{ID: "TASK-00001", Links: []models.Link{{Type: models.EdgeDependsOn, Target: "TASK-00002"}}},
	}}, nil)

	mem := &fakeMemIndexer{}
	ki := NewKnowledgeIndexer(mem, &fakeBacklogStore{backlog: backlog}, graph, filepath.Join(dir, "tickets"))

	stats, err := ki.IndexWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Tickets != 1 || stats.Files != 2 || stats.Edges != 1 {
		t.Fatalf("stats = %+v, want {Tickets:1 Files:2 Edges:1}", stats)
	}

	var ticketNS, graphNS int
	sawContext := false
	for _, c := range mem.calls {
		switch c.ns {
		case "tickets/TASK-00001":
			ticketNS++
			if c.key == "context.md" && c.content == "the problem" {
				sawContext = true
			}
		case "graph":
			graphNS++
			if c.content != "TASK-00001 depends_on TASK-00002" {
				t.Fatalf("edge content = %q", c.content)
			}
		}
	}
	if ticketNS != 2 || !sawContext {
		t.Fatalf("ticket upserts = %d (sawContext=%v), want 2 incl. context.md", ticketNS, sawContext)
	}
	if graphNS != 1 {
		t.Fatalf("graph upserts = %d, want 1", graphNS)
	}
}

func TestKnowledgeIndexer_NilMemory(t *testing.T) {
	ki := NewKnowledgeIndexer(nil, &fakeBacklogStore{backlog: &models.Backlog{}}, nil, "")
	if _, err := ki.IndexWorkspace(context.Background()); err == nil {
		t.Fatal("expected error when no memory indexer is wired")
	}
}
