package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// This file is the drift-detection CLI (TASK-00031 phase 2): `adb task
// validate` reports ticket drift (duplicate seed, stale template_version) so
// agents self-check before claiming done, and `adb task normalize --dedup`
// runs the one-time de-duplication migration over legacy tickets.

// newTaskValidateCmd builds `adb task validate <task-id>`.
func newTaskValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <task-id>",
		Short: "Check a ticket for drift (duplicate description seed, stale template version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.BacklogManager == nil {
				return fmt.Errorf("app not initialised")
			}
			taskID := args[0]
			task, err := App.BacklogManager.GetTask(taskID)
			if err != nil || task == nil {
				return fmt.Errorf("task %s not found", taskID)
			}
			lm, err := core.NewLayeredTemplateManager(templatesDir(), templates.FS)
			if err != nil {
				return err
			}
			_, vErr := runTicketValidate(cmd, task, lm)
			return vErr
		},
	}
	return cmd
}

// runTicketInspect inspects one ticket and prints its findings; returns the
// finding count.
func runTicketValidate(cmd *cobra.Command, task *models.Task, lm *core.LayeredTemplateManager) (int, error) {
	dir := task.TicketPath
	if dir == "" {
		if d, err := core.ResolveTicketDir(filepath.Join(App.BasePath, "tickets"), task.ID); err == nil && d != "" {
			dir = d
		}
	}
	if dir == "" {
		return 0, fmt.Errorf("ticket directory for %s not resolved", task.ID)
	}
	if _, err := os.Stat(dir); err != nil {
		return 0, fmt.Errorf("ticket directory %s: %w", dir, err)
	}

	description := ticketDescription(dir)
	findings := core.InspectTicket(dir, description, lm)
	out := cmd.OutOrStdout()
	if len(findings) == 0 {
		fmt.Fprintf(out, "✓ %s: ticket clean\n", task.ID)
		return 0, nil
	}
	for _, f := range findings {
		fmt.Fprintf(out, "⚠ %s: %s — %s\n", task.ID, f.Kind, f.Detail)
	}
	return len(findings), nil
}

// ticketDescription extracts the description from the ticket's context.md
// (## Description section) — the ground truth the duplicate seed copied.
func ticketDescription(ticketDir string) string {
	b, err := os.ReadFile(filepath.Join(ticketDir, "context.md"))
	if err != nil {
		return ""
	}
	return core.ExtractSection(string(b), "Description")
}
