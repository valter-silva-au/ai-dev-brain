package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// Executor is the CLIExecutor used by the exec command.
// Set during application wiring (Task #43).
var Executor integration.CLIExecutor

// ExecAliases holds the configured CLI aliases.
// Populated during application wiring.
var ExecAliases []integration.CLIAlias

// ExecTaskCtx holds the active task context, if any.
// Populated during application wiring when a task is active.
var ExecTaskCtx *integration.TaskEnvContext

var execCmd = &cobra.Command{
	Use:   "exec [cli] [args...]",
	Short: "Execute an external CLI tool with alias resolution and task context",
	Long: `Execute an external CLI tool, resolving configured aliases and injecting
task environment variables (ADB_TASK_ID, ADB_BRANCH, etc.) when a task is active.

With no arguments, lists all configured CLI aliases.`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle --help / -h manually since DisableFlagParsing is true.
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}

		// Handle --simple flag: set CLAUDE_CODE_SIMPLE=1 for reduced output.
		if len(args) > 0 && args[0] == "--simple" {
			_ = os.Setenv("CLAUDE_CODE_SIMPLE", "1")
			args = args[1:]
		}

		if Executor == nil {
			return fmt.Errorf("CLI executor not initialized")
		}

		// No arguments: list aliases.
		if len(args) == 0 {
			aliases := Executor.ListAliases(ExecAliases)
			if len(aliases) == 0 {
				fmt.Println("No CLI aliases configured.")
				fmt.Println("Add aliases in .taskconfig under cli_aliases.")
				return nil
			}
			fmt.Println("Configured CLI aliases:")
			for _, a := range aliases {
				fmt.Printf("  %s\n", a)
			}
			return nil
		}

		// Execute the CLI tool.
		cliName := args[0]
		cliArgs := args[1:]

		config := integration.CLIExecConfig{
			CLI:     cliName,
			Args:    cliArgs,
			TaskCtx: ExecTaskCtx,
			Aliases: ExecAliases,
			Stdin:   os.Stdin,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
		}

		result, err := Executor.Exec(config)
		if err != nil {
			return fmt.Errorf("exec %s: %w", cliName, err)
		}

		if result.ExitCode != 0 {
			osExit(result.ExitCode)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
