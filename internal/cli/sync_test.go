package cli

import (
	"fmt"
	"strings"
	"testing"
)

// --- Registration Tests ---

func TestSyncCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "sync" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'sync' command to be registered on root")
	}
}

func TestSyncCmd_Subcommands(t *testing.T) {
	expected := []string{"context", "task-context", "repos", "claude-user", "all"}
	subs := make(map[string]bool)
	for _, cmd := range syncCmd.Commands() {
		subs[cmd.Name()] = true
	}
	for _, name := range expected {
		if !subs[name] {
			t.Errorf("expected subcommand %q on 'sync', but it was not registered", name)
		}
	}
}

// --- sync context Tests ---

func TestSyncContext_NilGenerator(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()
	AICtxGen = nil

	err := syncContextSubCmd.RunE(syncContextSubCmd, []string{})
	if err == nil {
		t.Fatal("expected error when AICtxGen is nil")
	}
	if !strings.Contains(err.Error(), "AI context generator not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncContext_Success(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()

	syncCalled := false
	AICtxGen = &mockAIContextGenerator{
		syncContextFn: func() error {
			syncCalled = true
			return nil
		},
	}

	err := syncContextSubCmd.RunE(syncContextSubCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !syncCalled {
		t.Error("SyncContext was not called")
	}
}

func TestSyncContext_Error(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()

	AICtxGen = &mockAIContextGenerator{
		syncContextFn: func() error {
			return fmt.Errorf("disk full")
		},
	}

	err := syncContextSubCmd.RunE(syncContextSubCmd, []string{})
	if err == nil {
		t.Fatal("expected error from SyncContext")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- sync repos Tests ---

func TestSyncRepos_NilRepoSyncMgr(t *testing.T) {
	origMgr := RepoSyncMgr
	defer func() { RepoSyncMgr = origMgr }()
	RepoSyncMgr = nil

	err := syncReposSubCmd.RunE(syncReposSubCmd, []string{})
	if err == nil {
		t.Fatal("expected error when RepoSyncMgr is nil")
	}
	if !strings.Contains(err.Error(), "repo sync manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- sync all Tests ---

func TestSyncAll_AllFail(t *testing.T) {
	origGen := AICtxGen
	origMgr := RepoSyncMgr
	defer func() {
		AICtxGen = origGen
		RepoSyncMgr = origMgr
	}()

	AICtxGen = nil
	RepoSyncMgr = nil

	err := syncAllCmd.RunE(syncAllCmd, []string{})
	if err == nil {
		t.Fatal("expected error when all syncs fail")
	}
	if !strings.Contains(err.Error(), "sync operation(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncAll_PartialSuccess(t *testing.T) {
	origGen := AICtxGen
	origMgr := RepoSyncMgr
	defer func() {
		AICtxGen = origGen
		RepoSyncMgr = origMgr
	}()

	// Context sync succeeds, repos and claude-user fail.
	AICtxGen = &mockAIContextGenerator{
		syncContextFn: func() error {
			return nil
		},
	}
	RepoSyncMgr = nil

	err := syncAllCmd.RunE(syncAllCmd, []string{})
	if err == nil {
		t.Fatal("expected error when some syncs fail")
	}
}

// --- Deprecation Tests ---

func TestDeprecated_OldSyncContextCommand(t *testing.T) {
	if syncContextCmd.Deprecated == "" {
		t.Error("expected 'sync-context' command to have Deprecated set")
	}
	if !strings.Contains(syncContextCmd.Deprecated, "adb sync context") {
		t.Errorf("sync-context Deprecated = %q, should mention 'adb sync context'", syncContextCmd.Deprecated)
	}
}

func TestDeprecated_OldSyncTaskContextCommand(t *testing.T) {
	if syncTaskContextCmd.Deprecated == "" {
		t.Error("expected 'sync-task-context' command to have Deprecated set")
	}
	if !strings.Contains(syncTaskContextCmd.Deprecated, "adb sync task-context") {
		t.Errorf("sync-task-context Deprecated = %q, should mention 'adb sync task-context'", syncTaskContextCmd.Deprecated)
	}
}

func TestDeprecated_OldSyncReposCommand(t *testing.T) {
	if syncReposCmd.Deprecated == "" {
		t.Error("expected 'sync-repos' command to have Deprecated set")
	}
	if !strings.Contains(syncReposCmd.Deprecated, "adb sync repos") {
		t.Errorf("sync-repos Deprecated = %q, should mention 'adb sync repos'", syncReposCmd.Deprecated)
	}
}

func TestDeprecated_OldSyncClaudeUserCommand(t *testing.T) {
	if syncClaudeUserCmd.Deprecated == "" {
		t.Error("expected 'sync-claude-user' command to have Deprecated set")
	}
	if !strings.Contains(syncClaudeUserCmd.Deprecated, "adb sync claude-user") {
		t.Errorf("sync-claude-user Deprecated = %q, should mention 'adb sync claude-user'", syncClaudeUserCmd.Deprecated)
	}
}

func TestDeprecated_OldInitClaudeCommand(t *testing.T) {
	if initClaudeCmd.Deprecated == "" {
		t.Error("expected 'init-claude' command to have Deprecated set")
	}
	if !strings.Contains(initClaudeCmd.Deprecated, "adb init claude") {
		t.Errorf("init-claude Deprecated = %q, should mention 'adb init claude'", initClaudeCmd.Deprecated)
	}
}

// --- Init Claude Sub-command Tests ---

func TestInitClaude_SubcommandRegistration(t *testing.T) {
	found := false
	for _, cmd := range initCmd.Commands() {
		if cmd.Name() == "claude" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'claude' subcommand on 'init'")
	}
}
