package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ---- fakes ----

type fakeRuleStore struct {
	set models.RuleSet
}

func (f *fakeRuleStore) Load() (models.RuleSet, error) { return f.set, nil }
func (f *fakeRuleStore) Save(rs models.RuleSet) error  { f.set = rs; return nil }

type recordedExec struct {
	args []string
}

type recordedSkill struct {
	rule, skill string
	payload     map[string]string
}

type fakeRunner struct {
	execs  []recordedExec
	skills []recordedSkill
	err    error
}

func (f *fakeRunner) RunExec(_ context.Context, args []string) (string, error) {
	f.execs = append(f.execs, recordedExec{args: args})
	return "exec-ran", f.err
}

func (f *fakeRunner) RecordSkillRequest(rule, skill string, payload map[string]string) (string, error) {
	f.skills = append(f.skills, recordedSkill{rule: rule, skill: skill, payload: payload})
	return "skill-recorded", f.err
}

type recordedEdge struct {
	from string
	link models.Link
}

type fakeEdgeWriter struct {
	edges []recordedEdge
	// failFrom, when set, makes AddEdge return an error for that source entity
	// (simulating an edge whose from-entity doesn't exist yet). Zero value =
	// never fail, so existing users are unaffected.
	failFrom string
}

func (f *fakeEdgeWriter) AddEdge(from string, link models.Link) error {
	if f.failFrom != "" && from == f.failFrom {
		return fmt.Errorf("cannot add edge: no task or initiative %q", from)
	}
	f.edges = append(f.edges, recordedEdge{from: from, link: link})
	return nil
}

type recordedArtifact struct {
	path, content string
}

type fakeArtifactWriter struct {
	artifacts []recordedArtifact
}

func (f *fakeArtifactWriter) WriteArtifact(relPath, content string) error {
	f.artifacts = append(f.artifacts, recordedArtifact{path: relPath, content: content})
	return nil
}

// fakeGraph implements core.GraphManager, returning configured neighbours.
type fakeGraph struct {
	neighbors map[string][]models.GraphEdge
}

func (f *fakeGraph) Graph() (*Graph, error)   { return buildGraph(nil), nil }
func (f *fakeGraph) Rebuild() (*Graph, error) { return buildGraph(nil), nil }
func (f *fakeGraph) Neighbors(id string) ([]models.GraphEdge, error) {
	return f.neighbors[id], nil
}
func (f *fakeGraph) NeighborsByType(id string, t models.EdgeType) ([]models.GraphEdge, error) {
	var out []models.GraphEdge
	for _, e := range f.neighbors[id] {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func newTestEngine(set models.RuleSet, graph GraphManager) (*fakeRunner, *fakeEdgeWriter, *fakeArtifactWriter, RuleEngine) {
	runner := &fakeRunner{}
	edges := &fakeEdgeWriter{}
	artifacts := &fakeArtifactWriter{}
	eng := NewRuleEngine(&fakeRuleStore{set: set}, graph, runner, edges, artifacts,
		WithRuleClock(func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) }))
	return runner, edges, artifacts, eng
}

// ---- tests ----

func TestRuleEngine_TimeRules_FilterEnabledSchedules(t *testing.T) {
	off := false
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "sched-on", On: models.RuleTrigger{Schedule: "1m"}, Run: models.RuleAction{Skill: "a"}},
		{Name: "sched-off", Enabled: &off, On: models.RuleTrigger{Schedule: "1m"}, Run: models.RuleAction{Skill: "b"}},
		{Name: "evt", On: models.RuleTrigger{Event: "task.created"}, Run: models.RuleAction{Skill: "c"}},
	}}
	_, _, _, eng := newTestEngine(set, nil)
	tr, err := eng.TimeRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 1 || tr[0].Name != "sched-on" {
		t.Fatalf("TimeRules() = %+v, want only sched-on", tr)
	}
}

func TestRuleEngine_FireByName_SkillRecorded(t *testing.T) {
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "nightly", On: models.RuleTrigger{Schedule: "15m"}, Run: models.RuleAction{Skill: "repos-pull"}},
	}}
	runner, _, _, eng := newTestEngine(set, nil)
	f, err := eng.FireByName(context.Background(), "nightly", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != FiringFired {
		t.Fatalf("status = %q (%s), want fired", f.Status, f.Reason)
	}
	if len(runner.skills) != 1 || runner.skills[0].skill != "repos-pull" {
		t.Fatalf("skills recorded = %+v", runner.skills)
	}
}

func TestRuleEngine_FireByName_UnknownRule(t *testing.T) {
	_, _, _, eng := newTestEngine(models.RuleSet{}, nil)
	if _, err := eng.FireByName(context.Background(), "ghost", nil); err == nil {
		t.Fatal("expected error for unknown rule, got nil")
	}
}

func TestRuleEngine_Dispatch_EventMatchAndExec(t *testing.T) {
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "on-status", On: models.RuleTrigger{Event: "task.status_changed"}, Run: models.RuleAction{Exec: []string{"echo", "{{.task_id}}"}}},
		{Name: "on-create", On: models.RuleTrigger{Event: "task.created"}, Run: models.RuleAction{Skill: "greet"}},
	}}
	runner, _, _, eng := newTestEngine(set, nil)
	firings, err := eng.Dispatch(context.Background(), "task.status_changed", map[string]string{"task_id": "TASK-42"})
	if err != nil {
		t.Fatal(err)
	}
	if len(firings) != 1 || firings[0].Status != FiringFired {
		t.Fatalf("firings = %+v, want one fired", firings)
	}
	if len(runner.execs) != 1 {
		t.Fatalf("execs = %+v, want one", runner.execs)
	}
	if got := runner.execs[0].args; len(got) != 2 || got[1] != "TASK-42" {
		t.Fatalf("exec args = %v, want [echo TASK-42] (template expanded)", got)
	}
}

func TestRuleEngine_Condition_Gates(t *testing.T) {
	graph := &fakeGraph{neighbors: map[string][]models.GraphEdge{
		"TASK-1": {{From: "TASK-1", Type: models.EdgeDependsOn, To: "TASK-9"}},
		"TASK-2": {{From: "TASK-2", Type: models.EdgeRelatesTo, To: "TASK-8"}},
	}}
	set := models.RuleSet{Rules: []models.Rule{
		{
			Name: "flag-blocked",
			On:   models.RuleTrigger{Event: "task.status_changed"},
			If:   &models.RuleCondition{Entity: "{{.task_id}}", HasEdge: models.EdgeDependsOn},
			Run:  models.RuleAction{Skill: "triage"},
		},
	}}
	runner, _, _, eng := newTestEngine(set, graph)

	// Entity with a depends_on edge → condition holds → fires.
	f, _ := eng.Dispatch(context.Background(), "task.status_changed", map[string]string{"task_id": "TASK-1"})
	if len(f) != 1 || f[0].Status != FiringFired {
		t.Fatalf("TASK-1 firing = %+v, want fired", f)
	}
	// Entity without a depends_on edge → condition fails → skipped.
	f, _ = eng.Dispatch(context.Background(), "task.status_changed", map[string]string{"task_id": "TASK-2"})
	if len(f) != 1 || f[0].Status != FiringSkipped {
		t.Fatalf("TASK-2 firing = %+v, want skipped", f)
	}
	if len(runner.skills) != 1 {
		t.Fatalf("skills recorded = %d, want 1 (only TASK-1)", len(runner.skills))
	}
}

func TestRuleEngine_Condition_NoGraphSkips(t *testing.T) {
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "needs-graph", On: models.RuleTrigger{Event: "task.created"}, If: &models.RuleCondition{Entity: "TASK-1", HasEdge: models.EdgeDependsOn}, Run: models.RuleAction{Skill: "s"}},
	}}
	_, _, _, eng := newTestEngine(set, nil) // no graph wired
	f, _ := eng.Dispatch(context.Background(), "task.created", nil)
	if len(f) != 1 || f[0].Status != FiringSkipped {
		t.Fatalf("firing = %+v, want skipped (no graph)", f)
	}
}

func TestRuleEngine_Outputs_EdgeAndArtifact(t *testing.T) {
	set := models.RuleSet{Rules: []models.Rule{
		{
			Name: "link-and-record",
			On:   models.RuleTrigger{Event: "task.status_changed"},
			Run:  models.RuleAction{Skill: "s"},
			Write: []models.RuleOutput{
				{Artifact: "reports/{{.task_id}}.md"},
				{Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "INIT-1"}, EdgeFrom: "{{.task_id}}"},
			},
		},
	}}
	_, edges, artifacts, eng := newTestEngine(set, nil)
	f, _ := eng.Dispatch(context.Background(), "task.status_changed", map[string]string{"task_id": "TASK-7"})
	if len(f) != 1 || f[0].Status != FiringFired {
		t.Fatalf("firing = %+v, want fired", f)
	}
	if len(artifacts.artifacts) != 1 || artifacts.artifacts[0].path != "reports/TASK-7.md" {
		t.Fatalf("artifacts = %+v", artifacts.artifacts)
	}
	if len(edges.edges) != 1 {
		t.Fatalf("edges = %+v, want one", edges.edges)
	}
	got := edges.edges[0]
	if got.from != "TASK-7" || got.link.Type != models.EdgeRelatesTo || got.link.Target != "INIT-1" {
		t.Fatalf("edge = %+v, want TASK-7 --relates_to--> INIT-1", got)
	}
}

func TestRuleEngine_DisabledSkips(t *testing.T) {
	off := false
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "parked", Enabled: &off, On: models.RuleTrigger{Event: "task.created"}, Run: models.RuleAction{Skill: "s"}},
	}}
	runner, _, _, eng := newTestEngine(set, nil)
	f, _ := eng.Dispatch(context.Background(), "task.created", nil)
	if len(f) != 1 || f[0].Status != FiringSkipped {
		t.Fatalf("firing = %+v, want skipped (disabled)", f)
	}
	if len(runner.skills) != 0 {
		t.Fatal("disabled rule should not record a skill")
	}
}

func TestRuleEngine_UnresolvedTemplateSkips(t *testing.T) {
	graph := &fakeGraph{neighbors: map[string][]models.GraphEdge{}}
	set := models.RuleSet{Rules: []models.Rule{
		{Name: "needs-taskid", On: models.RuleTrigger{Event: "task.created"}, If: &models.RuleCondition{Entity: "{{.task_id}}", HasEdge: models.EdgeDependsOn}, Run: models.RuleAction{Skill: "s"}},
	}}
	_, _, _, eng := newTestEngine(set, graph)
	// Event without task_id → condition entity unresolved → skip (not a crash).
	f, _ := eng.Dispatch(context.Background(), "task.created", map[string]string{"other": "x"})
	if len(f) != 1 || f[0].Status != FiringSkipped {
		t.Fatalf("firing = %+v, want skipped (unresolved template)", f)
	}
}
