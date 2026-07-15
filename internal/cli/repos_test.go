package cli

import (
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestBuildRepoRegistry(t *testing.T) {
	tasks := []models.Task{
		{ID: "TASK-3", Repo: "github.com/o/r2", Status: models.TaskStatusInProgress},
		{ID: "TASK-1", Repo: "github.com/o/r1", Status: models.TaskStatusInProgress},
		{ID: "TASK-2", Repo: "github.com/o/r1", Status: models.TaskStatusBacklog},
		{ID: "TASK-4", Repo: "github.com/o/r2", Status: models.TaskStatusArchived}, // excluded
		{ID: "TASK-5", Repo: "", Status: models.TaskStatusInProgress},              // excluded (no repo)
	}
	got := buildRepoRegistry(tasks)
	want := []repoRegistryEntry{
		{Repo: "github.com/o/r1", Tickets: []string{"TASK-1", "TASK-2"}},
		{Repo: "github.com/o/r2", Tickets: []string{"TASK-3"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildRepoRegistry =\n %+v\nwant\n %+v", got, want)
	}
}

func TestDistinctRepos_ByInitiative(t *testing.T) {
	tasks := []models.Task{
		{ID: "T1", Repo: "github.com/o/r1", Initiative: "widget", Status: models.TaskStatusInProgress},
		{ID: "T2", Repo: "github.com/o/r2", Initiative: "widget", Status: models.TaskStatusInProgress},
		{ID: "T3", Repo: "github.com/o/r1", Initiative: "widget", Status: models.TaskStatusInProgress}, // dup repo
		{ID: "T4", Repo: "github.com/o/r3", Initiative: "other", Status: models.TaskStatusInProgress},  // other initiative
		{ID: "T5", Repo: "github.com/o/r4", Initiative: "widget", Status: models.TaskStatusArchived},   // archived
	}
	got := distinctRepos(tasks, func(t models.Task) bool { return t.Initiative == "widget" })
	want := []string{"github.com/o/r1", "github.com/o/r2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("distinctRepos(widget) = %v, want %v", got, want)
	}
}

func TestDistinctRepos_ByTicket(t *testing.T) {
	tasks := []models.Task{
		{ID: "T1", Repo: "github.com/o/r1", Status: models.TaskStatusInProgress},
		{ID: "T2", Repo: "github.com/o/r2", Status: models.TaskStatusInProgress},
	}
	got := distinctRepos(tasks, func(t models.Task) bool { return t.ID == "T2" })
	if !reflect.DeepEqual(got, []string{"github.com/o/r2"}) {
		t.Errorf("distinctRepos(T2) = %v, want [github.com/o/r2]", got)
	}
}
