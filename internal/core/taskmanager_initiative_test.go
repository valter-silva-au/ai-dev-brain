package core

import (
	"fmt"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeInitiativeResolver is an in-memory InitiativeResolver for unit-testing the
// ticket↔initiative association rules without a real StageManager/store.
type fakeInitiativeResolver struct {
	inits map[string]models.Initiative
}

func (f *fakeInitiativeResolver) GetInitiative(id string) (models.Initiative, error) {
	init, ok := f.inits[id]
	if !ok {
		return models.Initiative{}, fmt.Errorf("initiative %q not found", id)
	}
	return init, nil
}

func newFakeResolver() *fakeInitiativeResolver {
	return &fakeInitiativeResolver{inits: map[string]models.Initiative{
		"widget-launcher": {ID: "widget-launcher", Name: "Widget Launcher", OrgID: "acme", Stage: models.StageMVP},
	}}
}

// TestTaskManager_Create_WithInitiative validates the happy path: a known
// initiative is accepted and persisted on the task (issue #88 AC 1).
func TestTaskManager_Create_WithInitiative(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)
	tm.SetInitiativeResolver(newFakeResolver())

	task, err := tm.Create(CreateTaskOpts{
		Title:      "Build the widget",
		TaskType:   models.TaskTypeFeat,
		Initiative: "widget-launcher",
	})
	if err != nil {
		t.Fatalf("Create with initiative: %v", err)
	}
	if task.Initiative != "widget-launcher" {
		t.Errorf("task.Initiative = %q, want widget-launcher", task.Initiative)
	}

	// The association must survive a reload from the store.
	stored, err := backlogStore.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Initiative != "widget-launcher" {
		t.Errorf("persisted initiative = %q, want widget-launcher", stored.Initiative)
	}
}

// TestTaskManager_Create_UnknownInitiative_Errors verifies an unknown initiative
// is rejected before any task is minted.
func TestTaskManager_Create_UnknownInitiative_Errors(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)
	tm.SetInitiativeResolver(newFakeResolver())

	if _, err := tm.Create(CreateTaskOpts{Title: "Orphan", Initiative: "ghost"}); err == nil {
		t.Fatal("Create with unknown initiative should error")
	}
	// No partially-created task should linger.
	backlog, err := backlogStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(backlog.Tasks) != 0 {
		t.Errorf("expected no tasks after failed create, got %d", len(backlog.Tasks))
	}
}

// TestTaskManager_Create_NoInitiative_Unchanged verifies existing behaviour is
// preserved when no initiative is supplied (issue #88 AC 3) — even with no
// resolver wired at all.
func TestTaskManager_Create_NoInitiative_Unchanged(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	// Deliberately no resolver set.

	task, err := tm.Create(CreateTaskOpts{Title: "Plain task", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create without initiative: %v", err)
	}
	if task.Initiative != "" {
		t.Errorf("task.Initiative = %q, want empty", task.Initiative)
	}
}

// TestTaskManager_SetInitiative covers set / validate / clear on an existing
// task (issue #88 — the `adb task update --initiative` path).
func TestTaskManager_SetInitiative(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)
	tm.SetInitiativeResolver(newFakeResolver())

	task, err := tm.Create(CreateTaskOpts{Title: "Widget task", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set a valid initiative.
	if _, err := tm.SetInitiative(task.ID, "widget-launcher"); err != nil {
		t.Fatalf("SetInitiative(valid): %v", err)
	}
	if got, _ := backlogStore.GetTask(task.ID); got.Initiative != "widget-launcher" {
		t.Errorf("after set, initiative = %q, want widget-launcher", got.Initiative)
	}

	// An unknown initiative is rejected and leaves the association intact.
	if _, err := tm.SetInitiative(task.ID, "ghost"); err == nil {
		t.Error("SetInitiative(unknown) should error")
	}
	if got, _ := backlogStore.GetTask(task.ID); got.Initiative != "widget-launcher" {
		t.Errorf("failed set should not change association, got %q", got.Initiative)
	}

	// Clearing (empty id) is allowed and skips validation.
	if _, err := tm.SetInitiative(task.ID, ""); err != nil {
		t.Fatalf("SetInitiative(clear): %v", err)
	}
	if got, _ := backlogStore.GetTask(task.ID); got.Initiative != "" {
		t.Errorf("after clear, initiative = %q, want empty", got.Initiative)
	}
}
