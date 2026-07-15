package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command for the ADB CLI
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "adb",
		Short: "AI Dev Brain - Task management and workflow automation",
		Long: `AI Dev Brain (adb) is a task management and workflow automation tool
that integrates with git worktrees, Claude Code, and terminal environments.`,
		Version:      fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
		SilenceUsage: true,
	}

	// Add subcommands
	rootCmd.AddCommand(NewTaskCmd())
	rootCmd.AddCommand(NewSessionCmd())
	rootCmd.AddCommand(NewSyncCmd())
	rootCmd.AddCommand(NewInitCmd())
	rootCmd.AddCommand(NewExecCmd())
	rootCmd.AddCommand(NewRunCmd())
	rootCmd.AddCommand(NewMetricsCmd())
	rootCmd.AddCommand(NewAlertsCmd())
	rootCmd.AddCommand(NewEventsCmd())
	rootCmd.AddCommand(NewChatCmd())
	rootCmd.AddCommand(NewDashboardCmd())
	rootCmd.AddCommand(NewHookCmd())
	rootCmd.AddCommand(NewVersionCmd())
	rootCmd.AddCommand(NewTeamCmd())
	rootCmd.AddCommand(NewAgentsCmd())
	rootCmd.AddCommand(NewMCPCmd())
	rootCmd.AddCommand(NewPromptCmd())
	rootCmd.AddCommand(NewMemoryCmd())
	rootCmd.AddCommand(NewCommCmd())
	rootCmd.AddCommand(NewReposCmd())
	rootCmd.AddCommand(NewSchedulerCmd())
	rootCmd.AddCommand(NewScheduleCmd())
	rootCmd.AddCommand(NewIngestCmd())
	rootCmd.AddCommand(NewOrgCmd())
	rootCmd.AddCommand(NewInitiativeCmd())
	rootCmd.AddCommand(NewStageCmd())
	rootCmd.AddCommand(NewGraphCmd())
	rootCmd.AddCommand(NewPMFCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewCatalogCmd())
	rootCmd.AddCommand(NewConformanceCmd())
	rootCmd.AddCommand(NewADRCmd())
	rootCmd.AddCommand(NewDebtCmd())
	rootCmd.AddCommand(NewAuditCmd())
	rootCmd.AddCommand(NewComplianceCmd())
	rootCmd.AddCommand(NewSLOCmd())
	rootCmd.AddCommand(NewCRMCmd())
	rootCmd.AddCommand(NewGTMCmd())
	rootCmd.AddCommand(NewGovernanceCmd())
	rootCmd.AddCommand(NewPluginCmd())
	rootCmd.AddCommand(NewStatusCmd())
	rootCmd.AddCommand(NewWorkCmd())
	rootCmd.AddCommand(NewSerenaCmd())

	return rootCmd
}
