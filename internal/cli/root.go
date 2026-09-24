package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type RootOptions struct {
	IncludeInit bool
	IncludeOrg  bool
}

// NewRootCmd creates the root command for the ADB CLI
func NewRootCmd() *cobra.Command {
	return NewRootCmdWithOptions(RootOptions{
		IncludeInit: true,
		IncludeOrg:  true,
	})
}

// NewRootCmdWithOptions creates the legacy command tree for staged cutovers.
// Callers replacing a top-level capability can omit its legacy spelling while
// retaining every other command.
func NewRootCmdWithOptions(options RootOptions) *cobra.Command {
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
	// The `sync` namespace was redistributed by noun (TASK-00039 Q4). Each new
	// parent is registered here; the retired spellings stay reachable as hidden
	// deprecated alias trees so no existing script breaks.
	rootCmd.AddCommand(NewContextCmd())
	rootCmd.AddCommand(NewWikiCmd())
	rootCmd.AddCommand(NewIssuesCmd())
	rootCmd.AddCommand(NewArchiveCmd())
	rootCmd.AddCommand(NewHarnessCmd())
	rootCmd.AddCommand(newRetiredSyncCmd())
	rootCmd.AddCommand(newRetiredMemoryCmd())
	rootCmd.AddCommand(newRetiredPluginCmd())
	rootCmd.AddCommand(newRetiredReposCmd())
	if options.IncludeInit {
		rootCmd.AddCommand(NewInitCmd())
	}
	rootCmd.AddCommand(NewMetricsCmd())
	rootCmd.AddCommand(NewAlertsCmd())
	rootCmd.AddCommand(NewEventsCmd())
	rootCmd.AddCommand(NewHookCmd())
	rootCmd.AddCommand(NewVersionCmd())
	rootCmd.AddCommand(NewMCPCmd())
	rootCmd.AddCommand(NewPromptCmd())
	rootCmd.AddCommand(NewCommCmd())
	rootCmd.AddCommand(NewSchedulerCmd())
	rootCmd.AddCommand(NewScheduleCmd())
	rootCmd.AddCommand(NewIngestCmd())
	if options.IncludeOrg {
		rootCmd.AddCommand(NewOrgCmd())
	}
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
	rootCmd.AddCommand(NewProgramCmd())
	rootCmd.AddCommand(NewTemplatesCmd())
	rootCmd.AddCommand(NewGovernanceCmd())
	// Top-level `status` and `work` left the visible surface in TASK-00039 (Q7):
	// both were views over tasks, so they moved onto the `task` noun as
	// `task list --git` and `task worktree …`. Hidden aliases keep them working.
	rootCmd.AddCommand(newRetiredStatusCmd())
	rootCmd.AddCommand(newRetiredWorkCmd())

	return rootCmd
}
