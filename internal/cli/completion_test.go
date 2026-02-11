package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionCommand_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "completion" {
			found = true
			break
		}
	}
	if !found {
		t.Error("completion command not registered on root")
	}
}

func TestCompletionCommand_DisablesDefault(t *testing.T) {
	if !rootCmd.CompletionOptions.DisableDefaultCmd {
		t.Error("expected Cobra default completion command to be disabled")
	}
}

func TestCompletionCommand_NoArgsShowsHelp(t *testing.T) {
	var stdout bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion with no args should show help, not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Quick install") {
		t.Error("no-args output should show help with install instructions")
	}
}

func TestCompletionCommand_BashOutput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "bash"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "__start_adb") {
		t.Error("bash completion output should contain __start_adb function")
	}
}

func TestCompletionCommand_ZshOutput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "zsh"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "compdef") {
		t.Error("zsh completion output should contain compdef")
	}
}

func TestCompletionCommand_FishOutput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "fish"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "complete") {
		t.Error("fish completion output should contain complete command")
	}
}

func TestCompletionCommand_PowershellOutput(t *testing.T) {
	var stdout bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "powershell"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion powershell failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "Register-ArgumentCompleter") {
		t.Error("powershell completion output should contain Register-ArgumentCompleter")
	}
}

func TestCompletionCommand_UnsupportedShell(t *testing.T) {
	cmd := rootCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "nushell"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unsupported shell")
	}
}

func TestCompletionCommand_InstallBash(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // Windows

	cmd := rootCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "bash", "--install"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion bash --install failed: %v", err)
	}

	// Verify file was created somewhere under tmpHome.
	target := filepath.Join(tmpHome, ".local", "share", "bash-completion", "completions", "adb")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected bash completion file at %s: %v", target, err)
	}
	if !strings.Contains(string(data), "__start_adb") {
		t.Error("bash completion file should contain __start_adb function")
	}
}

func TestCompletionCommand_InstallZsh(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // Windows

	cmd := rootCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "zsh", "--install"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion zsh --install failed: %v", err)
	}

	target := filepath.Join(tmpHome, ".local", "share", "zsh", "site-functions", "_adb")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected zsh completion file at %s: %v", target, err)
	}
	if !strings.Contains(string(data), "compdef") {
		t.Error("zsh completion file should contain compdef")
	}
}

func TestCompletionCommand_InstallFish(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // Windows

	cmd := rootCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "fish", "--install"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("completion fish --install failed: %v", err)
	}

	target := filepath.Join(tmpHome, ".config", "fish", "completions", "adb.fish")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected fish completion file at %s: %v", target, err)
	}
	if !strings.Contains(string(data), "complete") {
		t.Error("fish completion file should contain complete command")
	}
}

func TestCompletionCommand_InstallPowershellFails(t *testing.T) {
	cmd := rootCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"completion", "powershell", "--install"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for powershell --install")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error should mention 'not supported', got: %v", err)
	}
}
