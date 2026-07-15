package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newTaskMigrateBlockedByCmd builds `adb task migrate-blocked-by` — a one-shot,
// idempotent migration that folds the legacy `blocked_by` dependency list onto
// the generic typed edge model (issue #110 / decision D6): each entry becomes a
// `depends_on` link in `links[]` and `blocked_by` is cleared, so there is one
// graph rather than two dependency representations.
//
// It mirrors `adb task migrate-types` (commit for #ws-b): backlog.yaml is the
// sole authoritative store, so this touches only backlog.yaml. Dry-run by
// default (changes printed, file untouched); pass --apply to write. Idempotent:
// a backlog with no `blocked_by` entries left finds nothing to migrate. Reads
// remain backward-compatible until migrated — IsBlocked() honours both
// `blocked_by` and a `depends_on` edge.
func newTaskMigrateBlockedByCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:    "migrate-blocked-by",
		Short:  "Fold legacy `blocked_by` onto the generic edge model (depends_on links)",
		Hidden: true,
		Long: `Rewrites backlog.yaml so every task's legacy ` + "`blocked_by`" + ` list is folded
onto the generic edge model as ` + "`depends_on`" + ` links, then clears
` + "`blocked_by`" + `. There is then one graph, not two dependency stores.

Idempotent: re-running finds nothing to change. Default is dry-run (changes
printed, backlog not touched). Use ` + "`--apply`" + ` to rewrite the file.
Backward-compatible: an un-migrated backlog is still interpreted correctly —
IsBlocked() honours both ` + "`blocked_by`" + ` and a ` + "`depends_on`" + ` edge.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.BacklogManager == nil {
				return fmt.Errorf("app not initialized")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("load backlog: %w", err)
			}
			changes := 0
			for i := range backlog.Tasks {
				t := &backlog.Tasks[i]
				if len(t.BlockedBy) == 0 {
					continue
				}
				was := strings.Join(t.BlockedBy, ", ")
				if t.MigrateBlockedByToLinks() {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: blocked_by [%s] -> depends_on links\n", t.ID, was)
					changes++
				}
			}
			if changes == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No blocked_by entries need migrating.")
				return nil
			}
			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDry run: %d task(s) would change. Pass --apply to rewrite backlog.yaml.\n", changes)
				return nil
			}
			if err := App.BacklogManager.Save(backlog); err != nil {
				return fmt.Errorf("save backlog: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Migrated %d task(s) in backlog.yaml.\n", changes)
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "actually rewrite backlog.yaml (default: dry-run)")
	return cmd
}
