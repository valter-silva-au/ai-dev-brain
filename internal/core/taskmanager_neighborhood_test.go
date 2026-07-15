package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// fakeInitiativeResolver (with GetInitiative) is declared in
// taskmanager_initiative_test.go and reused here.

// fakeNeighborResolver returns canned edges per id for the neighbourhood tests.
type fakeNeighborResolver struct {
	edges map[string][]models.GraphEdge
	err   error
}

func (f *fakeNeighborResolver) Neighbors(id string) ([]models.GraphEdge, error) {
	return f.edges[id], f.err
}

// TestNeighborhoodSiblings_FormatAndDirection checks incident edges render with
// direction arrows relative to the ticket: outgoing `→`, incoming `←`.
func TestNeighborhoodSiblings_FormatAndDirection(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	tm.SetNeighborResolver(&fakeNeighborResolver{edges: map[string][]models.GraphEdge{
		"TASK-00001": {
			{From: "TASK-00001", Type: models.EdgeDependsOn, To: "TASK-00002"},
			{From: "TASK-00003", Type: models.EdgeBlocks, To: "TASK-00001"},
		},
	}})

	got := tm.neighborhoodSiblings("TASK-00001")
	want := []string{"→ TASK-00002 (depends_on)", "← TASK-00003 (blocks)"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

// TestNeighborhoodSiblings_Cap bounds the list and appends a "more" pointer.
func TestNeighborhoodSiblings_Cap(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	var edges []models.GraphEdge
	for i := 0; i < maxContextNeighbors+5; i++ {
		edges = append(edges, models.GraphEdge{From: "TASK-00001", Type: models.EdgeRelatesTo, To: "TASK-0000" + string(rune('A'+i))})
	}
	tm.SetNeighborResolver(&fakeNeighborResolver{edges: map[string][]models.GraphEdge{"TASK-00001": edges}})

	got := tm.neighborhoodSiblings("TASK-00001")
	if len(got) != maxContextNeighbors+1 {
		t.Fatalf("got %d lines, want %d (cap + 1 more-line)", len(got), maxContextNeighbors+1)
	}
	if !strings.Contains(got[len(got)-1], "and 5 more") {
		t.Errorf("last line %q should note the 5 elided neighbours", got[len(got)-1])
	}
}

// TestNeighborhoodSiblings_NilAndEmpty: no resolver, or a resolver with no
// edges, yields nil so the context renders byte-identically to before.
func TestNeighborhoodSiblings_NilAndEmpty(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	if got := tm.neighborhoodSiblings("TASK-00001"); got != nil {
		t.Errorf("nil resolver should yield nil, got %v", got)
	}
	tm.SetNeighborResolver(&fakeNeighborResolver{edges: map[string][]models.GraphEdge{}})
	if got := tm.neighborhoodSiblings("TASK-00001"); got != nil {
		t.Errorf("no edges should yield nil, got %v", got)
	}
}

// realTemplateTaskManager builds a TaskManager backed by the REAL embedded
// task-context.md template (not the canned mock), so a test can assert the
// rendered Tier-0 file content.
func realTemplateTaskManager(t *testing.T) (*TaskManager, string) {
	t.Helper()
	tempDir := t.TempDir()
	tmpl, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager: %v", err)
	}
	tm := NewTaskManager(
		NewMockBacklogStore(), NewMockContextStore(), NewMockWorktreeCreator(),
		NewMockWorktreeRemover(), NewMockEventLogger(), NewMockSessionCapturer(),
		NewMockTerminalStateUpdater(), NewMockTaskIDGenerator("TASK"), tmpl,
		filepath.Join(tempDir, "tickets"), filepath.Join(tempDir, "_archived"),
		filepath.Join(tempDir, "work"),
	)
	return tm, tempDir
}

func readWorktreeContext(t *testing.T, task *models.Task) string {
	t.Helper()
	p := filepath.Join(task.WorktreePath, ".claude", "rules", "task-context.md")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read worktree task-context.md: %v", err)
	}
	return string(data)
}

// TestWorktreeContext_RendersNeighbourhood: after resume, a ticket with graph
// edges renders its 1-hop neighbourhood into the Tier-0 "Related tickets" block.
func TestWorktreeContext_RendersNeighbourhood(t *testing.T) {
	tm, _ := realTemplateTaskManager(t)
	tm.SetNeighborResolver(&fakeNeighborResolver{edges: map[string][]models.GraphEdge{
		"TASK-00001": {
			{From: "TASK-00001", Type: models.EdgeDependsOn, To: "TASK-00002"},
			{From: "TASK-00003", Type: models.EdgeRelatesTo, To: "TASK-00001"},
		},
	}})

	task, err := tm.Create(CreateTaskOpts{Title: "graphy task", TaskType: models.TaskTypeFeat, Repo: "github.com/acme/widgets"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := tm.Resume(task.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	content := readWorktreeContext(t, mustGetTask(t, tm, task.ID))
	if !strings.Contains(content, "→ TASK-00002 (depends_on)") {
		t.Errorf("neighbourhood missing outgoing edge:\n%s", content)
	}
	if !strings.Contains(content, "← TASK-00003 (relates_to)") {
		t.Errorf("neighbourhood missing incoming edge:\n%s", content)
	}
}

// TestWorktreeContext_NoLinks_Unchanged: a ticket with no graph edges renders
// the original "No siblings resolved yet" fallback — no regression.
func TestWorktreeContext_NoLinks_Unchanged(t *testing.T) {
	tm, _ := realTemplateTaskManager(t)
	tm.SetNeighborResolver(&fakeNeighborResolver{edges: map[string][]models.GraphEdge{}}) // resolver present, no edges

	task, err := tm.Create(CreateTaskOpts{Title: "lonely task", TaskType: models.TaskTypeFeat, Repo: "github.com/acme/widgets"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := tm.Resume(task.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	content := readWorktreeContext(t, mustGetTask(t, tm, task.ID))
	if !strings.Contains(content, "No siblings resolved yet") {
		t.Errorf("no-link ticket should render the sibling fallback:\n%s", content)
	}
	if strings.Contains(content, "→ TASK") || strings.Contains(content, "← TASK") {
		t.Errorf("no-link ticket should render no neighbour arrows:\n%s", content)
	}
}

// TestWorktreeContext_RendersInitiativeStageGate: a ticket associated with an
// initiative that has a gate renders the initiative/stage/gate lines (issue
// #112 — the seed carries initiative/stage/gate).
func TestWorktreeContext_RendersInitiativeStageGate(t *testing.T) {
	tm, _ := realTemplateTaskManager(t)
	tm.SetInitiativeResolver(&fakeInitiativeResolver{inits: map[string]models.Initiative{
		"widget-launch": {
			ID: "widget-launch", Name: "Widget Launch", OrgID: "acme", Stage: models.StageMVP,
			Gate: &models.GateState{Transition: "Idea->MVP", Passed: true},
		},
	}})

	task, err := tm.Create(CreateTaskOpts{Title: "gated task", TaskType: models.TaskTypeFeat, Repo: "github.com/acme/widgets", Initiative: "widget-launch"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	content := readWorktreeContext(t, mustGetTask(t, tm, task.ID))
	if !strings.Contains(content, "**Stage:** MVP") {
		t.Errorf("missing stage line:\n%s", content)
	}
	if !strings.Contains(content, "**Gate:** Idea->MVP (passed)") {
		t.Errorf("missing gate line:\n%s", content)
	}
}

// TestWorktreeContext_NoGate_OmitsGateLine: an initiative with no gate renders
// no Gate line (guarded — pre-gate initiatives are unchanged).
func TestWorktreeContext_NoGate_OmitsGateLine(t *testing.T) {
	tm, _ := realTemplateTaskManager(t)
	tm.SetInitiativeResolver(&fakeInitiativeResolver{inits: map[string]models.Initiative{
		"early": {ID: "early", Name: "Early", OrgID: "acme", Stage: models.StageIdea}, // Gate nil
	}})

	task, err := tm.Create(CreateTaskOpts{Title: "ungated task", TaskType: models.TaskTypeFeat, Repo: "github.com/acme/widgets", Initiative: "early"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	content := readWorktreeContext(t, mustGetTask(t, tm, task.ID))
	if !strings.Contains(content, "**Stage:** Idea") {
		t.Errorf("missing stage line:\n%s", content)
	}
	if strings.Contains(content, "**Gate:**") {
		t.Errorf("no-gate initiative should render no Gate line:\n%s", content)
	}
}

func mustGetTask(t *testing.T, tm *TaskManager, id string) *models.Task {
	t.Helper()
	task, err := tm.backlogStore.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return task
}
