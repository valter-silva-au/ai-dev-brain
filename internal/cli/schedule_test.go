package cli

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestBuildRuleFromFlags_TimeSkill(t *testing.T) {
	r, err := buildRuleFromFlags("nightly", "15m", "", "", "", "repos-pull", "", nil, nil, "", false)
	if err != nil {
		t.Fatalf("buildRuleFromFlags error = %v", err)
	}
	if r.Name != "nightly" || r.On.Schedule != "15m" || r.Run.Skill != "repos-pull" {
		t.Fatalf("unexpected rule: %+v", r)
	}
	if !r.IsEnabled() {
		t.Fatal("rule should be enabled by default")
	}
}

func TestBuildRuleFromFlags_EventConditionEdgeOutput(t *testing.T) {
	r, err := buildRuleFromFlags(
		"flag-blocked", "", "task.status_changed",
		"{{.task_id}}", "depends_on",
		"triage", "",
		[]string{"relates_to:INIT-1"}, []string{"reports/{{.task_id}}.md"},
		"{{.task_id}}", false,
	)
	if err != nil {
		t.Fatalf("buildRuleFromFlags error = %v", err)
	}
	if r.On.Event != "task.status_changed" {
		t.Fatalf("event = %q", r.On.Event)
	}
	if r.If == nil || r.If.HasEdge != models.EdgeDependsOn {
		t.Fatalf("condition = %+v", r.If)
	}
	if len(r.Write) != 2 {
		t.Fatalf("want 2 outputs, got %d", len(r.Write))
	}
	// StringArray order: artifacts appended before edges in buildRuleFromFlags.
	if r.Write[0].Artifact != "reports/{{.task_id}}.md" {
		t.Fatalf("output[0] = %+v", r.Write[0])
	}
	if r.Write[1].Edge == nil || r.Write[1].Edge.Type != models.EdgeRelatesTo || r.Write[1].Edge.Target != "INIT-1" {
		t.Fatalf("output[1] = %+v", r.Write[1])
	}
	if r.Write[1].EdgeFrom != "{{.task_id}}" {
		t.Fatalf("edge_from = %q", r.Write[1].EdgeFrom)
	}
}

func TestBuildRuleFromFlags_Disabled(t *testing.T) {
	r, err := buildRuleFromFlags("parked", "1h", "", "", "", "s", "", nil, nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.IsEnabled() {
		t.Fatal("--disabled should park the rule")
	}
}

func TestBuildRuleFromFlags_Rejections(t *testing.T) {
	cases := []struct {
		name                                                      string
		rname, every, event, ifEnt, ifEdge, skill, exec, edgeFrom string
		edges, arts                                               []string
	}{
		{name: "no trigger", rname: "r", skill: "s"},
		{name: "both triggers", rname: "r", every: "1m", event: "task.created", skill: "s"},
		{name: "no action", rname: "r", every: "1m"},
		{name: "both actions", rname: "r", every: "1m", skill: "s", exec: "echo hi"},
		{name: "bad edge spec", rname: "r", every: "1m", skill: "s", edges: []string{"noseparator"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildRuleFromFlags(tc.rname, tc.every, tc.event, tc.ifEnt, tc.ifEdge, tc.skill, tc.exec, tc.edges, tc.arts, tc.edgeFrom, false)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.name)
			}
		})
	}
}

func TestParseDataFlags(t *testing.T) {
	got, err := parseDataFlags([]string{"task_id=TASK-1", "note=has=equals"})
	if err != nil {
		t.Fatal(err)
	}
	if got["task_id"] != "TASK-1" {
		t.Fatalf("task_id = %q", got["task_id"])
	}
	if got["note"] != "has=equals" {
		t.Fatalf("note = %q (should split on first = only)", got["note"])
	}
	if _, err := parseDataFlags([]string{"noequals"}); err == nil {
		t.Fatal("expected error for a flag without =")
	}
	if got := mustNil(t, []string{}); got != nil {
		t.Fatalf("empty data should be nil, got %+v", got)
	}
}

func TestStringifyEventValue(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{"TASK-42", "TASK-42"},                    // strings pass through
		{float64(42), "42"},                       // whole JSON numbers: no ".0"
		{float64(42.5), "42.5"},                   // fractional numbers keep precision
		{float64(1e21), "1000000000000000000000"}, // no scientific notation
		{true, "true"},                            // other types fall back to %v
	}
	for _, c := range cases {
		if got := stringifyEventValue(c.in); got != c.want {
			t.Fatalf("stringifyEventValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func mustNil(t *testing.T, data []string) map[string]string {
	t.Helper()
	m, err := parseDataFlags(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
