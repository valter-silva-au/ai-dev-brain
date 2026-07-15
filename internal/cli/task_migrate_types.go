package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// newTaskMigrateTypesCmd builds `adb task migrate-types` — a one-shot,
// idempotent migration that rewrites the retired `bug` task type to `fix` in
// backlog.yaml, aligning existing entries with the WS-B Conventional taxonomy.
//
// backlog.yaml is the sole authoritative type store (adb's status.yaml template
// carries no `type` field), so — like `normalize-titles` (commit 9fb8cbd) —
// this touches only backlog.yaml. Dry-run by default; pass --apply to write.
func newTaskMigrateTypesCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "migrate-types",
		Short: "Rewrite the legacy `bug` task type to `fix` in backlog.yaml",
		Long: `Rewrites backlog.yaml so every task carrying the retired ` + "`bug`" + ` type
becomes ` + "`fix`" + `, matching the WS-B Conventional-Commits taxonomy. The
create path and MCP server already reject ` + "`bug`" + `; this brings pre-existing
backlog entries into line.

Idempotent: re-running finds nothing to change. Default is dry-run (changes
printed, backlog not touched). Use ` + "`--apply`" + ` to rewrite the file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.BacklogManager == nil {
				return fmt.Errorf("app not initialised")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("load backlog: %w", err)
			}
			changes := 0
			for i := range backlog.Tasks {
				t := &backlog.Tasks[i]
				if t.Type == models.TaskTypeBug {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %q -> %q\n", t.ID, t.Type, models.TaskTypeFix)
					t.Type = models.TaskTypeFix
					changes++
				}
			}
			if changes == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No task types need migrating.")
				return nil
			}
			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDry run: %d task type(s) would change. Pass --apply to rewrite backlog.yaml.\n", changes)
				return nil
			}
			if err := App.BacklogManager.Save(backlog); err != nil {
				return fmt.Errorf("save backlog: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Rewrote %d task type(s) in backlog.yaml.\n", changes)
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "actually rewrite backlog.yaml (default: dry-run)")
	return cmd
}
