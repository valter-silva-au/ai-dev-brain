package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// NewMetricsCmd creates the metrics command
func NewMetricsCmd() *cobra.Command {
	var (
		jsonOutput bool
		since      string
	)

	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Display workspace metrics",
		Long:  `Display metrics derived from the event log`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Compute metrics — windowed when --since is given, else all-time.
			var metrics *observability.Metrics
			if since != "" {
				duration, err := parseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid duration format: %w", err)
				}
				// Tally ONLY events within the window. (The old code replayed the
				// whole log and merely blanked the result when the newest event
				// predated the cutoff, so --since returned all-time counts unless
				// the workspace was idle — #153.)
				cutoff := time.Now().UTC().Add(-duration)
				metrics, err = App.MetricsCalculator.ComputeMetricsSince(cutoff)
				if err != nil {
					return fmt.Errorf("failed to compute metrics: %w", err)
				}
			} else {
				var err error
				metrics, err = App.MetricsCalculator.ComputeMetrics()
				if err != nil {
					return fmt.Errorf("failed to compute metrics: %w", err)
				}
			}

			// Output in JSON or human-readable format
			if jsonOutput {
				data, err := json.MarshalIndent(metrics, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal metrics: %w", err)
				}
				fmt.Println(string(data))
			} else {
				printMetrics(metrics)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().StringVar(&since, "since", "", "Show metrics since duration (e.g., 7d, 24h)")

	return cmd
}

// printMetrics prints metrics in human-readable format
func printMetrics(m *observability.Metrics) {
	fmt.Println("=== Workspace Metrics ===")
	fmt.Println()

	fmt.Printf("Tasks Created:     %d\n", m.TasksCreated)
	fmt.Printf("Tasks Completed:   %d\n", m.TasksCompleted)
	fmt.Printf("Agent Sessions:    %d\n", m.AgentSessions)
	fmt.Printf("Knowledge Items:   %d\n", m.KnowledgeExtracts)
	fmt.Printf("Worktrees Created: %d\n", m.WorktreesCreated)
	fmt.Printf("Worktrees Removed: %d\n", m.WorktreesRemoved)

	if !m.LastEventTimestamp.IsZero() {
		fmt.Printf("Last Event:        %s\n", m.LastEventTimestamp.Format(time.RFC3339))
	}

	if len(m.TasksByStatus) > 0 {
		fmt.Println("\n--- Tasks by Status ---")
		for status, count := range m.TasksByStatus {
			fmt.Printf("  %s: %d\n", status, count)
		}
	}

	if len(m.TasksByType) > 0 {
		fmt.Println("\n--- Tasks by Type ---")
		for taskType, count := range m.TasksByType {
			fmt.Printf("  %s: %d\n", taskType, count)
		}
	}
}

// parseDuration parses duration strings like "7d", "24h", "30m"
func parseDuration(s string) (time.Duration, error) {
	// Support days (d) format
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid day format: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// Use standard time.ParseDuration for other formats
	return time.ParseDuration(s)
}
