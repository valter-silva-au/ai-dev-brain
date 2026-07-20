package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// NewInitiativeCmd creates the `adb initiative` command group for managing
// initiatives and their stage.
func NewInitiativeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiative",
		Short: "Manage initiatives and their stage",
		Long: `Manage initiatives — units of work that belong to an organization and
carry a founder-playbook Stage (Idea, MVP, Launch, Scale).

Stage is orthogonal to a task's status: it tracks where a business initiative sits
on the Idea -> MVP -> Launch -> Scale journey. Initiatives are stored as workspace
metadata (initiatives/index.yaml).`,
	}
	cmd.AddCommand(newInitiativeCreateCmd())
	cmd.AddCommand(newInitiativeListCmd())
	cmd.AddCommand(newInitiativeShowCmd())
	cmd.AddCommand(newInitiativeSetStageCmd())
	cmd.AddCommand(newInitiativeGateCmd())
	cmd.AddCommand(newInitiativeScaffoldEvidenceCmd())
	cmd.AddCommand(newInitiativeLintInterviewCmd())
	return cmd
}

func newInitiativeScaffoldEvidenceCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "scaffold-evidence <id>",
		Short: "Scaffold the Idea/MVP validation worksheets into an initiative's evidence dir",
		Long: `Drop the founder-playbook validation worksheets (problem-hypothesis,
interview-framework, evidence-ledger, scope, measurement-framework,
sean-ellis-survey, false-positive-registry) into an initiative's evidence
directory so you can fill them in as StageGate evidence.

The scaffold is idempotent and clobber-safe: an up-to-date file is left
unchanged and a worksheet you have edited is skipped rather than overwritten
(pass --force to overwrite). --dry-run previews without writing.

Note: a scaffolded (unfilled) worksheet is non-empty, so it satisfies the
gate's DETERMINISTIC file check — but the adversarial verdict is what actually
judges the content, so placeholder worksheets do not earn an advance.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			evidenceDir, err := App.StageManager.EvidenceDir(args[0])
			if err != nil {
				return err
			}
			res, err := core.ScaffoldValidationTemplates(claude.FS, evidenceDir, core.HarnessInstallOptions{DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to scaffold validation templates: %w", err)
			}

			out := cmd.OutOrStdout()
			verb := "Scaffolding"
			if dryRun {
				verb = "Would scaffold"
			}
			fmt.Fprintf(out, "%s validation worksheets into %s...\n", verb, evidenceDir)
			for _, e := range res.Entries {
				note := ""
				if e.Action == core.HarnessSkipped {
					note = " (edited locally; pass --force to overwrite)"
				}
				fmt.Fprintf(out, "  %-9s %s%s\n", e.Action, e.Name+".md", note)
			}
			fmt.Fprintf(out, "✓ Validation pack: %d written, %d unchanged, %d skipped\n",
				res.Count(core.HarnessInstalled), res.Count(core.HarnessUnchanged), res.Count(core.HarnessSkipped))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without writing")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite worksheets that were edited (differ from the template)")
	return cmd
}

func newInitiativeLintInterviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint-interview <file>",
		Short: "Flag Mom Test violations in a customer-interview question file",
		Long: `Run the Mom Test linter over a file of interview questions. It flags
hypothetical/future ("would you use…"), opinion ("do you like the idea?"),
leading ("don't you hate…"), and hypothetical-pricing ("how much would you
pay?") questions — the ones that produce false positives. Good questions ask
about specific past behaviour and pass silently.

Exits non-zero when any question is flagged, so it doubles as a check.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(filepath.Clean(args[0]))
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", args[0], err)
			}
			findings := core.LintMomTest(string(data))
			out := cmd.OutOrStdout()
			if len(findings) == 0 {
				fmt.Fprintln(out, "✓ No Mom Test violations found.")
				return nil
			}
			for _, f := range findings {
				fmt.Fprintf(out, "  line %d [%s]: %s\n      → %s\n", f.Line, f.Rule, f.Text, f.Hint)
			}
			return fmt.Errorf("%d Mom Test violation(s) found", len(findings))
		},
	}
	return cmd
}

func newInitiativeCreateCmd() *cobra.Command {
	var (
		orgID      string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "create <name> --org <org-id>",
		Short: "Create an initiative (defaults to the Idea stage)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if orgID == "" {
				return fmt.Errorf("--org is required")
			}
			init, err := App.StageManager.CreateInitiative(args[0], orgID)
			if err != nil {
				return fmt.Errorf("failed to create initiative: %w", err)
			}
			if jsonOutput {
				return printJSON(init)
			}
			fmt.Printf("Created initiative %q (%s) in org %q at stage %s\n", init.Name, init.ID, init.OrgID, init.Stage)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org", "", "organization id this initiative belongs to (required)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newInitiativeListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List initiatives",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			inits, err := App.StageManager.ListInitiatives()
			if err != nil {
				return fmt.Errorf("failed to list initiatives: %w", err)
			}
			if jsonOutput {
				return printJSON(inits)
			}
			if len(inits) == 0 {
				fmt.Println("No initiatives. Create one with `adb initiative create <name> --org <org-id>`.")
				return nil
			}
			for _, init := range inits {
				fmt.Printf("%-24s %-24s %-16s %s\n", init.ID, init.Name, init.OrgID, init.Stage)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newInitiativeShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an initiative (including its stage)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			init, err := App.StageManager.GetInitiative(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(init)
			}
			fmt.Printf("ID:      %s\n", init.ID)
			fmt.Printf("Name:    %s\n", init.Name)
			fmt.Printf("Org:     %s\n", init.OrgID)
			fmt.Printf("Stage:   %s\n", init.Stage)
			fmt.Printf("Created: %s\n", init.Created.Format("2006-01-02 15:04:05 MST"))
			fmt.Printf("Updated: %s\n", init.Updated.Format("2006-01-02 15:04:05 MST"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newInitiativeSetStageCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "set-stage <id> <stage>",
		Short: "Set an initiative's stage (Idea, MVP, Launch, Scale)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			init, err := App.StageManager.SetStage(args[0], models.Stage(args[1]))
			if err != nil {
				return fmt.Errorf("failed to set stage: %w", err)
			}
			if jsonOutput {
				return printJSON(init)
			}
			fmt.Printf("Initiative %q is now at stage %s\n", init.ID, init.Stage)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newInitiativeGateCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "gate <id>",
		Short: "Show the current-stage gate evaluation (read-only; does not advance)",
		Long: `Evaluate the gate for the initiative's CURRENT stage and print the result
WITHOUT advancing or persisting anything — the read companion to ` + "`adb stage advance`" + `.

This is deliberately distinct from the gate shown by ` + "`adb initiative show`" + `:
` + "`adb stage advance`" + ` persists a gate only when it DECIDES an advance, so the
stored gate is the LAST TRANSITION DECISION and may describe an EARLIER
transition than the current stage (e.g. an overridden Idea->MVP gate left on an
initiative that now sits at MVP). This command computes the CURRENT-stage gate
fresh instead; --json returns both the fresh current_evaluation (with
evaluated_at) and the stored last_transition_decision so a caller never confuses
them. At the terminal Scale stage there is no gate (has_gate=false).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			res, err := App.StageManager.EvaluateCurrentGate(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(toGateView(res))
			}
			printGateHuman(res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// printGateHuman renders a current-gate evaluation for the terminal (non-JSON path).
func printGateHuman(res core.CurrentGateEvaluation) {
	fmt.Printf("Initiative: %s (stage %s)\n", res.InitiativeID, res.Stage)
	if !res.HasGate {
		fmt.Printf("No gate: %s is the terminal stage — nothing to advance.\n", res.Stage)
	} else {
		state := "BLOCKED"
		if res.CurrentEvaluation.Passed {
			state = "READY"
		}
		fmt.Printf("Current gate: %s — %s (evaluated %s)\n",
			res.CurrentEvaluation.Transition, state, res.EvaluatedAt.Format("2006-01-02 15:04:05 MST"))
		for _, it := range res.CurrentEvaluation.Items {
			fmt.Printf("  %-8s %-22s %s\n", it.Status, it.ID, it.Detail)
		}
	}
	if d := res.LastTransitionDecision; d != nil {
		suffix := ""
		if d.Overridden {
			suffix = fmt.Sprintf(" (overridden: %s)", d.Reason)
		}
		fmt.Printf("Last transition decision: %s%s — evaluated %s\n",
			d.Transition, suffix, d.Evaluated.Format("2006-01-02 15:04:05 MST"))
	}
}

// gateView is the JSON projection of a current-gate evaluation for
// `adb initiative gate --json` (the CLI-level shape a UI/HTTP layer consumes,
// mirroring taskStatusJSON's projection role). It uses pointers so a terminal
// stage (no gate) serializes current_evaluation/evaluated_at as null rather than
// a misleading zero value; models.GateState already carries json tags.
type gateView struct {
	InitiativeID           string            `json:"initiative_id"`
	Stage                  string            `json:"stage"`
	HasGate                bool              `json:"has_gate"`
	CurrentEvaluation      *models.GateState `json:"current_evaluation"`
	EvaluatedAt            *time.Time        `json:"evaluated_at"`
	LastTransitionDecision *models.GateState `json:"last_transition_decision"`
}

// toGateView maps the core evaluation to its JSON projection, nil-ing the
// current-gate fields at a terminal stage so the JSON is unambiguous.
func toGateView(res core.CurrentGateEvaluation) gateView {
	v := gateView{
		InitiativeID:           res.InitiativeID,
		Stage:                  string(res.Stage),
		HasGate:                res.HasGate,
		LastTransitionDecision: res.LastTransitionDecision,
	}
	if res.HasGate {
		ce := res.CurrentEvaluation
		ea := res.EvaluatedAt
		v.CurrentEvaluation = &ce
		v.EvaluatedAt = &ea
	}
	return v
}
