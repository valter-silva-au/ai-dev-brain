package cli

import (
	"fmt"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/spf13/cobra"
)

// FeedbackLoop is set during app initialization in app.go.
var FeedbackLoop core.FeedbackLoopOrchestrator

var loopDryRun bool
var loopChannel string

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Run the feedback loop",
	Long: `Execute a full feedback loop cycle: fetch items from input channels,
classify and route each item, deliver outputs, and record knowledge.

Use --dry-run to preview what would happen without processing items or
delivering outputs. Use --channel to restrict processing to a single
channel adapter.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if FeedbackLoop == nil {
			return fmt.Errorf("feedback loop not initialized")
		}

		opts := core.RunOptions{
			DryRun:        loopDryRun,
			ChannelFilter: loopChannel,
		}

		result, err := FeedbackLoop.Run(opts)
		if err != nil {
			return fmt.Errorf("running feedback loop: %w", err)
		}

		if loopDryRun {
			fmt.Println("Feedback loop dry run completed:")
		} else {
			fmt.Println("Feedback loop completed:")
		}
		fmt.Printf("  Items fetched:      %d\n", result.ItemsFetched)
		fmt.Printf("  Items processed:    %d\n", result.ItemsProcessed)
		fmt.Printf("  Outputs delivered:  %d\n", result.OutputsDelivered)
		fmt.Printf("  Knowledge added:    %d\n", result.KnowledgeAdded)
		fmt.Printf("  Skipped:            %d\n", result.Skipped)

		if len(result.Errors) > 0 {
			fmt.Printf("\nWarnings (%d):\n", len(result.Errors))
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		return nil
	},
}

func init() {
	loopCmd.Flags().BoolVar(&loopDryRun, "dry-run", false, "Show what would happen without processing or delivering")
	loopCmd.Flags().StringVar(&loopChannel, "channel", "", "Only process items from a specific channel adapter")
	rootCmd.AddCommand(loopCmd)
}
