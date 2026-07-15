package core

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file implements the unified declarative rule engine (decision D7):
//
//	on <trigger> [if <graph condition>] run <action> → write <outputs>
//
// The engine decides WHEN a rule fires (a time schedule, or a matched event,
// optionally guarded by a graph condition) and applies the rule's declared
// OUTPUTS (artifacts + typed graph edges). WHAT a rule does is delegated to an
// ActionRunner — logic lives in skills / external commands, never here. Per the
// house convention core defines the seams (RuleStore, ActionRunner, EdgeWriter,
// ArtifactWriter); file-backed defaults live alongside and app.go bridges the
// graph-mutating EdgeWriter to the entity stores.

// Firing statuses. A firing is exactly one of these.
const (
	FiringFired   = "fired"   // the action ran and every output was applied
	FiringSkipped = "skipped" // disabled, or the condition did not hold
	FiringError   = "error"   // the action or an output failed
)

// Firing is the outcome of evaluating one rule against one trigger. It is a
// value type so `adb schedule run/dispatch` can print (or JSON-emit) exactly
// what happened without the engine knowing about output formats.
type Firing struct {
	Rule   string `json:"rule"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Output string `json:"output,omitempty"`
}

// RuleStore persists the declarative rule set (automation/rules.yaml). It is the
// source of truth for automation; the engine holds no rules of its own.
type RuleStore interface {
	Load() (models.RuleSet, error)
	Save(models.RuleSet) error
}

// ActionRunner executes a rule's action. It is injected so core neither shells
// out nor writes skill requests directly (both are IO the house pattern keeps
// behind a seam, and a fake makes the engine testable without a filesystem or a
// subprocess).
type ActionRunner interface {
	// RunExec runs a command action and returns its combined output.
	RunExec(ctx context.Context, args []string) (string, error)
	// RecordSkillRequest records a skill-invocation request (adb cannot itself
	// run a Claude skill) and returns a short human summary of what it wrote.
	RecordSkillRequest(rule, skill string, payload map[string]string) (string, error)
}

// EdgeWriter appends a typed graph edge onto a source entity's frontmatter — the
// graph's SOURCE OF TRUTH (decision D6). The derived index is rebuilt from the
// entities afterwards. app.go bridges this to the backlog + initiative stores.
type EdgeWriter interface {
	AddEdge(from string, link models.Link) error
}

// ArtifactWriter writes a rule-output artifact under the workspace. A file-backed
// default lives in this package; the seam keeps the engine testable.
type ArtifactWriter interface {
	WriteArtifact(relPath, content string) error
}

// RuleEngine evaluates declarative rules. Per the house convention the
// constructor returns the interface so callers depend on behaviour.
type RuleEngine interface {
	// Rules returns every rule (enabled or not), in author order.
	Rules() ([]models.Rule, error)
	// TimeRules returns the enabled time-triggered rules (the ones the scheduler
	// turns into recurring jobs).
	TimeRules() ([]models.Rule, error)
	// FireByName fires a single rule by name with the given payload context,
	// regardless of its trigger kind. Used by `adb schedule run <name>` and by
	// the scheduler job wrapper for time rules.
	FireByName(ctx context.Context, name string, payload map[string]string) (Firing, error)
	// Dispatch fires every enabled event-rule whose trigger matches evtType and
	// whose optional condition holds, given the event's payload. Resilient: a
	// single rule's failure becomes an error Firing, never an aborted batch.
	Dispatch(ctx context.Context, evtType string, payload map[string]string) ([]Firing, error)
}

type ruleEngine struct {
	store     RuleStore
	graph     GraphManager
	runner    ActionRunner
	edges     EdgeWriter
	artifacts ArtifactWriter
	now       func() time.Time
}

// RuleEngineOption customises a RuleEngine.
type RuleEngineOption func(*ruleEngine)

// WithRuleClock injects the clock used to stamp output-artifact provenance.
// Defaults to time.Now; tests pin it for deterministic content.
func WithRuleClock(now func() time.Time) RuleEngineOption {
	return func(e *ruleEngine) {
		if now != nil {
			e.now = now
		}
	}
}

// NewRuleEngine wires the declarative rule engine. graph may be nil (rules with
// a condition then skip); edges may be nil (edge outputs then error); artifacts
// may be nil (artifact outputs then error). runner and store are required.
func NewRuleEngine(store RuleStore, graph GraphManager, runner ActionRunner, edges EdgeWriter, artifacts ArtifactWriter, opts ...RuleEngineOption) RuleEngine {
	e := &ruleEngine{
		store:     store,
		graph:     graph,
		runner:    runner,
		edges:     edges,
		artifacts: artifacts,
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

func (e *ruleEngine) Rules() ([]models.Rule, error) {
	rs, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	return rs.Rules, nil
}

func (e *ruleEngine) TimeRules() ([]models.Rule, error) {
	rs, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	var out []models.Rule
	for _, r := range rs.Rules {
		if r.On.IsSchedule() && r.IsEnabled() {
			out = append(out, r)
		}
	}
	return out, nil
}

func (e *ruleEngine) FireByName(ctx context.Context, name string, payload map[string]string) (Firing, error) {
	rs, err := e.store.Load()
	if err != nil {
		return Firing{}, err
	}
	for _, r := range rs.Rules {
		if r.Name == name {
			return e.fire(ctx, r, payload), nil
		}
	}
	return Firing{}, fmt.Errorf("no rule named %q", name)
}

func (e *ruleEngine) Dispatch(ctx context.Context, evtType string, payload map[string]string) ([]Firing, error) {
	rs, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	var out []Firing
	for _, r := range rs.Rules {
		if r.On.IsEvent() && r.On.Event == evtType {
			out = append(out, e.fire(ctx, r, payload))
		}
	}
	return out, nil
}

// fire evaluates one rule end-to-end: disabled/condition gate → action → outputs.
// It never returns an error; failures surface as an error Firing so a batch
// dispatch stays resilient.
func (e *ruleEngine) fire(ctx context.Context, rule models.Rule, payload map[string]string) Firing {
	if !rule.IsEnabled() {
		return skipped(rule.Name, "rule disabled")
	}

	// Optional graph condition: the resolved entity must have an incident edge
	// of the required type. An unresolved entity or an absent graph degrades to
	// a skip (never blocks, never panics).
	firingEntity := ""
	if rule.If != nil {
		entity, err := expandTemplate(rule.If.Entity, payload)
		if err != nil || strings.TrimSpace(entity) == "" {
			return skipped(rule.Name, fmt.Sprintf("condition entity %q unresolved", rule.If.Entity))
		}
		firingEntity = entity
		if e.graph == nil {
			return skipped(rule.Name, "condition requires a graph, none wired")
		}
		edges, err := e.graph.Neighbors(entity)
		if err != nil {
			return errored(rule.Name, fmt.Sprintf("graph query for %q: %v", entity, err))
		}
		if !hasEdgeType(edges, rule.If.HasEdge) {
			return skipped(rule.Name, fmt.Sprintf("condition not met: %s has no %s edge", entity, rule.If.HasEdge))
		}
	}

	out, err := e.runAction(ctx, rule, payload)
	if err != nil {
		return errored(rule.Name, err.Error())
	}
	if err := e.applyOutputs(rule, payload, firingEntity); err != nil {
		return errored(rule.Name, err.Error())
	}
	return Firing{Rule: rule.Name, Status: FiringFired, Output: out}
}

func (e *ruleEngine) runAction(ctx context.Context, rule models.Rule, payload map[string]string) (string, error) {
	if strings.TrimSpace(rule.Run.Skill) != "" {
		return e.runner.RecordSkillRequest(rule.Name, rule.Run.Skill, payload)
	}
	args := make([]string, len(rule.Run.Exec))
	for i, a := range rule.Run.Exec {
		ex, err := expandTemplate(a, payload)
		if err != nil {
			return "", fmt.Errorf("expand exec arg %d: %w", i, err)
		}
		args[i] = ex
	}
	return e.runner.RunExec(ctx, args)
}

// resolvedOutput is one output with every template already expanded, so the
// resolve pass can fail before any side effect is performed.
type resolvedOutput struct {
	artifactPath string       // "" when this output writes no artifact
	content      string       // artifact content (valid iff artifactPath != "")
	edgeFrom     string       // "" when this output writes no edge
	link         *models.Link // edge to add (valid iff edgeFrom != "")
}

func (e *ruleEngine) applyOutputs(rule models.Rule, payload map[string]string, firingEntity string) error {
	// RESOLVE every output's templates first, so a template/wiring error cannot
	// leave a partially-applied set (e.g. an artifact written before a later edge
	// fails to expand). Only once all resolve do we perform side effects.
	ops := make([]resolvedOutput, 0, len(rule.Write))
	for i, o := range rule.Write {
		var op resolvedOutput
		if strings.TrimSpace(o.Artifact) != "" {
			path, err := expandTemplate(o.Artifact, payload)
			if err != nil {
				return fmt.Errorf("output %d: expand artifact path: %w", i, err)
			}
			if e.artifacts == nil {
				return fmt.Errorf("output %d: artifact requested but no artifact writer wired", i)
			}
			op.artifactPath = path
			op.content = e.artifactContent(rule, payload)
		}
		if o.Edge != nil {
			from := strings.TrimSpace(o.EdgeFrom)
			if from == "" {
				from = firingEntity
			}
			from, err := expandTemplate(from, payload)
			if err != nil {
				return fmt.Errorf("output %d: expand edge_from: %w", i, err)
			}
			if strings.TrimSpace(from) == "" {
				return fmt.Errorf("output %d: edge needs edge_from (no condition entity to default to)", i)
			}
			target, err := expandTemplate(o.Edge.Target, payload)
			if err != nil {
				return fmt.Errorf("output %d: expand edge target: %w", i, err)
			}
			if e.edges == nil {
				return fmt.Errorf("output %d: edge requested but no edge writer wired", i)
			}
			op.edgeFrom = from
			op.link = &models.Link{Type: o.Edge.Type, Target: target}
		}
		ops = append(ops, op)
	}

	// APPLY. Writes are individually idempotent (artifact overwrite, edge dedup),
	// so a rule that re-fires never accretes state.
	for i, op := range ops {
		if op.artifactPath != "" {
			if err := e.artifacts.WriteArtifact(op.artifactPath, op.content); err != nil {
				return fmt.Errorf("output %d: write artifact %s: %w", i, op.artifactPath, err)
			}
		}
		if op.link != nil {
			if err := e.edges.AddEdge(op.edgeFrom, *op.link); err != nil {
				return fmt.Errorf("output %d: add edge %s --%s--> %s: %w", i, op.edgeFrom, op.link.Type, op.link.Target, err)
			}
		}
	}
	return nil
}

// artifactContent renders the provenance stub written for an artifact output. It
// records the rule, the fire time, the action, and the payload so a derived
// artifact traces back to what produced it (the D8 provenance discipline applied
// to rule outputs).
func (e *ruleEngine) artifactContent(rule models.Rule, payload map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Automation firing: %s\n\n", rule.Name)
	fmt.Fprintf(&b, "- rule: %s\n", rule.Name)
	fmt.Fprintf(&b, "- fired: %s\n", e.now().Format(time.RFC3339))
	if rule.On.IsSchedule() {
		fmt.Fprintf(&b, "- trigger: schedule %s\n", rule.On.Schedule)
	} else {
		fmt.Fprintf(&b, "- trigger: event %s\n", rule.On.Event)
	}
	if strings.TrimSpace(rule.Run.Skill) != "" {
		fmt.Fprintf(&b, "- action: skill %s\n", rule.Run.Skill)
	} else {
		fmt.Fprintf(&b, "- action: exec %s\n", strings.Join(rule.Run.Exec, " "))
	}
	if len(payload) > 0 {
		b.WriteString("- payload:\n")
		for _, k := range sortedKeys(payload) {
			fmt.Fprintf(&b, "  - %s: %s\n", k, payload[k])
		}
	}
	return b.String()
}

func skipped(rule, reason string) Firing {
	return Firing{Rule: rule, Status: FiringSkipped, Reason: reason}
}
func errored(rule, reason string) Firing {
	return Firing{Rule: rule, Status: FiringError, Reason: reason}
}

// hasEdgeType reports whether any edge is of type t.
func hasEdgeType(edges []models.GraphEdge, t models.EdgeType) bool {
	for _, e := range edges {
		if e.Type == t {
			return true
		}
	}
	return false
}

// expandTemplate resolves "{{.field}}" placeholders against payload. A string
// with no placeholders is returned verbatim (the common case). A reference to a
// key the payload lacks is an error (missingkey=error) so a rule that needs a
// field an event doesn't carry skips cleanly rather than writing a half-rendered
// path.
func expandTemplate(tmpl string, payload map[string]string) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}
	t, err := template.New("rule").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, payload); err != nil {
		return "", fmt.Errorf("expand %q: %w", tmpl, err)
	}
	return buf.String(), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
