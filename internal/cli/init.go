package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// NewInitCmd creates the init command with all subcommands
func NewInitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize workspace",
		Long:  `Initialize a new AI Dev Brain workspace with scaffolding`,
	}

	// Add subcommands
	initCmd.AddCommand(
		newInitWorkspaceCmd(),
		newInitClaudeCmd(),
		newInitProjectCmd(),
		newInitUpdateCmd(),
	)

	return initCmd
}

// newInitUpdateCmd creates the 'init update' command — the copier/cruft-style
// re-sync of a scaffolded project to the current template version (#128).
func newInitUpdateCmd() *cobra.Command {
	var (
		apply bool
		force bool
	)
	cmd := &cobra.Command{
		Use:   "update [path]",
		Short: "Re-sync a scaffolded project to the current template version",
		Long: `Diff a project scaffolded by 'adb init project' against the current template
set and (with --apply) update it. Uses the .adb/template-manifest.yaml provenance
recorded at scaffold time for a three-way diff:

  added     — a file the current templates add
  updated   — a template change to a file you have NOT edited (safe to apply)
  conflict  — a template change to a file you HAVE edited (skipped unless --force)
  unchanged — no template change since your last sync

Dry-run by default; pass --apply to write. --force also overwrites conflicts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			pi := core.NewFileProjectInitializer(claude.FS)
			var plan *core.UpdatePlan
			if apply {
				plan, err = pi.ApplyUpdate(absPath, force)
			} else {
				plan, err = pi.PlanUpdate(absPath)
			}
			if err != nil {
				return fmt.Errorf("template update failed: %w", err)
			}

			mode := "Plan"
			if apply {
				mode = "Applied"
			}
			fmt.Printf("%s template update %s → %s\n", mode, plan.FromVersion, plan.ToVersion)
			printUpdateGroup("added", plan.Added)
			printUpdateGroup("updated", plan.Updated)
			printUpdateGroup("conflict", plan.Conflicts)
			printUpdateGroup("unchanged", plan.Unchanged)

			if !apply {
				if plan.HasChanges() || len(plan.Conflicts) > 0 {
					fmt.Println("\nRun with --apply to write (add --force to overwrite conflicts).")
				} else {
					fmt.Println("\nAlready up to date.")
				}
			} else if len(plan.Conflicts) > 0 && !force {
				fmt.Printf("\n%d conflict(s) skipped — re-run with --apply --force to overwrite.\n", len(plan.Conflicts))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the update (default: dry-run diff)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite conflicted (user-modified) files too")
	return cmd
}

// printUpdateGroup prints one status group of an update plan (nothing when empty).
func printUpdateGroup(label string, files []string) {
	if len(files) == 0 {
		return
	}
	fmt.Printf("  %s (%d):\n", label, len(files))
	for _, f := range files {
		fmt.Printf("    %s\n", f)
	}
}

// newInitWorkspaceCmd creates the 'init' command for full workspace scaffolding
func newInitWorkspaceCmd() *cobra.Command {
	var (
		name     string
		ai       string
		prefix   string
		buildCmd string
		testCmd  string
	)

	cmd := &cobra.Command{
		Use:   "workspace [path]",
		Short: "Initialize a new workspace",
		Long:  `Initialize a new AI Dev Brain workspace with full scaffolding`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}

			// Create absolute path
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			// Check if directory exists
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				if err := os.MkdirAll(absPath, 0o755); err != nil {
					return fmt.Errorf("failed to create directory: %w", err)
				}
			}

			fmt.Printf("Initializing workspace at %s...\n", absPath)

			// Create directory structure
			dirs := []string{
				"tickets",
				"work",
				"sessions",
				".adb",
			}

			for _, dir := range dirs {
				dirPath := filepath.Join(absPath, dir)
				if err := os.MkdirAll(dirPath, 0o755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
				fmt.Printf("  ✓ Created %s/\n", dir)
			}

			// Create initial backlog.yaml
			backlogPath := filepath.Join(absPath, "backlog.yaml")
			if _, err := os.Stat(backlogPath); os.IsNotExist(err) {
				backlogContent := "tasks: []\n"
				if err := os.WriteFile(backlogPath, []byte(backlogContent), 0o644); err != nil {
					return fmt.Errorf("failed to create backlog.yaml: %w", err)
				}
				fmt.Println("  ✓ Created backlog.yaml")
			}

			// Create .taskrc config file
			taskrcPath := filepath.Join(absPath, ".taskrc")
			if _, err := os.Stat(taskrcPath); os.IsNotExist(err) {
				workspaceName := name
				if workspaceName == "" {
					workspaceName = filepath.Base(absPath)
				}

				taskIDPrefix := prefix
				if taskIDPrefix == "" {
					taskIDPrefix = "TASK"
				}

				// YAML-encode each user value (core.YAMLScalar emits its own
				// quoting) so a double-quote/backslash/colon in --name or
				// --build-command can't produce an invalid .taskrc that bricks
				// every later command (#156).
				//
				// Keys here MUST match the flat RepoConfig schema the binary
				// actually parses (pkg/models/config.go): repo_name,
				// task_id_prefix, build_command, test_command, base_branch,
				// worktree_base_path. The pre-#… template emitted `name:` +
				// nested `build:`/`git:` blocks that RepoConfig has no fields
				// for, so Viper silently dropped them — `adb config show`
				// reported an empty repo_name and the build/test commands never
				// reached the TaskCompleted gate. `ai_provider` is intentionally
				// omitted: it is not a RepoConfig field either.
				taskrcContent := fmt.Sprintf(`# AI Dev Brain Repository Configuration
# This file configures the workspace for AI-assisted development.
# These are the flat keys the adb binary reads (pkg/models/config.go RepoConfig).

repo_name: %s
task_id_prefix: %s

# Build/test/lint commands are project-specific and drive adb's TaskCompleted
# quality gate. Set them to whatever your stack uses (e.g. "npm run build" /
# "npm test", or "cargo build" / "cargo test"). Empty means "no command".
build_command: %s
test_command: %s
lint_command: ""

base_branch: "main"
worktree_base_path: "work"
`, core.YAMLScalar(workspaceName), core.YAMLScalar(taskIDPrefix), core.YAMLScalar(buildCmd), core.YAMLScalar(testCmd))

				if err := os.WriteFile(taskrcPath, []byte(taskrcContent), 0o644); err != nil {
					return fmt.Errorf("failed to create .taskrc: %w", err)
				}
				fmt.Println("  ✓ Created .taskrc")
			}

			// Create README.md
			readmePath := filepath.Join(absPath, "README.md")
			if _, err := os.Stat(readmePath); os.IsNotExist(err) {
				workspaceName := name
				if workspaceName == "" {
					workspaceName = filepath.Base(absPath)
				}

				readmeContent := "# " + workspaceName + "\n\n" +
					"This workspace is managed by [AI Dev Brain](https://github.com/valter-silva-au/ai-dev-brain).\n\n" +
					"## Quick Start\n\n" +
					"- Create a task: `adb task create <branch-name>`\n" +
					"- Resume a task: `adb task resume <task-id>`\n" +
					"- View tasks: `adb task status`\n\n" +
					"## Structure\n\n" +
					"- `tickets/` - Task-specific context and notes\n" +
					"- `work/` - Git worktrees for task isolation\n" +
					"- `backlog.yaml` - Task backlog\n" +
					"- `.taskrc` - Workspace configuration\n"

				if err := os.WriteFile(readmePath, []byte(readmeContent), 0o644); err != nil {
					return fmt.Errorf("failed to create README.md: %w", err)
				}
				fmt.Println("  ✓ Created README.md")
			}

			fmt.Printf("\n✓ Workspace initialized at %s\n", absPath)
			fmt.Println("\nNext steps:")
			fmt.Printf("  cd %s\n", absPath)
			fmt.Println("  adb task create <branch-name>")

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Workspace name")
	cmd.Flags().StringVar(&ai, "ai", "claude", "AI provider (claude, gpt)")
	cmd.Flags().StringVar(&prefix, "prefix", "TASK", "Task ID prefix")
	cmd.Flags().StringVar(&buildCmd, "build-command", "", "Build command for .taskrc (default: unset; not language-specific)")
	cmd.Flags().StringVar(&testCmd, "test-command", "", "Test command for .taskrc (default: unset; not language-specific)")

	return cmd
}

// newInitClaudeCmd creates the 'init claude' command
func newInitClaudeCmd() *cobra.Command {
	var managed bool

	cmd := &cobra.Command{
		Use:   "claude [path]",
		Short: "Initialize Claude-specific files",
		Long:  `Initialize Claude configuration and context files`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}

			// Create absolute path
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			fmt.Printf("Initializing Claude files at %s...\n", absPath)

			// Create .adb directory if it doesn't exist
			adbDir := filepath.Join(absPath, ".adb")
			if err := os.MkdirAll(adbDir, 0o755); err != nil {
				return fmt.Errorf("failed to create .adb directory: %w", err)
			}

			// Create CLAUDE.md
			claudePath := filepath.Join(absPath, "CLAUDE.md")
			if _, err := os.Stat(claudePath); os.IsNotExist(err) {
				claudeContent := "# Claude Context\n\n" +
					"This workspace uses AI Dev Brain for task management and AI-assisted development.\n\n" +
					"## Usage\n\n" +
					"- Use `adb task create` to create new tasks\n" +
					"- Each task gets isolated in a git worktree\n" +
					"- Task context is maintained in `tickets/TASK-XXXXX/context.md`\n\n" +
					"## Commands\n\n" +
					"- `adb task status` - View all tasks\n" +
					"- `adb sync context` - Regenerate this file\n" +
					"- `adb metrics` - View workspace metrics\n" +
					"- `adb dashboard` - Open TUI dashboard\n"

				if managed {
					claudeContent += "\n## Managed Mode\n\nThis workspace is in managed mode - context files are auto-regenerated.\n"
				}

				if err := os.WriteFile(claudePath, []byte(claudeContent), 0o644); err != nil {
					return fmt.Errorf("failed to create CLAUDE.md: %w", err)
				}
				fmt.Println("  ✓ Created CLAUDE.md")
			}

			// Create claude-user.md
			userContextPath := filepath.Join(adbDir, "claude-user.md")
			if _, err := os.Stat(userContextPath); os.IsNotExist(err) {
				userContent := "# Claude User Context\n\n" +
					"User-specific preferences and context for Claude AI integration.\n\n" +
					"## Preferences\n\n" +
					"- Code style: Follow project conventions\n" +
					"- Testing: Always include tests\n" +
					"- Documentation: Keep inline comments minimal\n"

				if err := os.WriteFile(userContextPath, []byte(userContent), 0o644); err != nil {
					return fmt.Errorf("failed to create claude-user.md: %w", err)
				}
				fmt.Println("  ✓ Created claude-user.md")
			}

			fmt.Println("\n✓ Claude files initialized")

			return nil
		},
	}

	cmd.Flags().BoolVar(&managed, "managed", false, "Enable managed mode (auto-regeneration)")

	return cmd
}

// newInitProjectCmd creates the 'init project' command using ProjectInitializer
func newInitProjectCmd() *cobra.Command {
	var (
		name     string
		ai       string
		prefix   string
		buildCmd string
		testCmd  string
		gitInit  bool
		withBMAD bool
	)

	cmd := &cobra.Command{
		Use:   "project [path]",
		Short: "Initialize a new project with full scaffolding",
		Long: `Initialize a new project with complete workspace scaffolding including:
- Directory structure (tickets, work, sessions, .adb, .claude)
- Configuration files (.taskrc, backlog.yaml)
- Git repository (optional)
- BMAD artifacts (PRD, tech-spec, architecture-doc, quality gates) (optional)
- Claude integration files`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}

			// Create absolute path
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			fmt.Printf("🚀 Initializing project at %s...\n", absPath)

			// Create ProjectInitializer
			initializer := core.NewFileProjectInitializer(claude.FS)

			// Configure options. Defaults (name/ai/prefix) are applied by the
			// initializer itself, so we just forward the flag values.
			options := core.InitOptions{
				Name:         name,
				AIProvider:   ai,
				TaskIDPrefix: prefix,
				BuildCommand: buildCmd,
				TestCommand:  testCmd,
				GitInit:      gitInit,
				WithBMAD:     withBMAD,
			}

			// Initialize project
			if err := initializer.InitializeProject(absPath, options); err != nil {
				return fmt.Errorf("failed to initialize project: %w", err)
			}

			fmt.Printf("\n✅ Project initialized successfully!\n\n")
			fmt.Println("📁 Created:")
			fmt.Println("   • Directory structure (tickets, work, sessions, .adb, .claude)")
			fmt.Println("   • Configuration files (.taskrc, backlog.yaml)")
			if gitInit {
				fmt.Println("   • Git repository (.git, .gitignore)")
			}
			if withBMAD {
				fmt.Println("   • BMAD artifacts (docs/bmad/*.md)")
			}
			fmt.Println("   • Claude integration files (.claude/)")

			fmt.Printf("\n💡 Next steps:\n")
			if absPath != "." {
				fmt.Printf("   cd %s\n", absPath)
			}
			fmt.Println("   adb task create <branch-name>  # Create your first task")
			fmt.Println("   adb agents                      # See available agents")
			fmt.Println("   adb team dev \"your task\"        # Launch multi-agent team")

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&ai, "ai", "claude", "AI provider (claude, gpt)")
	cmd.Flags().StringVar(&prefix, "prefix", "TASK", "Task ID prefix")
	cmd.Flags().StringVar(&buildCmd, "build-command", "", "Build command for .taskrc (default: unset; not language-specific)")
	cmd.Flags().StringVar(&testCmd, "test-command", "", "Test command for .taskrc (default: unset; not language-specific)")
	cmd.Flags().BoolVar(&gitInit, "git", false, "Initialize git repository")
	cmd.Flags().BoolVar(&withBMAD, "bmad", false, "Include BMAD artifacts (PRD, tech-spec, etc.)")

	return cmd
}
