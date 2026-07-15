package models

import (
	"fmt"
	"strings"
	"time"
)

// Rule is one declarative automation rule (decision D7). It reads:
//
//	on <trigger> [if <graph condition>] run <action> → write <outputs>
//
// A rule is authored into automation/rules.yaml (the source of truth) and
// surfaced by `adb schedule`. The logic a rule invokes lives in a skill or an
// external command — never in Go — so the engine only decides WHEN a rule fires
// and applies its declared outputs; WHAT it does is delegated to the action.
type Rule struct {
	// Name is a stable, unique identifier used to select the rule (e.g. by
	// `adb schedule run <name>`). Required.
	Name string `yaml:"name" json:"name"`
	// Enabled toggles the rule. Nil means enabled (the default) so an
	// unset field marshals away; an explicit `enabled: false` parks the rule.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// On is the trigger — exactly one of a time schedule or an event type.
	On RuleTrigger `yaml:"on" json:"on"`
	// If is an optional graph condition that must hold for the rule to fire.
	If *RuleCondition `yaml:"if,omitempty" json:"if,omitempty"`
	// Run is the action the rule invokes — exactly one of a skill or a command.
	Run RuleAction `yaml:"run" json:"run"`
	// Write is the set of outputs applied after the action succeeds — artifacts
	// and/or typed graph edges (the #109 edge model).
	Write []RuleOutput `yaml:"write,omitempty" json:"write,omitempty"`
}

// RuleTrigger is a rule's trigger. Exactly one of Schedule (time-triggered) or
// Event (event-triggered) is set. Schedule is a Go duration string (e.g. "15m",
// "6h"); Event is one of the observability KnownEventTypes (e.g.
// "task.status_changed"). The two-mode split mirrors decision D7: time-triggers
// first, event-triggers next.
type RuleTrigger struct {
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Event    string `yaml:"event,omitempty" json:"event,omitempty"`
}

// RuleCondition is an optional graph guard: the resolved Entity must have an
// incident edge of type HasEdge for the rule to fire. Entity may be a literal
// entity id (a task id, initiative id) or a "{{.field}}" template resolved
// against the triggering event's payload (e.g. "{{.task_id}}").
type RuleCondition struct {
	Entity  string   `yaml:"entity" json:"entity"`
	HasEdge EdgeType `yaml:"has_edge" json:"has_edge"`
}

// RuleAction is what a rule invokes when it fires. Exactly one of Skill or Exec
// is set. Skill names a skill/agent whose invocation is RECORDED as a request
// (adb cannot itself run a Claude skill); Exec is a real command run via the
// action runner. Both may carry "{{.field}}" templates resolved against the
// firing payload.
type RuleAction struct {
	Skill string   `yaml:"skill,omitempty" json:"skill,omitempty"`
	Exec  []string `yaml:"exec,omitempty" json:"exec,omitempty"`
}

// RuleOutput is one output applied after a rule's action succeeds. Each output
// writes an Artifact (a file under the workspace, path may template) and/or a
// typed graph Edge onto a source entity's frontmatter. At least one of the two
// is required.
type RuleOutput struct {
	// Artifact is a workspace-relative path (may template) of a provenance file
	// to write recording that the rule fired.
	Artifact string `yaml:"artifact,omitempty" json:"artifact,omitempty"`
	// Edge is a typed graph edge to declare on EdgeFrom (or, when EdgeFrom is
	// empty, on the condition/event entity). The edge Target may template.
	Edge *Link `yaml:"edge,omitempty" json:"edge,omitempty"`
	// EdgeFrom is the source entity the edge is declared on. Optional; defaults
	// to the firing entity (the condition entity, else the event's entity).
	EdgeFrom string `yaml:"edge_from,omitempty" json:"edge_from,omitempty"`
}

// RuleSet is the automation/rules.yaml document: an ordered list of rules.
type RuleSet struct {
	Rules []Rule `yaml:"rules" json:"rules"`
}

// IsEnabled reports whether the rule is active. A nil Enabled pointer means
// enabled (the default), so an unset field keeps the rule live.
func (r Rule) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// IsSchedule reports whether the trigger is a time schedule.
func (t RuleTrigger) IsSchedule() bool { return strings.TrimSpace(t.Schedule) != "" }

// IsEvent reports whether the trigger is an event type.
func (t RuleTrigger) IsEvent() bool { return strings.TrimSpace(t.Event) != "" }

// Interval parses the schedule as a Go duration. It errors for an event
// trigger or an unparseable / non-positive duration.
func (t RuleTrigger) Interval() (time.Duration, error) {
	if !t.IsSchedule() {
		return 0, fmt.Errorf("trigger is not a schedule")
	}
	d, err := time.ParseDuration(strings.TrimSpace(t.Schedule))
	if err != nil {
		return 0, fmt.Errorf("invalid schedule %q: %w", t.Schedule, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("schedule %q must be a positive duration", t.Schedule)
	}
	return d, nil
}

// Validate checks a rule is structurally well-formed: a name, exactly one
// trigger, exactly one action, a complete condition when present, and complete
// outputs. It is deliberately structural — it does NOT check that an event type
// is a known one (the write surface does that, tolerating unknown types on read
// the way the graph tolerates unknown edge types).
func (r Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	// Trigger: exactly one of schedule / event.
	switch {
	case r.On.IsSchedule() && r.On.IsEvent():
		return fmt.Errorf("rule %q: trigger has both a schedule and an event; set exactly one", r.Name)
	case r.On.IsSchedule():
		if _, err := r.On.Interval(); err != nil {
			return fmt.Errorf("rule %q: %w", r.Name, err)
		}
	case r.On.IsEvent():
		// structural OK; known-type validation is the write surface's job
	default:
		return fmt.Errorf("rule %q: trigger must set a schedule or an event", r.Name)
	}
	// Action: exactly one of skill / exec.
	hasSkill := strings.TrimSpace(r.Run.Skill) != ""
	hasExec := len(r.Run.Exec) > 0
	switch {
	case hasSkill && hasExec:
		return fmt.Errorf("rule %q: action has both a skill and an exec; set exactly one", r.Name)
	case !hasSkill && !hasExec:
		return fmt.Errorf("rule %q: action must set a skill or an exec", r.Name)
	}
	// Condition (optional): both fields required when present.
	if r.If != nil {
		if strings.TrimSpace(r.If.Entity) == "" {
			return fmt.Errorf("rule %q: condition needs an entity", r.Name)
		}
		if strings.TrimSpace(string(r.If.HasEdge)) == "" {
			return fmt.Errorf("rule %q: condition needs a has_edge type", r.Name)
		}
	}
	// Outputs (optional): each must write an artifact and/or a complete edge.
	for i, out := range r.Write {
		hasArtifact := strings.TrimSpace(out.Artifact) != ""
		if out.Edge == nil && !hasArtifact {
			return fmt.Errorf("rule %q: output %d must set an artifact or an edge", r.Name, i)
		}
		if out.Edge != nil {
			if strings.TrimSpace(string(out.Edge.Type)) == "" || strings.TrimSpace(out.Edge.Target) == "" {
				return fmt.Errorf("rule %q: output %d edge needs both type and target", r.Name, i)
			}
		}
	}
	return nil
}

// Validate checks every rule and rejects duplicate names.
func (rs RuleSet) Validate() error {
	seen := make(map[string]struct{}, len(rs.Rules))
	for _, r := range rs.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
		name := strings.TrimSpace(r.Name)
		if _, dup := seen[name]; dup {
			return fmt.Errorf("duplicate rule name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
