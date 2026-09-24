package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewScheduleCmd creates the `adb schedule` command group — the surface for the
// unified declarative rule engine (decision D7). Rules are authored into
// automation/rules.yaml (the source of truth); this command adds/lists/removes
// them and fires them on demand. Time-triggered rules also run under the
// background scheduler (`adb scheduler start`); event-triggered rules fire when
// the scheduler drains the event log (opt-in via automation.enabled) or via
// `adb schedule dispatch`.
func NewScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Author and run declarative automation rules",
		Long: `Declarative automation rules (decision D7):

  on <trigger> [if <graph condition>] run <action> → write <outputs>

Rules live in automation/rules.yaml. A trigger is either a time schedule
(e.g. every 15m) or an event type (e.g. task.status_changed). An optional
graph condition guards firing. The action is a skill (recorded as a request
for an agent to run) or an exec command (run for real). Outputs are written
artifacts and/or typed graph edges.

  adb schedule list
  adb schedule add --name nightly-pull --every 15m --run-skill repos-pull
  adb schedule add --name flag-blocked --on-event task.status_changed \
      --if-entity '{{.task_id}}' --if-edge depends_on --run-skill triage
  adb schedule run [<name>]                 # fire a rule now (or all time rules)
  adb schedule dispatch --event task.status_changed --data task_id=TASK-1
  adb schedule remove <name>`,
	}
	cmd.AddCommand(
		newScheduleListCmd(),
		newScheduleAddCmd(),
		newScheduleRemoveCmd(),
		newScheduleRunCmd(),
		newScheduleDispatchCmd(),
	)
	return cmd
}

func ruleStore() *storage.FileRuleStore {
	return storage.NewFileRuleStore(schedulerBase())
}

func newScheduleListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List automation rules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.RuleEngine == nil {
				return fmt.Errorf("app not initialized")
			}
			rules, err := App.RuleEngine.Rules()
			if err != nil {
				return fmt.Errorf("load rules: %w", err)
			}
			if jsonOutput {
				return printJSON(rules)
			}
			if len(rules) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No automation rules. Add one with `adb schedule add`.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tENABLED\tTRIGGER\tCONDITION\tACTION\tOUTPUTS")
			for _, r := range rules {
				fmt.Fprintf(w, "%s\t%t\t%s\t%s\t%s\t%d\n",
					r.Name, r.IsEnabled(), triggerLabel(r), conditionLabel(r), actionLabel(r), len(r.Write))
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func triggerLabel(r models.Rule) string {
	if r.On.IsSchedule() {
		return "every " + r.On.Schedule
	}
	if r.On.IsEvent() {
		return "on " + r.On.Event
	}
	return "-"
}

func conditionLabel(r models.Rule) string {
	if r.If == nil {
		return "-"
	}
	return fmt.Sprintf("%s has %s", r.If.Entity, r.If.HasEdge)
}

func actionLabel(r models.Rule) string {
	if strings.TrimSpace(r.Run.Skill) != "" {
		return "skill " + r.Run.Skill
	}
	return "exec " + strings.Join(r.Run.Exec, " ")
}

func newScheduleAddCmd() *cobra.Command {
	var (
		name       string
		every      string
		onEvent    string
		ifEntity   string
		ifEdge     string
		runSkill   string
		runExec    string
		writeEdges []string
		writeArts  []string
		edgeFrom   string
		disabled   bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an automation rule to automation/rules.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			rule, err := buildRuleFromFlags(name, every, onEvent, ifEntity, ifEdge, runSkill, runExec, writeEdges, writeArts, edgeFrom, disabled)
			if err != nil {
				return err
			}
			store := ruleStore()
			set, err := store.Load()
			if err != nil {
				return fmt.Errorf("load rules: %w", err)
			}
			for _, existing := range set.Rules {
				if existing.Name == rule.Name {
					return fmt.Errorf("a rule named %q already exists; remove it first", rule.Name)
				}
			}
			set.Rules = append(set.Rules, rule)
			if err := store.Save(set); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Added rule %q (%s → %s).\n", rule.Name, triggerLabel(rule), actionLabel(rule))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "unique rule name (required)")
	f.StringVar(&every, "every", "", "time trigger: a Go duration, e.g. 15m, 6h")
	f.StringVar(&onEvent, "on-event", "", "event trigger: "+eventTypeHint())
	f.StringVar(&ifEntity, "if-entity", "", "graph condition entity (may template, e.g. '{{.task_id}}')")
	f.StringVar(&ifEdge, "if-edge", "", "graph condition edge type the entity must have, one of "+edgeTypeHint())
	f.StringVar(&runSkill, "run-skill", "", "action: record a request to run this skill")
	f.StringVar(&runExec, "run-exec", "", "action: run this command (whitespace-split)")
	f.StringArrayVar(&writeEdges, "write-edge", nil, "output: write a typed edge 'type:target', type one of "+edgeTypeHint()+" (repeatable)")
	f.StringArrayVar(&writeArts, "write-artifact", nil, "output: write an artifact at this path (repeatable)")
	f.StringVar(&edgeFrom, "edge-from", "", "output: source entity for --write-edge (defaults to the condition entity)")
	f.BoolVar(&disabled, "disabled", false, "add the rule parked (enabled: false)")
	return cmd
}

// edgeTypeHint renders the closed edge-type vocabulary for flag help, derived
// from models.CanonicalEdgeTypes so the advertised set cannot drift from the set
// EdgeType.Validate enforces.
func edgeTypeHint() string {
	names := make([]string, 0, len(models.CanonicalEdgeTypes))
	for _, t := range models.CanonicalEdgeTypes {
		names = append(names, string(t))
	}
	return strings.Join(names, "|")
}

// eventTypeHint renders the event-type vocabulary for FLAG HELP, derived from
// observability.KnownEventTypes so the advertised set cannot drift from the set
// validateEventType enforces — the same contract edgeTypeHint has.
//
// It renders GROUP PREFIXES and a count rather than the full list, which is the
// one place this deliberately differs from edgeTypeHint: there are 19 event types
// against 5 edge types, and pipe-joining them produces a ~330-character usage
// string that dwarfs every other flag on `adb schedule add --help`. The full set
// goes in validateEventType's rejection error instead — read once, by someone who
// is stuck, which is where that length earns its place. Both halves stay derived,
// so a new type in a new group shows up as a new prefix and a new type in an
// existing group bumps the count.
func eventTypeHint() string {
	prefixes := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, e := range observability.KnownEventTypes {
		prefix, _, ok := strings.Cut(string(e), ".")
		if !ok {
			// A type with no group (none today) is advertised whole rather than
			// silently dropped from the hint.
			prefix = string(e)
		}
		if _, dup := seen[prefix]; dup {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix+".*")
	}
	return fmt.Sprintf("one of the %d known event types (%s)",
		len(observability.KnownEventTypes), strings.Join(prefixes, ", "))
}

// eventTypeList renders the full vocabulary, for the rejection error only.
func eventTypeList() string {
	names := make([]string, 0, len(observability.KnownEventTypes))
	for _, e := range observability.KnownEventTypes {
		names = append(names, string(e))
	}
	return strings.Join(names, ", ")
}

// validateEventType is the WRITE-path gate over observability.IsKnownEventType,
// and the single spelling of the rule for this package — the event-type analogue
// of models.EdgeType.Validate, which its error phrasing mirrors.
//
// It lives here rather than on observability.EventType because `adb schedule add
// --on-event` is currently the ONLY surface that accepts an event type as
// authored input. (An edge type has four such surfaces, which is why that gate had
// to be promoted into models.) If a second one appears — an MCP schedule-add tool,
// say — promote this to an observability.EventType.Validate method rather than
// copying it.
//
// The accepted set is exactly KnownEventTypes, INCLUDING the two reserved types
// nothing currently emits (task.completed, knowledge.extracted): emitted-ness is
// not recorded anywhere a program can read, so excluding them means a hand-written
// denylist that goes stale in the dangerous direction — worktree.created was
// reserved until #206 graduated it. `adb schedule dispatch --event <type>` also
// fires such a rule today, so "nothing emits it" is not "it can never fire".
//
// READ paths must NOT call this — see Rule.Validate, which stays structural.
func validateEventType(event string) error {
	if observability.IsKnownEventType(observability.EventType(event)) {
		return nil
	}
	return fmt.Errorf("unknown event type %q (must be one of %s)", event, eventTypeList())
}

// buildRuleFromFlags assembles + validates a Rule from `adb schedule add` flags.
func buildRuleFromFlags(name, every, onEvent, ifEntity, ifEdge, runSkill, runExec string, writeEdges, writeArts []string, edgeFrom string, disabled bool) (models.Rule, error) {
	rule := models.Rule{Name: strings.TrimSpace(name)}
	if disabled {
		off := false
		rule.Enabled = &off
	}
	switch {
	case strings.TrimSpace(every) != "" && strings.TrimSpace(onEvent) != "":
		return models.Rule{}, fmt.Errorf("set exactly one of --every or --on-event")
	case strings.TrimSpace(every) != "":
		rule.On = models.RuleTrigger{Schedule: strings.TrimSpace(every)}
	case strings.TrimSpace(onEvent) != "":
		event := strings.TrimSpace(onEvent)
		// The trigger is matched literally against the dispatched event name, so a
		// typo (`task.status-changed`) does not error at fire time — nothing ever
		// dispatches that name, and the rule silently never fires. Same defect
		// shape as --if-edge below, so it gets the same treatment: gate it here,
		// where the author can see it, and name the vocabulary in the error.
		if err := validateEventType(event); err != nil {
			return models.Rule{}, fmt.Errorf("--on-event: %w", err)
		}
		rule.On = models.RuleTrigger{Event: event}
	default:
		return models.Rule{}, fmt.Errorf("a trigger is required: pass --every <dur> or --on-event <type>")
	}
	if strings.TrimSpace(ifEntity) != "" || strings.TrimSpace(ifEdge) != "" {
		hasEdge := models.EdgeType(strings.TrimSpace(ifEdge))
		// The condition is matched literally against the graph, so a typo
		// (`depends-on`) does not error at fire time — it just never matches, and
		// the rule silently never fires. Gate it here, where the author can see it.
		// A MISSING type is left to Rule.Validate, whose "condition needs a
		// has_edge type" says the useful thing; the vocabulary hint would not.
		if hasEdge != "" {
			if err := hasEdge.Validate(); err != nil {
				return models.Rule{}, fmt.Errorf("--if-edge: %w", err)
			}
		}
		rule.If = &models.RuleCondition{Entity: strings.TrimSpace(ifEntity), HasEdge: hasEdge}
	}
	switch {
	case strings.TrimSpace(runSkill) != "" && strings.TrimSpace(runExec) != "":
		return models.Rule{}, fmt.Errorf("set exactly one of --run-skill or --run-exec")
	case strings.TrimSpace(runSkill) != "":
		rule.Run = models.RuleAction{Skill: strings.TrimSpace(runSkill)}
	case strings.TrimSpace(runExec) != "":
		rule.Run = models.RuleAction{Exec: strings.Fields(runExec)}
	default:
		return models.Rule{}, fmt.Errorf("an action is required: pass --run-skill <name> or --run-exec <cmd>")
	}
	for _, spec := range writeArts {
		if strings.TrimSpace(spec) != "" {
			rule.Write = append(rule.Write, models.RuleOutput{Artifact: strings.TrimSpace(spec)})
		}
	}
	for _, spec := range writeEdges {
		typ, target, ok := strings.Cut(spec, ":")
		if !ok || strings.TrimSpace(typ) == "" || strings.TrimSpace(target) == "" {
			return models.Rule{}, fmt.Errorf("--write-edge %q must be 'type:target', e.g. relates_to:TASK-1", spec)
		}
		edgeType := models.EdgeType(strings.TrimSpace(typ))
		// This is the one public surface that lets a human name an edge type that
		// gets WRITTEN onto an entity's frontmatter, so it is where the closed D6
		// vocabulary is enforced — at authoring time rather than when the rule fires
		// unattended (the engine gates it again there, as the last line of defence).
		if err := edgeType.Validate(); err != nil {
			return models.Rule{}, fmt.Errorf("--write-edge %q: %w", spec, err)
		}
		rule.Write = append(rule.Write, models.RuleOutput{
			Edge:     &models.Link{Type: edgeType, Target: strings.TrimSpace(target)},
			EdgeFrom: strings.TrimSpace(edgeFrom),
		})
	}
	if err := rule.Validate(); err != nil {
		return models.Rule{}, err
	}
	return rule, nil
}

func newScheduleRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an automation rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			name := args[0]
			store := ruleStore()
			set, err := store.Load()
			if err != nil {
				return fmt.Errorf("load rules: %w", err)
			}
			kept := make([]models.Rule, 0, len(set.Rules))
			found := false
			for _, r := range set.Rules {
				if r.Name == name {
					found = true
					continue
				}
				kept = append(kept, r)
			}
			if !found {
				return fmt.Errorf("no rule named %q", name)
			}
			set.Rules = kept
			if err := store.Save(set); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Removed rule %q.\n", name)
			return nil
		},
	}
	return cmd
}

func newScheduleRunCmd() *cobra.Command {
	var (
		data       []string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "run [name]",
		Short: "Fire a rule now (or every time-triggered rule when no name is given)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.RuleEngine == nil {
				return fmt.Errorf("app not initialized")
			}
			payload, err := parseDataFlags(data)
			if err != nil {
				return err
			}
			var firings []core.Firing
			if len(args) == 1 {
				f, err := App.RuleEngine.FireByName(context.Background(), args[0], payload)
				if err != nil {
					return err
				}
				firings = append(firings, f)
			} else {
				timeRules, err := App.RuleEngine.TimeRules()
				if err != nil {
					return fmt.Errorf("load time rules: %w", err)
				}
				for _, r := range timeRules {
					f, err := App.RuleEngine.FireByName(context.Background(), r.Name, payload)
					if err != nil {
						return err
					}
					firings = append(firings, f)
				}
			}
			return printFirings(cmd, firings, jsonOutput)
		},
	}
	cmd.Flags().StringArrayVar(&data, "data", nil, "payload key=value for template expansion (repeatable)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output firings as JSON")
	return cmd
}

func newScheduleDispatchCmd() *cobra.Command {
	var (
		event      string
		data       []string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "dispatch --event <type>",
		Short: "Fire event-triggered rules for one event (manual / hook-driven)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.RuleEngine == nil {
				return fmt.Errorf("app not initialized")
			}
			if strings.TrimSpace(event) == "" {
				return fmt.Errorf("--event is required, e.g. --event task.status_changed")
			}
			payload, err := parseDataFlags(data)
			if err != nil {
				return err
			}
			firings, err := App.RuleEngine.Dispatch(context.Background(), event, payload)
			if err != nil {
				return err
			}
			return printFirings(cmd, firings, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&event, "event", "", "the event type to dispatch, e.g. task.status_changed")
	cmd.Flags().StringArrayVar(&data, "data", nil, "event payload key=value (repeatable)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output firings as JSON")
	return cmd
}

// parseDataFlags turns repeated "key=value" flags into a payload map.
func parseDataFlags(data []string) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(data))
	for _, kv := range data {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("--data %q must be key=value", kv)
		}
		out[strings.TrimSpace(key)] = val
	}
	return out, nil
}

func printFirings(cmd *cobra.Command, firings []core.Firing, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(firings)
	}
	if len(firings) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No rules fired.")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RULE\tSTATUS\tDETAIL")
	for _, f := range firings {
		detail := f.Reason
		if f.Status == core.FiringFired {
			detail = f.Output
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", f.Rule, f.Status, truncateText(detail, 70))
	}
	return w.Flush()
}
