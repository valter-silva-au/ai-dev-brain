package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	claudetpl "github.com/valter-silva-au/ai-dev-brain/templates/claude"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Handle Claude Code hook events",
	Long: `Process Claude Code hook events and update adb artifacts.

Each subcommand handles a specific hook type by reading JSON from stdin
and performing the appropriate actions (validation, formatting, tracking,
quality checks, knowledge extraction).

These commands are called by shell wrapper scripts installed in .claude/hooks/.`,
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install adb hook wrappers for Claude Code",
	Long: `Generate shell wrapper scripts and update .claude/settings.json
to use adb-native hooks instead of standalone shell scripts.

This creates .claude/hooks/ wrapper scripts that delegate to 'adb hook <type>'
and configures Claude Code to use them.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, _ := cmd.Flags().GetString("dir")
		if targetDir == "" {
			var err error
			targetDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		return installHookWrappers(targetDir)
	},
}

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hook configuration status",
	Long:  `Display which adb hooks are enabled and their current configuration.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if HookEngine == nil {
			fmt.Println("Hook engine not initialized.")
			return nil
		}

		// Load and display hook config from the engine.
		// For now, read directly from the config manager.
		if BasePath == "" {
			fmt.Println("No adb workspace found.")
			return nil
		}

		cfg, err := loadHookConfigFromDisk()
		if err != nil {
			fmt.Println("Using default hook configuration (no .taskconfig hooks section).")
			return nil
		}

		fmt.Printf("Hook system: %s\n\n", enabledStr(cfg.Enabled))
		fmt.Printf("PreToolUse:     %s\n", enabledStr(cfg.PreToolUse.Enabled))
		fmt.Printf("  block_vendor:       %s\n", enabledStr(cfg.PreToolUse.BlockVendor))
		fmt.Printf("  block_go_sum:       %s\n", enabledStr(cfg.PreToolUse.BlockGoSum))
		fmt.Printf("  architecture_guard: %s\n", enabledStr(cfg.PreToolUse.ArchitectureGuard))
		fmt.Printf("  adr_conflict_check: %s\n", enabledStr(cfg.PreToolUse.ADRConflictCheck))
		fmt.Println()
		fmt.Printf("PostToolUse:    %s\n", enabledStr(cfg.PostToolUse.Enabled))
		fmt.Printf("  go_format:            %s\n", enabledStr(cfg.PostToolUse.GoFormat))
		fmt.Printf("  change_tracking:      %s\n", enabledStr(cfg.PostToolUse.ChangeTracking))
		fmt.Printf("  dependency_detection: %s\n", enabledStr(cfg.PostToolUse.DependencyDetection))
		fmt.Println()
		fmt.Printf("Stop:           %s\n", enabledStr(cfg.Stop.Enabled))
		fmt.Printf("  uncommitted_check: %s\n", enabledStr(cfg.Stop.UncommittedCheck))
		fmt.Printf("  build_check:       %s\n", enabledStr(cfg.Stop.BuildCheck))
		fmt.Printf("  vet_check:         %s\n", enabledStr(cfg.Stop.VetCheck))
		fmt.Printf("  context_update:    %s\n", enabledStr(cfg.Stop.ContextUpdate))
		fmt.Printf("  status_timestamp:  %s\n", enabledStr(cfg.Stop.StatusTimestamp))
		fmt.Println()
		fmt.Printf("TaskCompleted:  %s\n", enabledStr(cfg.TaskCompleted.Enabled))
		fmt.Printf("  check_uncommitted: %s\n", enabledStr(cfg.TaskCompleted.CheckUncommitted))
		fmt.Printf("  run_tests:         %s\n", enabledStr(cfg.TaskCompleted.RunTests))
		fmt.Printf("  run_lint:          %s\n", enabledStr(cfg.TaskCompleted.RunLint))
		fmt.Printf("  test_command:      %s\n", cfg.TaskCompleted.TestCommand)
		fmt.Printf("  lint_command:      %s\n", cfg.TaskCompleted.LintCommand)
		fmt.Printf("  extract_knowledge: %s\n", enabledStr(cfg.TaskCompleted.ExtractKnowledge))
		fmt.Printf("  update_wiki:       %s\n", enabledStr(cfg.TaskCompleted.UpdateWiki))
		fmt.Printf("  generate_adrs:     %s\n", enabledStr(cfg.TaskCompleted.GenerateADRs))
		fmt.Printf("  update_context:    %s\n", enabledStr(cfg.TaskCompleted.UpdateContext))
		fmt.Println()
		fmt.Printf("SessionEnd:     %s\n", enabledStr(cfg.SessionEnd.Enabled))
		fmt.Printf("  capture_session:    %s\n", enabledStr(cfg.SessionEnd.CaptureSession))
		fmt.Printf("  min_turns_capture:  %d\n", cfg.SessionEnd.MinTurnsCapture)
		fmt.Printf("  update_context:     %s\n", enabledStr(cfg.SessionEnd.UpdateContext))
		fmt.Printf("  extract_knowledge:  %s\n", enabledStr(cfg.SessionEnd.ExtractKnowledge))
		fmt.Printf("  log_communications: %s\n", enabledStr(cfg.SessionEnd.LogCommunications))

		return nil
	},
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func loadHookConfigFromDisk() (*models.HookConfig, error) {
	if BasePath == "" {
		return nil, fmt.Errorf("no base path")
	}
	cfgMgr := core.NewConfigurationManager(BasePath)
	globalCfg, err := cfgMgr.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}
	cfg := globalCfg.Hooks
	if cfg == (models.HookConfig{}) {
		cfg = models.DefaultHookConfig()
	}
	return &cfg, nil
}

// installHookWrappers writes shell wrappers and updates settings.json.
func installHookWrappers(targetDir string) error {
	hooksDir := filepath.Join(targetDir, ".claude", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	// Write shell wrapper templates from embedded FS.
	hookFiles := []string{
		"adb-hook-pre-tool-use.sh",
		"adb-hook-post-tool-use.sh",
		"adb-hook-stop.sh",
		"adb-hook-task-completed.sh",
		"adb-hook-session-end.sh",
	}

	for _, name := range hookFiles {
		data, err := claudetpl.FS.ReadFile("hooks/" + name)
		if err != nil {
			return fmt.Errorf("reading embedded template %s: %w", name, err)
		}
		dest := filepath.Join(hooksDir, name)
		if err := os.WriteFile(dest, data, 0o755); err != nil {
			return fmt.Errorf("writing hook script %s: %w", name, err)
		}
		fmt.Printf("  Wrote %s\n", dest)
	}

	// Update settings.json with hook entries.
	settingsPath := filepath.Join(targetDir, ".claude", "settings.json")
	if err := updateSettingsWithHooks(settingsPath, hooksDir); err != nil {
		return fmt.Errorf("updating settings.json: %w", err)
	}

	fmt.Printf("\nHook wrappers installed in %s\n", hooksDir)
	fmt.Println("Claude Code will now use adb-native hooks.")
	return nil
}

func updateSettingsWithHooks(settingsPath, hooksDir string) error {
	// Read existing settings or create new.
	var settings map[string]interface{}

	data, err := os.ReadFile(settingsPath) //nolint:gosec // G304: path from trusted CLI input
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	} else {
		settings = make(map[string]interface{})
	}

	// Build hooks section.
	hooksSection := map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"type":     "command",
				"command":  filepath.Join(hooksDir, "adb-hook-pre-tool-use.sh"),
				"triggers": []string{"Edit", "Write"},
			},
		},
		"PostToolUse": []interface{}{
			map[string]interface{}{
				"type":     "command",
				"command":  filepath.Join(hooksDir, "adb-hook-post-tool-use.sh"),
				"triggers": []string{"Edit", "Write"},
			},
		},
		"Stop": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": filepath.Join(hooksDir, "adb-hook-stop.sh"),
			},
		},
		"TaskCompleted": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": filepath.Join(hooksDir, "adb-hook-task-completed.sh"),
			},
		},
		"SessionEnd": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": filepath.Join(hooksDir, "adb-hook-session-end.sh"),
			},
		},
	}

	settings["hooks"] = hooksSection

	// Write back settings.
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	// Ensure trailing newline.
	if !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}
	fmt.Printf("  Updated %s\n", settingsPath)
	return nil
}

func init() {
	hookInstallCmd.Flags().String("dir", "", "Target directory (defaults to current directory)")

	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookStatusCmd)
	rootCmd.AddCommand(hookCmd)
}
