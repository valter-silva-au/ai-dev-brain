package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	metricsJSON  bool
	metricsSince string
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Display task and agent metrics",
	Long: `Display aggregated metrics derived from the event log.

Metrics include task creation/completion counts, tasks by status and type,
agent session counts, and knowledge extraction counts.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if MetricsCalc == nil {
			return fmt.Errorf("metrics calculator not initialized (observability may be disabled)")
		}

		sinceTime, err := parseSinceDuration(metricsSince)
		if err != nil {
			return fmt.Errorf("parsing --since: %w", err)
		}

		metrics, err := MetricsCalc.Calculate(sinceTime)
		if err != nil {
			return fmt.Errorf("calculating metrics: %w", err)
		}

		if metricsJSON {
			data, err := json.MarshalIndent(metrics, "", "  ")
			if err != nil {
				return fmt.Errorf("formatting metrics as JSON: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}

		// Table format.
		fmt.Printf("Metrics (since %s)\n\n", sinceTime.Format("2006-01-02"))
		fmt.Printf("  %-24s %d\n", "Events recorded:", metrics.EventCount)
		fmt.Printf("  %-24s %d\n", "Tasks created:", metrics.TasksCreated)
		fmt.Printf("  %-24s %d\n", "Tasks completed:", metrics.TasksCompleted)
		fmt.Printf("  %-24s %d\n", "Agent sessions:", metrics.AgentSessions)
		fmt.Printf("  %-24s %d\n", "Knowledge extracted:", metrics.KnowledgeExtracted)

		if len(metrics.TasksByType) > 0 {
			fmt.Println("\n  Tasks by type:")
			for taskType, count := range metrics.TasksByType {
				fmt.Printf("    %-20s %d\n", taskType+":", count)
			}
		}

		if len(metrics.TasksByStatus) > 0 {
			fmt.Println("\n  Status transitions:")
			for status, count := range metrics.TasksByStatus {
				fmt.Printf("    %-20s %d\n", status+":", count)
			}
		}

		if metrics.OldestEvent != nil {
			fmt.Printf("\n  %-24s %s\n", "Oldest event:", metrics.OldestEvent.Format(time.RFC3339))
		}
		if metrics.NewestEvent != nil {
			fmt.Printf("  %-24s %s\n", "Newest event:", metrics.NewestEvent.Format(time.RFC3339))
		}

		return nil
	},
}

// parseSinceDuration parses a human-friendly duration string like "7d", "30d",
// or "24h" and returns the corresponding time in the past.
func parseSinceDuration(s string) (time.Time, error) {
	now := time.Now().UTC()
	s = strings.TrimSpace(s)
	if s == "" {
		return now.AddDate(0, 0, -7), nil
	}

	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid day duration %q", s)
		}
		return now.AddDate(0, 0, -days), nil
	}

	if strings.HasSuffix(s, "h") {
		hours, err := strconv.Atoi(strings.TrimSuffix(s, "h"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid hour duration %q", s)
		}
		return now.Add(-time.Duration(hours) * time.Hour), nil
	}

	return time.Time{}, fmt.Errorf("unsupported duration format %q (use e.g. 7d, 30d, 24h)", s)
}

func init() {
	metricsCmd.Flags().BoolVar(&metricsJSON, "json", false, "Output metrics as JSON")
	metricsCmd.Flags().StringVar(&metricsSince, "since", "7d", "Time window for metrics (e.g. 7d, 30d, 24h)")
	rootCmd.AddCommand(metricsCmd)
}
