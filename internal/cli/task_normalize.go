package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newTaskNormalizeTitlesCmd builds `adb task normalize-titles` — a
// one-shot migration that strips the type prefix from stored task
// titles. Older versions of `adb task create` pre-baked the task type
// into the stored Title as `[feat] branch`; the renderer at
// `adb task status` then wrapped it again, producing
// `[feat] [feat] branch`. PR #52 fixed create to store the raw branch,
// but existing backlog entries keep their old doubled titles until
// this migration runs.
//
// Dry-run by default. Pass --apply to actually rewrite backlog.yaml.
func newTaskNormalizeTitlesCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "normalize-titles",
		Short: "Strip duplicate [type] prefix from stored task titles",
		Long: `Rewrites backlog.yaml so each task's Title no longer carries the
` + "`[type]`" + ` prefix. The ` + "`adb task status`" + ` renderer adds the
prefix back at display time, so after this migration old entries
display identically to freshly-created ones.

Default is dry-run (changes printed, backlog not touched). Use
` + "`--apply`" + ` to rewrite the file.`,
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
				prefix := fmt.Sprintf("[%s] ", t.Type)
				if strings.HasPrefix(t.Title, prefix) {
					newTitle := strings.TrimPrefix(t.Title, prefix)
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %q -> %q\n", t.ID, t.Title, newTitle)
					t.Title = newTitle
					changes++
				}
			}
			if changes == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No titles need normalising.")
				return nil
			}
			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDry run: %d title(s) would change. Pass --apply to rewrite backlog.yaml.\n", changes)
				return nil
			}
			if err := App.BacklogManager.Save(backlog); err != nil {
				return fmt.Errorf("save backlog: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Rewrote %d title(s) in backlog.yaml.\n", changes)
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "actually rewrite backlog.yaml (default: dry-run)")
	return cmd
}
