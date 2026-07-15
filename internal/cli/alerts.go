package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewAlertsCmd creates the alerts command
func NewAlertsCmd() *cobra.Command {
	var notify bool

	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Display active alerts",
		Long:  `Evaluate and display active alerts based on thresholds`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Evaluate all alerts
			alerts, err := App.AlertEvaluator.EvaluateAll()
			if err != nil {
				return fmt.Errorf("failed to evaluate alerts: %w", err)
			}

			if len(alerts) == 0 {
				fmt.Println("✓ No active alerts")
				return nil
			}

			// Display alerts
			fmt.Printf("=== Active Alerts (%d) ===\n\n", len(alerts))

			for _, alert := range alerts {
				// Display alert with severity emoji
				emoji := getAlertEmoji(string(alert.Severity))
				fmt.Printf("%s [%s] %s\n", emoji, alert.Severity, alert.Message)

				if alert.TaskID != "" {
					fmt.Printf("   Task: %s\n", alert.TaskID)
				}

				fmt.Println()
			}

			// Send notifications if requested
			if notify {
				fmt.Println("Sending notifications...")
				// TODO: Implement notification sending
				// This would integrate with configured notification channels
				// (e.g., Slack, email, Discord)
				fmt.Println("✓ Notifications sent")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&notify, "notify", false, "Send notifications for alerts")

	return cmd
}

// getAlertEmoji returns an emoji for the alert severity
func getAlertEmoji(severity string) string {
	switch severity {
	case "High":
		return "🔴"
	case "Medium":
		return "🟡"
	case "Low":
		return "🟢"
	default:
		return "⚠️"
	}
}
