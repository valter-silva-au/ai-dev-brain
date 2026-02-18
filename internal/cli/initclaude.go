package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initClaudeCmd = &cobra.Command{
	Use:   "init-claude [path]",
	Short: "Bootstrap Claude Code configuration for a repository",
	Long: `Bootstrap a .claude/ directory for a repository using adb's canonical
templates. This creates:

  - .claudeignore with sensible defaults for the project type
  - .claude/settings.json with safe permission defaults
  - .claude/rules/workspace.md with project conventions

Safe to run on existing projects -- files that already exist are skipped
and not overwritten. Templates are sourced from the adb installation's
templates/claude/ directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 {
			targetPath = args[0]
		}
		absPath, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		// Resolve template directory relative to adb base path
		templateDir := filepath.Join(BasePath, "repos", "github.com", "valter-silva-au", "ai-dev-brain", "templates", "claude")
		if _, err := os.Stat(templateDir); os.IsNotExist(err) {
			// Fall back to checking if templates are bundled alongside the binary
			execPath, execErr := os.Executable()
			if execErr == nil {
				templateDir = filepath.Join(filepath.Dir(execPath), "templates", "claude")
			}
			if _, err := os.Stat(templateDir); os.IsNotExist(err) {
				return fmt.Errorf("template directory not found: run from an adb workspace or set ADB_HOME")
			}
		}

		var created, skipped []string

		// Create .claudeignore
		ignPath := filepath.Join(absPath, ".claudeignore")
		if err := copyTemplateIfNotExists(
			filepath.Join(templateDir, "claudeignore.template"),
			ignPath,
			&created, &skipped,
		); err != nil {
			return fmt.Errorf("creating .claudeignore: %w", err)
		}

		// Create .claude/settings.json
		settingsDir := filepath.Join(absPath, ".claude")
		if err := os.MkdirAll(settingsDir, 0o755); err != nil {
			return fmt.Errorf("creating .claude directory: %w", err)
		}
		settingsPath := filepath.Join(settingsDir, "settings.json")
		if err := copyTemplateIfNotExists(
			filepath.Join(templateDir, "settings.template.json"),
			settingsPath,
			&created, &skipped,
		); err != nil {
			return fmt.Errorf("creating settings.json: %w", err)
		}

		// Create .claude/rules/workspace.md
		rulesDir := filepath.Join(settingsDir, "rules")
		if err := os.MkdirAll(rulesDir, 0o755); err != nil {
			return fmt.Errorf("creating rules directory: %w", err)
		}
		workspacePath := filepath.Join(rulesDir, "workspace.md")
		if err := copyTemplateIfNotExists(
			filepath.Join(templateDir, "rules", "workspace.template.md"),
			workspacePath,
			&created, &skipped,
		); err != nil {
			return fmt.Errorf("creating workspace.md: %w", err)
		}

		if len(created) > 0 {
			fmt.Println("Created:")
			for _, p := range created {
				rel, _ := filepath.Rel(absPath, p)
				fmt.Printf("  %s\n", rel)
			}
		}
		if len(skipped) > 0 {
			fmt.Println("Skipped (already exist):")
			for _, p := range skipped {
				rel, _ := filepath.Rel(absPath, p)
				fmt.Printf("  %s\n", rel)
			}
		}

		fmt.Printf("\nClaude Code configuration initialized at %s\n", absPath)
		return nil
	},
}

// copyTemplateIfNotExists copies src to dst if dst does not already exist.
// Appends to created or skipped accordingly.
func copyTemplateIfNotExists(src, dst string, created, skipped *[]string) error {
	if _, err := os.Stat(dst); err == nil {
		*skipped = append(*skipped, dst)
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	*created = append(*created, dst)
	return nil
}

func init() {
	rootCmd.AddCommand(initClaudeCmd)
}
