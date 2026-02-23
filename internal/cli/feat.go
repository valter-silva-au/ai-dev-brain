package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TaskMgr is the TaskManager used by task lifecycle commands.
// Set during application wiring (Task #43).
var TaskMgr core.TaskManager

// BasePath is the resolved base path for the adb workspace.
// Set during application wiring.
var BasePath string

// taskCreateFlags holds the optional flags shared by feat/bug/spike/refactor commands.
type taskCreateFlags struct {
	repo     string
	priority string
	owner    string
	tags     []string
	prefix   string
}

// newTaskCommand creates a Cobra command for a given task type (feat, bug, spike, refactor).
// All four commands share the same logic, differing only in the TaskType passed to CreateTask.
func newTaskCommand(taskType models.TaskType) *cobra.Command {
	var flags taskCreateFlags

	cmd := &cobra.Command{
		Use:   string(taskType) + " <branch-name>",
		Short: fmt.Sprintf("Create a new %s task", taskType),
		Long: fmt.Sprintf(`Create a new %s task with the given branch name.

This bootstraps a ticket folder, creates a git worktree, initializes context
files, and registers the task in the backlog.`, taskType),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if TaskMgr == nil {
				return fmt.Errorf("task manager not initialized")
			}

			branchName := args[0]
			repoPath := flags.repo

			// Auto-detect git repository from cwd when --repo is not provided.
			if repoPath == "" {
				if detected := detectGitRoot(); detected != "" {
					repoPath = detected
				}
			}

			// Determine the task prefix: explicit --prefix flag takes priority,
			// otherwise auto-detect from cwd if under repos/.
			prefix := flags.prefix
			if prefix == "" && repoPath != "" {
				prefix = detectRepoPrefix(repoPath, BasePath)
			}

			task, err := TaskMgr.CreateTask(taskType, branchName, repoPath, core.CreateTaskOpts{
				Priority:      models.Priority(flags.priority),
				Owner:         flags.owner,
				Tags:          flags.tags,
				BranchPattern: BranchPattern,
				Prefix:        prefix,
			})
			if err != nil {
				return fmt.Errorf("creating %s task: %w", taskType, err)
			}

			fmt.Printf("Created task %s\n", task.ID)
			fmt.Printf("  Type:     %s\n", task.Type)
			fmt.Printf("  Branch:   %s\n", task.Branch)
			if task.Repo != "" {
				fmt.Printf("  Repo:     %s\n", task.Repo)
			}
			if task.WorktreePath != "" {
				fmt.Printf("  Worktree: %s\n", task.WorktreePath)
			}
			fmt.Printf("  Ticket:   %s\n", task.TicketPath)

			// Inject accumulated project knowledge into task context.
			if task.WorktreePath != "" {
				appendKnowledgeToTaskContext(task.WorktreePath)
			}

			// Post-create workflow: rename terminal tab and launch Claude Code.
			if task.WorktreePath != "" {
				// Set ADB_TASK_TYPE and ADB_REPO_SHORT so the status line script
				// can display them without re-parsing status.yaml.
				if task.Type != "" {
					_ = os.Setenv("ADB_TASK_TYPE", string(task.Type))
				}
				if task.Repo != "" {
					_ = os.Setenv("ADB_REPO_SHORT", repoShortName(task.Repo))
				}
				launchWorkflow(task.ID, task.Branch, task.WorktreePath, task.TicketPath, false)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&flags.repo, "repo", "", "Repository path (e.g. github.com/org/repo)")
	cmd.Flags().StringVar(&flags.priority, "priority", "", "Task priority (P0, P1, P2, P3)")
	cmd.Flags().StringVar(&flags.owner, "owner", "", "Task owner (e.g. @username)")
	cmd.Flags().StringSliceVar(&flags.tags, "tags", nil, "Comma-separated tags for the task")
	cmd.Flags().StringVar(&flags.prefix, "prefix", "", "Custom prefix for organizing task folders (e.g. 'finance')")

	return cmd
}

func init() {
	for _, tt := range []models.TaskType{models.TaskTypeFeat, models.TaskTypeBug, models.TaskTypeSpike, models.TaskTypeRefactor} {
		cmd := newTaskCommand(tt)
		registerTaskCommandCompletions(cmd)
		rootCmd.AddCommand(cmd)
	}
}

// detectGitRoot returns the git repository root of the current working
// directory, or an empty string if not inside a git repository.
func detectGitRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// setTerminalTitle sets the terminal tab/window title using the ANSI OSC 0
// escape sequence. Writes directly to /dev/tty to bypass any stdout buffering
// or redirection. Falls back to stderr if /dev/tty is unavailable.
func setTerminalTitle(title string) {
	seq := fmt.Sprintf("\033]0;%s\007", title)
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		_, _ = fmt.Fprint(tty, seq)
		_ = tty.Close()
	} else {
		_, _ = fmt.Fprint(os.Stderr, seq)
	}
}

// repoShortName extracts the final path segment from a repo identifier,
// stripping any .git suffix. Handles both URL and path-style repo strings.
func repoShortName(repo string) string {
	if repo == "" {
		return ""
	}
	// Strip .git suffix.
	r := strings.TrimSuffix(repo, ".git")
	// Take last segment after any / or \ or :
	for _, sep := range []string{"/", "\\", ":"} {
		if idx := strings.LastIndex(r, sep); idx >= 0 {
			r = r[idx+1:]
		}
	}
	return r
}

// detectRepoPrefix extracts platform/org/repo from a repo path relative to
// basePath, for use as a task ID prefix. If the repo path is not under
// basePath/repos/, returns empty string.
func detectRepoPrefix(repoPath, basePath string) string {
	if basePath == "" || repoPath == "" {
		return ""
	}
	return core.NormalizeRepoToPrefix(repoPath, basePath)
}

// resolveShell returns the path to an interactive shell appropriate for the
// current platform. On Windows it tries COMSPEC, then bash (for Git Bash/WSL),
// then powershell, then falls back to cmd.exe. On Unix it respects the SHELL
// env var, then tries exec.LookPath("bash"), then falls back to /bin/bash.
func resolveShell() string {
	if runtime.GOOS == "windows" {
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		if p, err := exec.LookPath("bash"); err == nil {
			return p
		}
		if p, err := exec.LookPath("powershell"); err == nil {
			return p
		}
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return "/bin/bash"
}

// launchWorkflow renames the terminal tab, launches Claude Code in the
// worktree directory, and then drops the user into a shell in the worktree
// so they remain in the work directory after Claude exits.
func launchWorkflow(taskID, branch, worktreePath, ticketPath string, resume bool) {
	// Check if worktree directory exists.
	if _, err := os.Stat(worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: worktree directory does not exist: %s\n", worktreePath)
		return
	}

	// Set terminal title with rich context.
	title := fmt.Sprintf("%s (%s)", taskID, branch)
	setTerminalTitle(title)

	// Look for claude binary.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		if resume {
			fmt.Printf("\nTo start working, run:\n  cd %s\n  claude --dangerously-skip-permissions --resume\n", worktreePath)
		} else {
			fmt.Printf("\nTo start working, run:\n  cd %s\n  claude --dangerously-skip-permissions\n", worktreePath)
		}
		return
	}

	fmt.Printf("\nLaunching Claude Code in %s...\n", worktreePath)
	claudeArgs := []string{"--dangerously-skip-permissions"}
	if resume {
		claudeArgs = append(claudeArgs, "--resume")
	}

	// Check Claude Code version for feature support.
	if VersionChecker != nil {
		ver, verErr := VersionChecker.DetectVersion()
		if verErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not detect Claude Code version: %v\n", verErr)
		} else {
			supported, featErr := VersionChecker.SupportsFeature("worktree_isolation")
			if featErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not check worktree feature support: %v\n", featErr)
			} else if supported {
				claudeArgs = append(claudeArgs, "--worktree")
			}
			// Log detected version.
			fmt.Printf("  Claude Code v%d.%d.%d detected\n", ver.Major, ver.Minor, ver.Patch)
		}
	}

	// Build ADB environment variables for both Claude and the shell.
	adbEnv := append(os.Environ(),
		"ADB_TASK_ID="+taskID,
		"ADB_BRANCH="+branch,
		"ADB_WORKTREE_PATH="+worktreePath,
		"ADB_TICKET_PATH="+ticketPath,
		"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1",
	)

	claudeCmd := exec.Command(claudePath, claudeArgs...)
	claudeCmd.Dir = worktreePath
	claudeCmd.Env = adbEnv
	claudeCmd.Stdin = os.Stdin
	claudeCmd.Stdout = os.Stdout
	claudeCmd.Stderr = os.Stderr

	if err := claudeCmd.Run(); err != nil {
		// Non-zero exit from Claude is not necessarily an error (user pressed Ctrl-C).
		fmt.Printf("Claude Code exited: %v\n", err)
	}

	// Restore terminal title after Claude exits.
	setTerminalTitle(title)

	// Drop the user into an interactive shell in the worktree directory so
	// they remain in the work directory after Claude exits.
	shell := resolveShell()

	fmt.Printf("\nDropping into shell at %s\n", worktreePath)
	fmt.Printf("Type 'exit' to return to your original directory.\n\n")

	// Build the printf command that sets the terminal title via ANSI OSC 0.
	titleSeq := fmt.Sprintf(`printf "\033]0;%s\007"`, title)

	shellEnv := adbEnv

	var shellCmd *exec.Cmd

	if runtime.GOOS == "windows" {
		// On Windows, skip ZDOTDIR and PROMPT_COMMAND logic -- just launch
		// the resolved shell directly with Dir set to the worktree.
		shellCmd = exec.Command(shell)
		shellCmd.Env = shellEnv
	} else if strings.HasSuffix(shell, "/zsh") {
		// For zsh, create a temporary ZDOTDIR with a .zshenv that sources
		// the user's real config and then installs a precmd hook to maintain
		// the terminal title on every prompt.
		tmpDir, mkErr := os.MkdirTemp("", "adb-zsh-*")
		if mkErr == nil {
			defer func() { _ = os.RemoveAll(tmpDir) }()

			realZDOTDIR := os.Getenv("ZDOTDIR")
			if realZDOTDIR == "" {
				realZDOTDIR = os.Getenv("HOME")
			}

			zshenvContent := fmt.Sprintf(
				"# adb: source user's .zshenv then install title hook\n"+
					"[[ -f %q/.zshenv ]] && source %q/.zshenv\n"+
					"function precmd { %s; }\n",
				realZDOTDIR, realZDOTDIR, titleSeq,
			)

			zshrcContent := fmt.Sprintf(
				"# adb: source user's .zshrc then re-install title hook\n"+
					"[[ -f %q/.zshrc ]] && source %q/.zshrc\n"+
					"function precmd { %s; }\n",
				realZDOTDIR, realZDOTDIR, titleSeq,
			)

			zshenvPath := filepath.Join(tmpDir, ".zshenv")
			zshrcPath := filepath.Join(tmpDir, ".zshrc")
			_ = os.WriteFile(zshenvPath, []byte(zshenvContent), 0o644)
			_ = os.WriteFile(zshrcPath, []byte(zshrcContent), 0o644)

			shellEnv = append(shellEnv, "ZDOTDIR="+tmpDir)
		}

		shellCmd = exec.Command(shell)
		shellCmd.Env = shellEnv
	} else {
		// For bash and other POSIX shells, PROMPT_COMMAND is executed
		// before each prompt, which will re-set the terminal title.
		shellEnv = append(shellEnv, "PROMPT_COMMAND="+titleSeq)
		shellCmd = exec.Command(shell)
		shellCmd.Env = shellEnv
	}

	shellCmd.Dir = worktreePath
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr

	if err := shellCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Shell exited with error: %v\n", err)
	}
}
