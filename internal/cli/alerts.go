package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Show active alerts and warnings",
	Long: `Evaluate alert conditions against the event log and display any triggered alerts.

Alerts check for blocked tasks, stale tasks, long-running reviews, and backlog size.

Use --notify to send alerts to configured notification channels (e.g. Slack).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if AlertEngine == nil {
			return fmt.Errorf("alert engine not initialized (observability may be disabled)")
		}

		alerts, err := AlertEngine.Evaluate()
		if err != nil {
			return fmt.Errorf("evaluating alerts: %w", err)
		}

		if len(alerts) == 0 {
			fmt.Println("No active alerts.")
			return nil
		}

		fmt.Printf("%d active alert(s):\n\n", len(alerts))
		for _, alert := range alerts {
			severity := strings.ToUpper(string(alert.Severity))
			fmt.Printf("  [%s] %s\n", severity, alert.Message)
			fmt.Printf("         triggered at %s\n\n", alert.TriggeredAt.Format("2006-01-02 15:04 UTC"))
		}

		notify, _ := cmd.Flags().GetBool("notify")
		if notify {
			if Notifier == nil {
				return fmt.Errorf("notifier not configured (set notifications.enabled and slack.webhook_url in .taskconfig)")
			}
			if err := Notifier.Notify(alerts); err != nil {
				return fmt.Errorf("sending notifications: %w", err)
			}
			fmt.Println("Notifications sent.")
		}

		return nil
	},
}

func init() {
	alertsCmd.Flags().Bool("notify", false, "Send alerts to configured notification channels")
	rootCmd.AddCommand(alertsCmd)
}
