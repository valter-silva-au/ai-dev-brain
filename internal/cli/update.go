package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// UpdateGen is the UpdateGenerator used by the update command.
// Set during application wiring (Task #43).
var UpdateGen core.UpdateGenerator

var updateCmd = &cobra.Command{
	Use:   "update [task-id]",
	Short: "Generate stakeholder communication updates for a task",
	Long: `Generate a plan of stakeholder communication updates based on the task's
current context, recent progress, blockers, and communication history.

This command DOES NOT send any messages. It only generates content for
your review. You decide what to send and when.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UpdateGen == nil {
			return fmt.Errorf("update generator not initialized")
		}

		taskID := args[0]
		plan, err := UpdateGen.GenerateUpdates(taskID)
		if err != nil {
			return fmt.Errorf("generating updates for %s: %w", taskID, err)
		}

		if len(plan.Messages) == 0 && len(plan.InformationNeeded) == 0 {
			fmt.Printf("No updates needed for %s.\n", taskID)
			return nil
		}

		fmt.Printf("Update plan for %s (generated %s)\n\n", plan.TaskID, plan.GeneratedAt.Format("2006-01-02 15:04"))

		if len(plan.Messages) > 0 {
			fmt.Printf("== Planned Messages (%d) ==\n\n", len(plan.Messages))
			for i, msg := range plan.Messages {
				fmt.Printf("[%d] To: %s via %s [%s]\n", i+1, msg.Recipient, msg.Channel, msg.Priority)
				fmt.Printf("    Subject: %s\n", msg.Subject)
				fmt.Printf("    Reason: %s\n", msg.Reason)
				fmt.Printf("    ---\n")
				fmt.Printf("    %s\n\n", msg.Body)
			}
		}

		if len(plan.InformationNeeded) > 0 {
			fmt.Printf("== Information Needed (%d) ==\n\n", len(plan.InformationNeeded))
			for i, req := range plan.InformationNeeded {
				blocking := ""
				if req.Blocking {
					blocking = " [BLOCKING]"
				}
				fmt.Printf("[%d] From: %s%s\n", i+1, req.From, blocking)
				fmt.Printf("    Question: %s\n", req.Question)
				fmt.Printf("    Context: %s\n\n", req.Context)
			}
		}

		return nil
	},
}

func init() {
	updateCmd.Deprecated = "use 'adb task update'"
	updateCmd.ValidArgsFunction = completeTaskIDs()
	rootCmd.AddCommand(updateCmd)
}
