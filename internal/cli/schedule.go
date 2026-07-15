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
			// Reject unknown event types at the write surface (read stays tolerant).
			if rule.On.IsEvent() && !observability.IsKnownEventType(observability.EventType(rule.On.Event)) {
				return fmt.Errorf("unknown event type %q; must be one of the adb event schema (see `adb events`)", rule.On.Event)
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
	f.StringVar(&onEvent, "on-event", "", "event trigger: a known event type, e.g. task.status_changed")
	f.StringVar(&ifEntity, "if-entity", "", "graph condition entity (may template, e.g. '{{.task_id}}')")
	f.StringVar(&ifEdge, "if-edge", "", "graph condition edge type the entity must have, e.g. depends_on")
	f.StringVar(&runSkill, "run-skill", "", "action: record a request to run this skill")
	f.StringVar(&runExec, "run-exec", "", "action: run this command (whitespace-split)")
	f.StringArrayVar(&writeEdges, "write-edge", nil, "output: write a typed edge 'type:target' (repeatable)")
	f.StringArrayVar(&writeArts, "write-artifact", nil, "output: write an artifact at this path (repeatable)")
	f.StringVar(&edgeFrom, "edge-from", "", "output: source entity for --write-edge (defaults to the condition entity)")
	f.BoolVar(&disabled, "disabled", false, "add the rule parked (enabled: false)")
	return cmd
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
		rule.On = models.RuleTrigger{Event: strings.TrimSpace(onEvent)}
	default:
		return models.Rule{}, fmt.Errorf("a trigger is required: pass --every <dur> or --on-event <type>")
	}
	if strings.TrimSpace(ifEntity) != "" || strings.TrimSpace(ifEdge) != "" {
		rule.If = &models.RuleCondition{Entity: strings.TrimSpace(ifEntity), HasEdge: models.EdgeType(strings.TrimSpace(ifEdge))}
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
		rule.Write = append(rule.Write, models.RuleOutput{
			Edge:     &models.Link{Type: models.EdgeType(strings.TrimSpace(typ)), Target: strings.TrimSpace(target)},
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
