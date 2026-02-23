package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// ProjectInit is the ProjectInitializer used by the init command.
// Set during application wiring.
var ProjectInit core.ProjectInitializer

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new adb project workspace",
	Long: `Initialize a new or existing directory with the full recommended
adb workspace structure including task configuration, backlog registry,
ticket directories, documentation templates, and tool directories.

Safe to run on existing projects -- files and directories that already
exist are skipped and not overwritten.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if ProjectInit == nil {
			return fmt.Errorf("project initializer not initialized")
		}

		basePath := "."
		if len(args) > 0 {
			basePath = args[0]
		}
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		name, _ := cmd.Flags().GetString("name")
		ai, _ := cmd.Flags().GetString("ai")
		prefix, _ := cmd.Flags().GetString("prefix")

		if name == "" {
			name = filepath.Base(absPath)
		}

		result, err := ProjectInit.Init(core.InitConfig{
			BasePath: absPath,
			Name:     name,
			AI:       ai,
			Prefix:   prefix,
		})
		if err != nil {
			return fmt.Errorf("initializing project: %w", err)
		}

		if len(result.Created) > 0 {
			fmt.Println("Created:")
			for _, p := range result.Created {
				rel, _ := filepath.Rel(absPath, p)
				fmt.Printf("  %s\n", rel)
			}
		}
		if len(result.Skipped) > 0 {
			fmt.Println("Skipped (already exist):")
			for _, p := range result.Skipped {
				rel, _ := filepath.Rel(absPath, p)
				fmt.Printf("  %s\n", rel)
			}
		}

		fmt.Printf("\nProject %q initialized at %s\n", name, absPath)
		return nil
	},
}

func init() {
	initCmd.Flags().String("name", "", "Project name (defaults to directory basename)")
	initCmd.Flags().String("ai", "claude", "Default AI assistant type")
	initCmd.Flags().String("prefix", "TASK", "Task ID prefix")
	rootCmd.AddCommand(initCmd)
}
