package models

import (
	"testing"
	"time"
)

func TestRule_IsEnabled(t *testing.T) {
	on := true
	off := false
	cases := []struct {
		name string
		e    *bool
		want bool
	}{
		{"nil defaults enabled", nil, true},
		{"explicit true", &on, true},
		{"explicit false", &off, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Rule{Name: "r", Enabled: tc.e}
			if got := r.IsEnabled(); got != tc.want {
				t.Fatalf("IsEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRuleTrigger_Interval(t *testing.T) {
	if _, err := (RuleTrigger{Event: "task.created"}).Interval(); err == nil {
		t.Fatal("expected error for an event trigger, got nil")
	}
	if _, err := (RuleTrigger{Schedule: "notaduration"}).Interval(); err == nil {
		t.Fatal("expected error for unparseable schedule, got nil")
	}
	if _, err := (RuleTrigger{Schedule: "0s"}).Interval(); err == nil {
		t.Fatal("expected error for non-positive schedule, got nil")
	}
	d, err := (RuleTrigger{Schedule: "15m"}).Interval()
	if err != nil {
		t.Fatalf("Interval() error = %v", err)
	}
	if d != 15*time.Minute {
		t.Fatalf("Interval() = %v, want 15m", d)
	}
}

func TestRule_Validate(t *testing.T) {
	valid := Rule{
		Name: "nightly-pull",
		On:   RuleTrigger{Schedule: "15m"},
		Run:  RuleAction{Skill: "repos-pull"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}

	cases := []struct {
		name string
		rule Rule
	}{
		{"no name", Rule{On: RuleTrigger{Schedule: "1m"}, Run: RuleAction{Skill: "s"}}},
		{"no trigger", Rule{Name: "r", Run: RuleAction{Skill: "s"}}},
		{"both triggers", Rule{Name: "r", On: RuleTrigger{Schedule: "1m", Event: "task.created"}, Run: RuleAction{Skill: "s"}}},
		{"bad schedule", Rule{Name: "r", On: RuleTrigger{Schedule: "nope"}, Run: RuleAction{Skill: "s"}}},
		{"no action", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}}},
		{"both actions", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}, Run: RuleAction{Skill: "s", Exec: []string{"echo"}}}},
		{"condition missing entity", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}, Run: RuleAction{Skill: "s"}, If: &RuleCondition{HasEdge: EdgeDependsOn}}},
		{"condition missing edge", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}, Run: RuleAction{Skill: "s"}, If: &RuleCondition{Entity: "TASK-1"}}},
		{"empty output", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}, Run: RuleAction{Skill: "s"}, Write: []RuleOutput{{}}}},
		{"edge output missing target", Rule{Name: "r", On: RuleTrigger{Event: "task.created"}, Run: RuleAction{Skill: "s"}, Write: []RuleOutput{{Edge: &Link{Type: EdgeRelatesTo}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.rule.Validate(); err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.name)
			}
		})
	}
}

func TestRule_Validate_EventTriggerWithExecAndOutputs(t *testing.T) {
	r := Rule{
		Name: "flag-blocked",
		On:   RuleTrigger{Event: "task.status_changed"},
		If:   &RuleCondition{Entity: "{{.task_id}}", HasEdge: EdgeDependsOn},
		Run:  RuleAction{Exec: []string{"echo", "hi"}},
		Write: []RuleOutput{
			{Artifact: "reports/{{.task_id}}.md"},
			{Edge: &Link{Type: EdgeRelatesTo, Target: "TASK-1"}, EdgeFrom: "{{.task_id}}"},
		},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("valid event rule rejected: %v", err)
	}
}

func TestRuleSet_Validate_DuplicateNames(t *testing.T) {
	rs := RuleSet{Rules: []Rule{
		{Name: "dup", On: RuleTrigger{Schedule: "1m"}, Run: RuleAction{Skill: "a"}},
		{Name: "dup", On: RuleTrigger{Schedule: "2m"}, Run: RuleAction{Skill: "b"}},
	}}
	if err := rs.Validate(); err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}
