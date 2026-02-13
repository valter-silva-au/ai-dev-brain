package cli

import (
	"bytes"
	"fmt"
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

func TestInstallCompletion_UnsupportedShell(t *testing.T) {
	err := installCompletion("nushell")
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteCompletionFile_CreateError(t *testing.T) {
	// Target a path that cannot be created (directory doesn't exist and name is invalid).
	target := filepath.Join(t.TempDir(), "nonexistent", "subdir", "file")
	err := writeCompletionFile(target, func(f *os.File) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error when creating file in non-existent directory")
	}
	if !strings.Contains(err.Error(), "creating completion file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteCompletionFile_GenFnError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "test-completion")
	err := writeCompletionFile(target, func(f *os.File) error {
		return fmt.Errorf("generation failed")
	})
	if err == nil {
		t.Fatal("expected error from genFn")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteCompletionFile_Success(t *testing.T) {
	target := filepath.Join(t.TempDir(), "test-completion")
	err := writeCompletionFile(target, func(f *os.File) error {
		_, err := f.WriteString("test content")
		return err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("file content = %q, want 'test content'", string(data))
	}
}

func TestBashCompletionTarget(t *testing.T) {
	got := bashCompletionTarget("/home/user")
	want := filepath.Join("/home/user", ".local", "share", "bash-completion", "completions", "adb")
	if got != want {
		t.Errorf("bashCompletionTarget = %q, want %q", got, want)
	}
}

func TestInstallBashCompletion_MkdirAllError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create a file where the directory needs to be, blocking MkdirAll.
	blockPath := filepath.Join(tmpHome, ".local")
	if err := os.WriteFile(blockPath, []byte("blocker"), 0o444); err != nil {
		t.Fatal(err)
	}

	err := installBashCompletion()
	if err == nil {
		t.Fatal("expected error when directory creation is blocked")
	}
	if !strings.Contains(err.Error(), "creating completion directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallZshCompletion_MkdirAllError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create a file where the directory needs to be, blocking MkdirAll.
	blockPath := filepath.Join(tmpHome, ".local")
	if err := os.WriteFile(blockPath, []byte("blocker"), 0o444); err != nil {
		t.Fatal(err)
	}

	err := installZshCompletion()
	if err == nil {
		t.Fatal("expected error when directory creation is blocked")
	}
	if !strings.Contains(err.Error(), "creating completion directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallFishCompletion_MkdirAllError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create a file where the directory needs to be, blocking MkdirAll.
	blockPath := filepath.Join(tmpHome, ".config")
	if err := os.WriteFile(blockPath, []byte("blocker"), 0o444); err != nil {
		t.Fatal(err)
	}

	err := installFishCompletion()
	if err == nil {
		t.Fatal("expected error when directory creation is blocked")
	}
	if !strings.Contains(err.Error(), "creating completion directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallBashCompletion_WriteError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create the target as a directory so that os.Create fails.
	target := bashCompletionTarget(tmpHome)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	err := installBashCompletion()
	if err == nil {
		t.Fatal("expected error when target is a directory")
	}
}

func TestInstallZshCompletion_WriteError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create the target as a directory so that os.Create fails.
	dir := filepath.Join(tmpHome, ".local", "share", "zsh", "site-functions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "_adb")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	err := installZshCompletion()
	if err == nil {
		t.Fatal("expected error when target is a directory")
	}
}

func TestInstallFishCompletion_WriteError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Create the target as a directory so that os.Create fails.
	dir := filepath.Join(tmpHome, ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "adb.fish")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	err := installFishCompletion()
	if err == nil {
		t.Fatal("expected error when target is a directory")
	}
}

func TestInstallBashCompletion_UserHomeDirError(t *testing.T) {
	// Unset HOME on Unix or USERPROFILE on Windows to trigger UserHomeDir error.
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
		if origUserProfile != "" {
			os.Setenv("USERPROFILE", origUserProfile)
		}
	}()

	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	err := installBashCompletion()
	if err == nil {
		t.Fatal("expected error when HOME is unset")
	}
	if !strings.Contains(err.Error(), "detecting home directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallZshCompletion_UserHomeDirError(t *testing.T) {
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
		if origUserProfile != "" {
			os.Setenv("USERPROFILE", origUserProfile)
		}
	}()

	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	err := installZshCompletion()
	if err == nil {
		t.Fatal("expected error when HOME is unset")
	}
	if !strings.Contains(err.Error(), "detecting home directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallFishCompletion_UserHomeDirError(t *testing.T) {
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
		if origUserProfile != "" {
			os.Setenv("USERPROFILE", origUserProfile)
		}
	}()

	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	err := installFishCompletion()
	if err == nil {
		t.Fatal("expected error when HOME is unset")
	}
	if !strings.Contains(err.Error(), "detecting home directory") {
		t.Errorf("unexpected error: %v", err)
	}
}
