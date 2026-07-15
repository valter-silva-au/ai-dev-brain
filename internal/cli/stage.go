package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewStageCmd creates the `adb stage` command group for advancing initiatives
// through the founder-playbook lifecycle behind their StageGates.
func NewStageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage",
		Short: "Advance initiatives through founder-playbook stage gates",
		Long: `Advance an initiative from one founder-playbook stage to the next behind a
StageGate — a declarative evidence bundle of deterministic checks (an artifact
exists and is non-empty) plus judgment items (an adversarial verdict, represented
but not yet automated). The advance is BLOCKED until the required deterministic
evidence is met; judgment items are shown as "pending" and never block.

Evidence artifacts for an initiative live under
initiatives/<initiative-id>/evidence/ (workspace metadata — not part of the
ticket/worktree path layout).`,
	}
	cmd.AddCommand(newStageAdvanceCmd())
	return cmd
}

func newStageAdvanceCmd() *cobra.Command {
	var (
		override bool
		reason   string
	)
	cmd := &cobra.Command{
		Use:   "advance <initiative-id>",
		Short: "Advance an initiative to the next stage if its StageGate passes",
		Long: `Advance an initiative to the next founder-playbook stage. The advance is
blocked until the StageGate's deterministic evidence is met.

--override advances PAST a blocked gate and requires --reason. Override is
HUMAN-ONLY: automations and agents must advance only on a clean pass. A clean
pass or an override emits a stage.advanced event; an override additionally emits
a stage.override event carrying the reason (see 'adb events query --type
stage.advanced').`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			// Detect an automation context so the human-only Launch→Scale gate
			// (and the human-only override) are actually enforced. The D7 rule
			// engine stamps ADB_AUTOMATION_ACTIVE=1 on the child it shells
			// (core/ruleactions.go), but the CLI previously hardcoded
			// Automated=false, so a rule running `adb stage advance --override`
			// could bypass the D5 human-only guard (#157).
			res, err := App.StageManager.AdvanceStage(args[0], core.AdvanceOptions{
				Override:  override,
				Reason:    reason,
				Automated: os.Getenv("ADB_AUTOMATION_ACTIVE") == "1",
			})
			if err != nil {
				return err
			}
			return reportAdvance(cmd, res)
		},
	}
	cmd.Flags().BoolVar(&override, "override", false, "Advance past a blocked gate (human-only; requires --reason)")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for the override (logged on the gate and in the stage.override event)")
	return cmd
}

// reportAdvance renders an AdvanceResult. On a clean pass it prints the stage
// transition; when blocked it prints every unmet deterministic item plus the
// pending judgment items and returns a non-zero error so the command fails.
func reportAdvance(cmd *cobra.Command, res core.AdvanceResult) error {
	out := cmd.OutOrStdout()

	if res.Advanced {
		if res.Overridden {
			fmt.Fprintf(out, "⚠ Initiative %q OVERRIDDEN %s → %s (reason: %s)\n", res.Initiative.ID, res.From, res.To, res.Gate.Reason)
		} else {
			fmt.Fprintf(out, "✓ Initiative %q advanced %s → %s\n", res.Initiative.ID, res.From, res.To)
		}
		for _, it := range res.Gate.Items {
			if it.Status == models.GateItemPending {
				fmt.Fprintf(out, "  · pending (not blocking): %s — %s\n", it.ID, it.Desc)
			}
		}
		return nil
	}

	// Blocked — list the unmet required items (missing file evidence OR
	// below-threshold metric nodes), any failed adversarial verdicts, and the
	// pending (non-blocking) judgment items.
	var missing, failed, pending int
	fmt.Fprintf(out, "✗ Stage gate %s is BLOCKED for %q.\n", res.Gate.Transition, res.Initiative.ID)
	for _, it := range res.Gate.Items {
		if it.Status == models.GateItemMissing {
			if missing == 0 {
				fmt.Fprintln(out, "\nUnmet required items (evidence / metrics):")
			}
			missing++
			fmt.Fprintf(out, "  ✗ %s: %s (%s)\n", it.ID, it.Desc, it.Detail)
		}
	}
	for _, it := range res.Gate.Items {
		if it.Status == models.GateItemFailed {
			if failed == 0 {
				fmt.Fprintln(out, "\nFailed adversarial verdict:")
			}
			failed++
			fmt.Fprintf(out, "  ✗ %s: %s (%s)\n", it.ID, it.Desc, it.Detail)
		}
	}
	for _, it := range res.Gate.Items {
		if it.Status == models.GateItemPending {
			pending++
		}
	}
	if pending > 0 {
		fmt.Fprintf(out, "\nJudgment items (represented, not blocking): %d pending.\n", pending)
	}
	return fmt.Errorf("stage gate blocked: %d required item(s) unmet, %d failed adversarial verdict(s)", missing, failed)
}
