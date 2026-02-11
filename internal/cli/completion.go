package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var completionInstall bool

var completionCmd = &cobra.Command{
	Use:   "completion <shell>",
	Short: "Set up shell completions for adb",
	Long: `Set up shell tab-completions for adb commands, flags, and arguments.

Supported shells: bash, zsh, fish, powershell

Quick install (adds completions to your shell profile):

  adb completion bash --install
  adb completion zsh --install
  adb completion fish --install

Or print the completion script to stdout (for manual setup):

  adb completion bash
  adb completion zsh
  adb completion fish
  adb completion powershell`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MaximumNArgs(1),
	RunE:      runCompletion,
}

func init() {
	completionCmd.Flags().BoolVar(&completionInstall, "install", false,
		"Install completions into your shell profile")

	// Remove Cobra's default completion command and add ours.
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	shell := args[0]

	if completionInstall {
		return installCompletion(shell)
	}

	// Print script to stdout; usage hints go to stderr so they don't
	// interfere with piping (e.g., eval "$(adb completion bash)").
	switch shell {
	case "bash":
		printHints(cmd,
			"# To load completions in your current session:",
			`#   eval "$(adb completion bash)"`,
			"#",
			"# To install permanently:",
			"#   adb completion bash --install",
			"#",
		)
		return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
	case "zsh":
		printHints(cmd,
			"# To load completions in your current session:",
			`#   eval "$(adb completion zsh)"`,
			"#",
			"# To install permanently:",
			"#   adb completion zsh --install",
			"#",
		)
		return rootCmd.GenZshCompletion(cmd.OutOrStdout())
	case "fish":
		printHints(cmd,
			"# To load completions in your current session:",
			"#   adb completion fish | source",
			"#",
			"# To install permanently:",
			"#   adb completion fish --install",
			"#",
		)
		return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
	case "powershell":
		printHints(cmd,
			"# To load completions in your current session:",
			"#   adb completion powershell | Out-String | Invoke-Expression",
			"#",
			"# Permanent install is not supported with --install for PowerShell.",
			"# Add the above command to your PowerShell profile manually.",
			"#",
		)
		return rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish, powershell)", shell)
	}
}

// printHints writes usage hints to stderr so they don't interfere with
// piping the completion script from stdout.
func printHints(cmd *cobra.Command, lines ...string) {
	w := cmd.OutOrStderr()
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

func installCompletion(shell string) error {
	switch shell {
	case "bash":
		return installBashCompletion()
	case "zsh":
		return installZshCompletion()
	case "fish":
		return installFishCompletion()
	case "powershell":
		return fmt.Errorf("automatic install is not supported for PowerShell; run 'adb completion powershell' and add the output to your profile")
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
}

// writeCompletionFile creates a file at target, calls genFn to write the
// completion script into it, and returns the target path. It ensures the
// file is closed properly and propagates close errors.
func writeCompletionFile(target string, genFn func(*os.File) error) error {
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("creating completion file %s: %w", target, err)
	}

	writeErr := genFn(f)
	closeErr := f.Close()

	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("closing completion file %s: %w", target, closeErr)
	}
	return nil
}

func installBashCompletion() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("detecting home directory: %w", err)
	}

	target := bashCompletionTarget(home)

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("creating completion directory: %w", err)
	}

	if err := writeCompletionFile(target, func(f *os.File) error {
		return rootCmd.GenBashCompletionV2(f, true)
	}); err != nil {
		return err
	}

	fmt.Printf("Bash completions installed to %s\n", target)
	fmt.Printf("Restart your shell or run: source %s\n", target)
	return nil
}

func bashCompletionTarget(home string) string {
	// Use user-local path on all platforms. This avoids needing root/admin
	// permissions and works with modern bash-completion (>= 2.0).
	return filepath.Join(home, ".local", "share", "bash-completion", "completions", "adb")
}

func installZshCompletion() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("detecting home directory: %w", err)
	}

	dir := filepath.Join(home, ".local", "share", "zsh", "site-functions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating completion directory: %w", err)
	}
	target := filepath.Join(dir, "_adb")

	if err := writeCompletionFile(target, func(f *os.File) error {
		return rootCmd.GenZshCompletion(f)
	}); err != nil {
		return err
	}

	fmt.Printf("Zsh completions installed to %s\n", target)
	fmt.Println()
	fmt.Println("Ensure this directory is in your fpath. Add to ~/.zshrc if needed:")
	fmt.Printf("  fpath=(%s $fpath)\n", dir)
	fmt.Println("  autoload -Uz compinit && compinit")
	return nil
}

func installFishCompletion() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("detecting home directory: %w", err)
	}

	dir := filepath.Join(home, ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating completion directory: %w", err)
	}
	target := filepath.Join(dir, "adb.fish")

	if err := writeCompletionFile(target, func(f *os.File) error {
		return rootCmd.GenFishCompletion(f, true)
	}); err != nil {
		return err
	}

	fmt.Printf("Fish completions installed to %s\n", target)
	fmt.Println("Completions will be available in new fish sessions automatically.")
	return nil
}
