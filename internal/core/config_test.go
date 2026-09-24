package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestNewViperConfigManager(t *testing.T) {
	t.Run("with custom paths", func(t *testing.T) {
		globalPath := "/custom/global/path"
		repoPath := "/custom/repo/path"

		cm := NewViperConfigManager(globalPath, repoPath)

		if cm.globalConfigPath != globalPath {
			t.Errorf("expected global path %s, got %s", globalPath, cm.globalConfigPath)
		}

		if cm.repoConfigPath != repoPath {
			t.Errorf("expected repo path %s, got %s", repoPath, cm.repoConfigPath)
		}
	})

	t.Run("with default paths", func(t *testing.T) {
		cm := NewViperConfigManager("", "")

		homeDir, _ := os.UserHomeDir()
		expectedGlobal := filepath.Join(homeDir, ".taskconfig")

		if cm.globalConfigPath != expectedGlobal {
			t.Errorf("expected global path %s, got %s", expectedGlobal, cm.globalConfigPath)
		}

		if cm.repoConfigPath != ".taskrc" {
			t.Errorf("expected repo path .taskrc, got %s", cm.repoConfigPath)
		}
	})
}

func TestGetGlobalConfig_NoFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")

	cm := NewViperConfigManager(nonExistentPath, "")

	config, err := cm.GetGlobalConfig()
	if err != nil {
		t.Fatalf("expected no error when file doesn't exist, got: %v", err)
	}

	// Should return default config
	defaultConfig := models.DefaultGlobalConfig()

	if config.TaskIDPrefix != defaultConfig.TaskIDPrefix {
		t.Errorf("expected default prefix %s, got %s", defaultConfig.TaskIDPrefix, config.TaskIDPrefix)
	}

	if !config.Hooks.Enabled {
		t.Error("expected hooks to be enabled by default")
	}
}

func TestGetGlobalConfig_WithFile(t *testing.T) {
	// Create a temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".taskconfig")

	configContent := `task_id_prefix: "PROJ"
defaults:
  priority: "P1"
  type: "feature"
notifications:
  enabled: true
  channels:
    - slack
    - email
  on_events:
    - task_completed
hooks:
  enabled: true
  pre_tool_use: true
  post_tool_use: false
  stop: true
  task_completed: true
  session_end: false
  knowledge_extraction: true
  conflict_detection: true
  auto_format: false
  block_vendor_edits: true
aliases:
  aliases:
    t: "task"
    l: "list"
mcp_servers:
  server1: "http://localhost:8080"
feature_flags:
  new_feature: true
custom_settings:
  setting1: "value1"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cm := NewViperConfigManager(configPath, "")

	config, err := cm.GetGlobalConfig()
	if err != nil {
		t.Fatalf("failed to load global config: %v", err)
	}

	// Verify loaded values
	if config.TaskIDPrefix != "PROJ" {
		t.Errorf("expected prefix PROJ, got %s", config.TaskIDPrefix)
	}

	if config.Defaults["priority"] != "P1" {
		t.Errorf("expected priority P1, got %s", config.Defaults["priority"])
	}

	if config.Defaults["type"] != "feature" {
		t.Errorf("expected type feature, got %s", config.Defaults["type"])
	}

	// The fixture above still carries a `notifications:` block, deliberately.
	// `NotificationConfig` was removed once `adb alerts --notify` — its only
	// mention, which printed "✓ Notifications sent" over a TODO and sent
	// nothing — was deleted. Removing a mapstructure field is a persisted-schema
	// change, so the block stays here as a real user's stale `.taskconfig`: it
	// pins that an unknown key is TOLERATED rather than fatal. That holds
	// because the loader calls `viper.Unmarshal`, not `UnmarshalExact` (only the
	// latter sets `DecoderConfig.ErrorUnused`). Every assertion below reads a
	// key declared *after* the removed block, so a decode that had turned fatal
	// would fail this test rather than pass it quietly.
	if !config.Hooks.PreToolUse {
		t.Error("expected pre_tool_use to be true")
	}

	if config.Hooks.PostToolUse {
		t.Error("expected post_tool_use to be false")
	}

	if config.Hooks.SessionEnd {
		t.Error("expected session_end to be false")
	}

	if !config.Hooks.KnowledgeExtraction {
		t.Error("expected knowledge_extraction to be true")
	}

	if !config.Hooks.ConflictDetection {
		t.Error("expected conflict_detection to be true")
	}

	if config.Aliases.Aliases["t"] != "task" {
		t.Errorf("expected alias 't' to map to 'task', got %s", config.Aliases.Aliases["t"])
	}

	if config.MCPServers["server1"] != "http://localhost:8080" {
		t.Errorf("expected server1 URL, got %s", config.MCPServers["server1"])
	}

	if !config.FeatureFlags["new_feature"] {
		t.Error("expected new_feature flag to be true")
	}

	if config.CustomSettings["setting1"] != "value1" {
		t.Errorf("expected setting1 to be value1, got %s", config.CustomSettings["setting1"])
	}
}

// TestGetGlobalConfig_PartialFileKeepsHookDefaults guards #177: a .taskconfig
// that omits the `hooks:` block must still return the hook DEFAULTS (enabled),
// matching the no-file path — not the all-false zero value. Before the fix a
// partial file silently disabled the Global tier's hooks in `adb config show`.
func TestGetGlobalConfig_PartialFileKeepsHookDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".taskconfig")
	// A realistic partial config: only a prefix, no hooks: block.
	if err := os.WriteFile(configPath, []byte("task_id_prefix: \"PROJ\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cm := NewViperConfigManager(configPath, "")
	config, err := cm.GetGlobalConfig()
	if err != nil {
		t.Fatalf("GetGlobalConfig: %v", err)
	}

	if config.TaskIDPrefix != "PROJ" {
		t.Errorf("prefix = %q, want PROJ", config.TaskIDPrefix)
	}
	// The omitted hooks: block must fall back to DefaultHookConfig() — same as
	// the no-file path — not the zero value. Compare the flags DefaultHookConfig
	// sets true (HookConfig has a slice field, so it isn't == comparable).
	def := models.DefaultHookConfig()
	got := config.Hooks
	if got.Enabled != def.Enabled || got.PreToolUse != def.PreToolUse ||
		got.PostToolUse != def.PostToolUse || got.Stop != def.Stop ||
		got.TaskCompleted != def.TaskCompleted || got.SessionEnd != def.SessionEnd ||
		got.AutoFormat != def.AutoFormat || got.BlockVendorEdits != def.BlockVendorEdits {
		t.Errorf("omitted hooks block should default to %+v, got %+v", def, got)
	}
}

// TestGetGlobalConfig_ExplicitHooksRespected ensures the #177 fix does not clobber
// a user who DELIBERATELY disables hooks: an explicit `hooks: {enabled: false}`
// must be honoured, not overwritten by the defaults.
func TestGetGlobalConfig_ExplicitHooksRespected(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".taskconfig")
	if err := os.WriteFile(configPath, []byte("hooks:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cm := NewViperConfigManager(configPath, "")
	config, err := cm.GetGlobalConfig()
	if err != nil {
		t.Fatalf("GetGlobalConfig: %v", err)
	}
	if config.Hooks.Enabled {
		t.Error("an explicit `hooks: {enabled: false}` must be respected, not defaulted back to true")
	}
}

func TestGetRepoConfig_NoFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")

	cm := NewViperConfigManager("", nonExistentPath)

	config, err := cm.GetRepoConfig()
	if err != nil {
		t.Fatalf("expected no error when file doesn't exist, got: %v", err)
	}

	// Should return default config
	defaultConfig := models.DefaultRepoConfig()

	if config.BaseBranch != defaultConfig.BaseBranch {
		t.Errorf("expected default base branch %s, got %s", defaultConfig.BaseBranch, config.BaseBranch)
	}

	if config.AutoSync != defaultConfig.AutoSync {
		t.Errorf("expected default auto_sync %v, got %v", defaultConfig.AutoSync, config.AutoSync)
	}
}

func TestGetRepoConfig_WithFile(t *testing.T) {
	// Create a temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".taskrc")

	configContent := `repo_name: "my-project"
build_command: "go build ./..."
test_command: "go test ./... -v"
lint_command: "golangci-lint run"
reviewers:
  - alice
  - bob
required_checks:
  - lint
  - test
conventions:
  - "Use snake_case for variables"
  - "Add tests for all functions"
base_branch: "develop"
worktree_base_path: "/tmp/worktrees"
auto_sync: true
custom_settings:
  repo_setting: "repo_value"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cm := NewViperConfigManager("", configPath)

	config, err := cm.GetRepoConfig()
	if err != nil {
		t.Fatalf("failed to load repo config: %v", err)
	}

	// Verify loaded values
	if config.RepoName != "my-project" {
		t.Errorf("expected repo_name my-project, got %s", config.RepoName)
	}

	if config.BuildCommand != "go build ./..." {
		t.Errorf("expected build command 'go build ./...', got %s", config.BuildCommand)
	}

	if config.TestCommand != "go test ./... -v" {
		t.Errorf("expected test command 'go test ./... -v', got %s", config.TestCommand)
	}

	if config.LintCommand != "golangci-lint run" {
		t.Errorf("expected lint command 'golangci-lint run', got %s", config.LintCommand)
	}

	if len(config.Reviewers) != 2 {
		t.Errorf("expected 2 reviewers, got %d", len(config.Reviewers))
	}

	if config.Reviewers[0] != "alice" || config.Reviewers[1] != "bob" {
		t.Errorf("unexpected reviewers: %v", config.Reviewers)
	}

	if len(config.RequiredChecks) != 2 {
		t.Errorf("expected 2 required checks, got %d", len(config.RequiredChecks))
	}

	if len(config.Conventions) != 2 {
		t.Errorf("expected 2 conventions, got %d", len(config.Conventions))
	}

	if config.BaseBranch != "develop" {
		t.Errorf("expected base branch develop, got %s", config.BaseBranch)
	}

	if config.WorktreeBasePath != "/tmp/worktrees" {
		t.Errorf("expected worktree path /tmp/worktrees, got %s", config.WorktreeBasePath)
	}

	if !config.AutoSync {
		t.Error("expected auto_sync to be true")
	}

	if config.CustomSettings["repo_setting"] != "repo_value" {
		t.Errorf("expected repo_setting to be repo_value, got %s", config.CustomSettings["repo_setting"])
	}
}

func TestLoadConfig_BothFiles(t *testing.T) {
	// Create a temp directory with both config files
	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, ".taskconfig")
	repoPath := filepath.Join(tmpDir, ".taskrc")

	globalContent := `task_id_prefix: "GLOBAL"
defaults:
  priority: "P0"
hooks:
  enabled: true
  knowledge_extraction: true
`

	repoContent := `repo_name: "test-repo"
build_command: "make build"
base_branch: "main"
`

	if err := os.WriteFile(globalPath, []byte(globalContent), 0o644); err != nil {
		t.Fatalf("failed to create global config: %v", err)
	}

	if err := os.WriteFile(repoPath, []byte(repoContent), 0o644); err != nil {
		t.Fatalf("failed to create repo config: %v", err)
	}

	cm := NewViperConfigManager(globalPath, repoPath)

	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load merged config: %v", err)
	}

	// Verify global config is present
	if merged.Global.TaskIDPrefix != "GLOBAL" {
		t.Errorf("expected global prefix GLOBAL, got %s", merged.Global.TaskIDPrefix)
	}

	if merged.Global.Defaults["priority"] != "P0" {
		t.Errorf("expected priority P0, got %s", merged.Global.Defaults["priority"])
	}

	if !merged.Global.Hooks.KnowledgeExtraction {
		t.Error("expected knowledge_extraction to be true")
	}

	// Verify repo config is present
	if merged.Repo.RepoName != "test-repo" {
		t.Errorf("expected repo_name test-repo, got %s", merged.Repo.RepoName)
	}

	if merged.Repo.BuildCommand != "make build" {
		t.Errorf("expected build command 'make build', got %s", merged.Repo.BuildCommand)
	}

	if merged.Repo.BaseBranch != "main" {
		t.Errorf("expected base branch main, got %s", merged.Repo.BaseBranch)
	}
}

func TestLoadConfig_NoFiles(t *testing.T) {
	// Create a temp directory with no config files
	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, ".taskconfig")
	repoPath := filepath.Join(tmpDir, ".taskrc")

	cm := NewViperConfigManager(globalPath, repoPath)

	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("expected no error with missing files, got: %v", err)
	}

	// Should return default configs
	if merged.Global.TaskIDPrefix != "TASK" {
		t.Errorf("expected default prefix TASK, got %s", merged.Global.TaskIDPrefix)
	}

	if merged.Repo.BaseBranch != "main" {
		t.Errorf("expected default base branch main, got %s", merged.Repo.BaseBranch)
	}
}

func TestGetGlobalConfig_InvalidYAML(t *testing.T) {
	// Create a temp directory with invalid YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".taskconfig")

	invalidContent := `invalid: yaml: content:
  - this is
  bad yaml
    nested incorrectly
`

	if err := os.WriteFile(configPath, []byte(invalidContent), 0o644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cm := NewViperConfigManager(configPath, "")

	_, err := cm.GetGlobalConfig()
	if err == nil {
		t.Error("expected error when loading invalid YAML")
	}
}

func TestDefaultHookConfig(t *testing.T) {
	config := DefaultHookConfig()

	// Verify Phase 1 features are enabled
	if !config.Enabled {
		t.Error("expected hooks to be enabled")
	}

	if !config.PreToolUse {
		t.Error("expected pre_tool_use to be enabled")
	}

	if !config.PostToolUse {
		t.Error("expected post_tool_use to be enabled")
	}

	if !config.Stop {
		t.Error("expected stop to be enabled")
	}

	if !config.TaskCompleted {
		t.Error("expected task_completed to be enabled")
	}

	if !config.SessionEnd {
		t.Error("expected session_end to be enabled")
	}

	if !config.AutoFormat {
		t.Error("expected auto_format to be enabled")
	}

	if !config.BlockVendorEdits {
		t.Error("expected block_vendor_edits to be enabled")
	}

	// Verify Phase 2/3 features are disabled (opt-in)
	if config.KnowledgeExtraction {
		t.Error("expected knowledge_extraction to be disabled by default (opt-in)")
	}

	if config.ConflictDetection {
		t.Error("expected conflict_detection to be disabled by default (opt-in)")
	}
}

func TestConfigPrecedence(t *testing.T) {
	// Test that repo config takes precedence over global config
	// This test verifies the conceptual precedence by loading both configs
	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, ".taskconfig")
	repoPath := filepath.Join(tmpDir, ".taskrc")

	globalContent := `task_id_prefix: "GLOBAL"
defaults:
  priority: "P2"
  type: "feat"
hooks:
  enabled: true
  auto_format: true
`

	repoContent := `repo_name: "precedence-test"
base_branch: "main"
`

	if err := os.WriteFile(globalPath, []byte(globalContent), 0o644); err != nil {
		t.Fatalf("failed to create global config: %v", err)
	}

	if err := os.WriteFile(repoPath, []byte(repoContent), 0o644); err != nil {
		t.Fatalf("failed to create repo config: %v", err)
	}

	cm := NewViperConfigManager(globalPath, repoPath)

	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load merged config: %v", err)
	}

	// Both should be present and independently loaded
	// Application logic should apply precedence when using values
	if merged.Global == nil {
		t.Error("expected global config to be present")
	}

	if merged.Repo == nil {
		t.Error("expected repo config to be present")
	}

	// Verify both configs have their respective values
	if merged.Global.TaskIDPrefix != "GLOBAL" {
		t.Errorf("expected global prefix, got %s", merged.Global.TaskIDPrefix)
	}

	if merged.Repo.RepoName != "precedence-test" {
		t.Errorf("expected repo name, got %s", merged.Repo.RepoName)
	}
}

// writeOrgConfig writes orgs/<id>/config.yaml under dir (the workspace root).
func writeOrgConfig(t *testing.T, dir, orgID, content string) {
	t.Helper()
	orgDir := filepath.Join(dir, "orgs", orgID)
	if err := os.MkdirAll(orgDir, 0o755); err != nil {
		t.Fatalf("mkdir org dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orgDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write org config: %v", err)
	}
}

func TestLoadConfig_OrgTier_FromTaskrc(t *testing.T) {
	// This case is specifically about the `org:` FIELD selecting the tier, and
	// LoadConfig gives ADB_ORG precedence over that field — so an exported ADB_ORG
	// (common in this workspace) silently tests the wrong path. Pin it empty; the env
	// path has its own test in TestLoadConfig_OrgTier_FromEnvOverride below.
	t.Setenv("ADB_ORG", "")

	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, ".taskconfig")
	repoPath := filepath.Join(tmpDir, ".taskrc")

	// Repo names the active org and overrides one custom setting; the org tier
	// provides a value the repo does not; the global tier is the fallback.
	if err := os.WriteFile(globalPath, []byte("task_id_prefix: \"G\"\ncustom_settings:\n  k: \"global\"\n  only_global: \"g\"\n"), 0o644); err != nil {
		t.Fatalf("write global: %v", err)
	}
	if err := os.WriteFile(repoPath, []byte("org: \"acme\"\ncustom_settings:\n  k: \"repo\"\n"), 0o644); err != nil {
		t.Fatalf("write repo: %v", err)
	}
	writeOrgConfig(t, tmpDir, "acme", "custom_settings:\n  k: \"org\"\n  only_org: \"o\"\n")

	cm := NewViperConfigManager(globalPath, repoPath)
	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if merged.Org == nil {
		t.Fatal("expected org tier to be loaded from .taskrc org field")
	}
	if merged.Org.OrgID != "acme" {
		t.Errorf("Org.OrgID = %q, want acme", merged.Org.OrgID)
	}
	if v, ok := merged.Setting("k"); !ok || v != "repo" {
		t.Errorf("Setting(k) = (%q,%v), want (repo,true)", v, ok)
	}
	if v, ok := merged.Setting("only_org"); !ok || v != "o" {
		t.Errorf("Setting(only_org) = (%q,%v), want (o,true)", v, ok)
	}
	if v, ok := merged.Setting("only_global"); !ok || v != "g" {
		t.Errorf("Setting(only_global) = (%q,%v), want (g,true)", v, ok)
	}
}

func TestLoadConfig_OrgTier_FromEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	globalPath := filepath.Join(tmpDir, ".taskconfig")
	repoPath := filepath.Join(tmpDir, ".taskrc")

	// .taskrc names org "acme"; ADB_ORG overrides it to "beta".
	if err := os.WriteFile(repoPath, []byte("org: \"acme\"\n"), 0o644); err != nil {
		t.Fatalf("write repo: %v", err)
	}
	writeOrgConfig(t, tmpDir, "acme", "custom_settings:\n  who: \"acme\"\n")
	writeOrgConfig(t, tmpDir, "beta", "custom_settings:\n  who: \"beta\"\n")
	_ = globalPath

	t.Setenv("ADB_ORG", "beta")
	cm := NewViperConfigManager(globalPath, repoPath)
	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if merged.Org == nil || merged.Org.OrgID != "beta" {
		t.Fatalf("expected ADB_ORG to select beta, got %+v", merged.Org)
	}
	if v, _ := merged.Setting("who"); v != "beta" {
		t.Errorf("Setting(who) = %q, want beta", v)
	}
}

func TestLoadConfig_NoOrg_BackwardCompatible(t *testing.T) {
	// "No org configured" has to mean no env var either — ADB_ORG is one of the two
	// ways to configure the tier, so leaving the ambient value in place would make
	// this assert something other than what its name says.
	t.Setenv("ADB_ORG", "")

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, ".taskrc")
	// No org field, no orgs/ dir → org tier is nil, behaviour unchanged.
	if err := os.WriteFile(repoPath, []byte("repo_name: \"solo\"\n"), 0o644); err != nil {
		t.Fatalf("write repo: %v", err)
	}
	cm := NewViperConfigManager(filepath.Join(tmpDir, ".taskconfig"), repoPath)
	merged, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if merged.Org != nil {
		t.Errorf("expected nil org tier when none configured, got %+v", merged.Org)
	}
	if merged.Repo.RepoName != "solo" {
		t.Errorf("Repo.RepoName = %q, want solo", merged.Repo.RepoName)
	}
}

func TestGetOrgConfig_MissingIsNotAnError(t *testing.T) {
	tmpDir := t.TempDir()
	cm := NewViperConfigManager(filepath.Join(tmpDir, ".taskconfig"), filepath.Join(tmpDir, ".taskrc"))

	// Empty org id → nil, nil.
	if cfg, err := cm.GetOrgConfig(""); err != nil || cfg != nil {
		t.Errorf("GetOrgConfig(\"\") = (%v, %v), want (nil, nil)", cfg, err)
	}
	// Unknown org id (no file) → nil, nil.
	if cfg, err := cm.GetOrgConfig("ghost"); err != nil || cfg != nil {
		t.Errorf("GetOrgConfig(ghost) = (%v, %v), want (nil, nil)", cfg, err)
	}
}

// captureConfigStderr swaps os.Stderr for a pipe while fn runs and returns whatever
// was written to it. Custom-setting problems are deliberately non-fatal and reported
// on stderr (the same channel the hook engine and task manager warn on), so this is
// how the tests below prove a skipped setting left a trace instead of vanishing.
func captureConfigStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	// Drain CONCURRENTLY. A pipe buffers ~64 KiB, so reading only after fn() returns
	// turns "this tier emitted more warnings than expected" into a DEADLOCK (fn blocks
	// in Write, the test blocks waiting for fn) instead of a failure — and a hung test
	// is far harder to diagnose than a wrong string. internal/core/hookengine_test.go
	// takes the simpler route of one fixed-size Read after the fact; that caps what it
	// can observe, which is fine there and not here.
	drained := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r) // ends when the write end is closed below
		drained <- buf.String()
	}()

	fn()

	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out := <-drained
	// Close the READ end too — this helper runs once per subtest, so leaking it leaks
	// one fd per case and eventually takes the whole package out on EMFILE.
	if err := r.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return out
}

// TestGetRepoConfig_CustomSettingsAreTolerant pins the fix for the footgun where a
// DOTTED key under custom_settings (`programs.search_paths:` instead of the canonical
// `programs_search_paths:`) made Viper hand mapstructure a nested map where a string
// was wanted, failing config load — which, because config load happens at app init,
// killed every adb command in the workspace. A nested block must now flatten back to
// its dotted form, and anything genuinely un-representable must be skipped with a
// warning rather than being fatal.
func TestGetRepoConfig_CustomSettingsAreTolerant(t *testing.T) {
	tests := []struct {
		name     string
		taskrc   string
		want     map[string]string
		wantWarn []string // substrings that must appear on stderr
		noWarn   bool     // stderr must stay empty
	}{
		{
			name:   "flat string setting is unchanged",
			taskrc: "custom_settings:\n  programs_search_paths: \"/tmp/packs\"\n",
			want:   map[string]string{"programs_search_paths": "/tmp/packs"},
			noWarn: true,
		},
		{
			name:   "dotted key viper nests is reachable under its dotted name",
			taskrc: "custom_settings:\n  programs.search_paths: \"/tmp/whatever\"\n",
			want:   map[string]string{"programs.search_paths": "/tmp/whatever"},
			noWarn: true,
		},
		{
			name:   "genuinely nested block flattens to the same dotted key",
			taskrc: "custom_settings:\n  programs:\n    search_paths: \"/tmp/nested\"\n",
			want:   map[string]string{"programs.search_paths": "/tmp/nested"},
			noWarn: true,
		},
		{
			name:   "deep nesting flattens at arbitrary depth",
			taskrc: "custom_settings:\n  a:\n    b:\n      c:\n        d: \"deep\"\n",
			want:   map[string]string{"a.b.c.d": "deep"},
			noWarn: true,
		},
		{
			name: "sibling leaves under one nested parent each get their own key",
			taskrc: "custom_settings:\n  programs:\n    search_paths: \"/tmp/one\"\n" +
				"    default: \"technical-design\"\n",
			want: map[string]string{
				"programs.search_paths": "/tmp/one",
				"programs.default":      "technical-design",
			},
			noWarn: true,
		},
		{
			name: "non-string scalar leaves are stringified",
			taskrc: "custom_settings:\n  nested:\n    count: 5\n    ratio: 1.5\n" +
				"    on: true\n    off: false\n  flat_count: 7\n",
			want: map[string]string{
				"nested.count": "5",
				"nested.ratio": "1.5",
				// Matches mapstructure's weakly-typed decode of a FLAT bool, so
				// flattening never changes a value's spelling.
				"nested.on":  "1",
				"nested.off": "0",
				"flat_count": "7",
			},
			noWarn: true,
		},
		{
			name: "un-representable list leaf is skipped, warned, and not fatal",
			taskrc: "custom_settings:\n  programs:\n    search_paths:\n      - \"/a\"\n      - \"/b\"\n" +
				"    default: \"technical-design\"\n",
			want:     map[string]string{"programs.default": "technical-design"},
			wantWarn: []string{".taskrc", "programs.search_paths", "a list"},
		},
		{
			name:     "empty leaf is skipped with a warning",
			taskrc:   "custom_settings:\n  programs:\n    search_paths:\n",
			want:     map[string]string{},
			wantWarn: []string{"programs.search_paths", "an empty value"},
		},
		{
			// The consistency half of the pair above: `a: {}` and `a:` both bottom
			// out at nothing, so both must leave a trace. `a: {}` used to be dropped
			// in total silence while `a:` warned.
			name:     "empty nested block is skipped with a warning, like a nil leaf",
			taskrc:   "custom_settings:\n  programs: {}\n  keep: \"me\"\n",
			want:     map[string]string{"keep": "me"},
			wantWarn: []string{"programs", "an empty block"},
		},
		{
			// An unquoted date is a YAML *timestamp*, so it arrives as time.Time. The
			// warning must say "a date" and tell the reader to quote it, not leak the
			// Go type name `time.Time` into a message about their config file.
			name:     "an unquoted date is skipped, and the advice is to quote it",
			taskrc:   "custom_settings:\n  cutoff: 2026-01-01\n  keep: \"me\"\n",
			want:     map[string]string{"keep": "me"},
			wantWarn: []string{"cutoff", "a date", "wrap it in quotes"},
		},
		{
			name:   "both spellings survive side by side",
			taskrc: "custom_settings:\n  programs_search_paths: \"/canonical\"\n  programs.search_paths: \"/dotted\"\n",
			want: map[string]string{
				"programs_search_paths": "/canonical",
				"programs.search_paths": "/dotted",
			},
			noWarn: true,
		},
		{
			name: "a literal dotted key beats the same key spelled by nesting",
			taskrc: "custom_settings:\n  programs.search_paths: \"/literal\"\n" +
				"  programs:\n    search_paths: \"/nested\"\n",
			want:     map[string]string{"programs.search_paths": "/literal"},
			wantWarn: []string{"programs.search_paths", "literal value wins"},
		},
		{
			name:     "a whole custom_settings block that is not a mapping is ignored, not fatal",
			taskrc:   "custom_settings: \"oops\"\n",
			want:     map[string]string{},
			wantWarn: []string{"ignoring custom_settings", "a single text value"},
		},
		{
			name:   "other fields still decode alongside a nested custom setting",
			taskrc: "base_branch: \"trunk\"\ncustom_settings:\n  programs.search_paths: \"/tmp/whatever\"\n",
			want:   map[string]string{"programs.search_paths": "/tmp/whatever"},
			noWarn: true,
		},

		// ---- TOP-LEVEL `custom_settings.<key>:` entries -------------------------
		//
		// This spelling worked before the flattening fix (Unmarshal decoded
		// AllSettings(), which merges every AllKeys() leaf) and is exactly where a
		// user lands after hitting the original nested-key error and hoisting the
		// key out of the block. Flattening only the block dropped it — silently.
		{
			name: "a top-level dotted entry merges with the block, keeping both",
			taskrc: "custom_settings.programs_search_paths: \"/tmp/packs\"\n" +
				"custom_settings:\n  other: keep-me\n",
			want: map[string]string{
				"programs_search_paths": "/tmp/packs",
				"other":                 "keep-me",
			},
			noWarn: true,
		},
		{
			name:   "a top-level dotted entry with no block at all still resolves",
			taskrc: "custom_settings.programs_search_paths: \"/tmp/packs\"\n",
			want:   map[string]string{"programs_search_paths": "/tmp/packs"},
			noWarn: true,
		},
		{
			name: "a top-level dotted entry with dots of its own keeps its dotted name",
			taskrc: "custom_settings.programs.search_paths: \"/tmp/dotty\"\n" +
				"custom_settings:\n  other: keep-me\n",
			want: map[string]string{
				"programs.search_paths": "/tmp/dotty",
				"other":                 "keep-me",
			},
			noWarn: true,
		},
		{
			// The pre-fix winner: Viper resolves a doubly-spelled key with a
			// longest-prefix-first lookup, so the top-level literal wins — and
			// AllSettings() used the same lookup. Preserved deliberately, and warned
			// about because two spellings disagreeing is a mistake, not an intent.
			name: "when both spellings set the same key the top-level one wins, with a warning",
			taskrc: "custom_settings.a: rootval\n" +
				"custom_settings:\n  a: blockval\n",
			want:     map[string]string{"a": "rootval"},
			wantWarn: []string{".taskrc", `"a"`, "top of the file", "rootval"},
		},
		{
			name: "an unusable top-level dotted entry is skipped with a warning, not fatal",
			taskrc: "custom_settings.programs_search_paths:\n  - \"/a\"\n  - \"/b\"\n" +
				"custom_settings:\n  other: keep-me\n",
			want:     map[string]string{"other": "keep-me"},
			wantWarn: []string{"programs_search_paths", "a list"},
		},
		{
			// Pre-fix this shape killed the tier outright: the top-level literal wins
			// the lookup and cannot decode into a string. Now the block's usable value
			// survives and the ignored entry is named.
			name: "an unusable top-level entry falls back to the block value",
			taskrc: "custom_settings.a:\n  - \"/list\"\n" +
				"custom_settings:\n  a: blockval\n",
			want:     map[string]string{"a": "blockval"},
			wantWarn: []string{"ignoring the top-level", "custom_settings.a", "blockval"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			repoPath := filepath.Join(tmpDir, ".taskrc")
			if err := os.WriteFile(repoPath, []byte(tt.taskrc), 0o644); err != nil {
				t.Fatalf("write .taskrc: %v", err)
			}
			cm := NewViperConfigManager(filepath.Join(tmpDir, ".taskconfig"), repoPath)

			var (
				cfg *models.RepoConfig
				err error
			)
			stderr := captureConfigStderr(t, func() {
				cfg, err = cm.GetRepoConfig()
			})
			if err != nil {
				t.Fatalf("GetRepoConfig: unexpected error (config load must never be fatal here): %v", err)
			}

			if len(cfg.CustomSettings) != len(tt.want) {
				t.Errorf("CustomSettings = %#v, want %#v", cfg.CustomSettings, tt.want)
			}
			for k, want := range tt.want {
				got, ok := cfg.CustomSettings[k]
				if !ok {
					t.Errorf("CustomSettings[%q] missing; got %#v", k, cfg.CustomSettings)
					continue
				}
				if got != want {
					t.Errorf("CustomSettings[%q] = %q, want %q", k, got, want)
				}
			}

			if tt.noWarn && stderr != "" {
				t.Errorf("expected no warnings, got stderr: %s", stderr)
			}
			for _, want := range tt.wantWarn {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not contain %q", stderr, want)
				}
			}
		})
	}
}

// TestGetRepoConfig_DottedCustomSettingRoundTripsToProgramLookup asserts the property
// internal/cli/program.go's programSearchPathsKeys relies on: the canonical underscore
// spelling is consulted first and wins, with the flattened dotted spelling as the
// live fallback (it used to be dead code, since no tier ever reached the map intact).
func TestGetRepoConfig_DottedCustomSettingRoundTripsToProgramLookup(t *testing.T) {
	// Mirrors internal/cli/program.go's programSearchPathsKeys order.
	lookupKeys := []string{"programs_search_paths", "programs.search_paths"}
	resolve := func(mc *models.MergedConfig) (string, string) {
		for _, k := range lookupKeys {
			if v, ok := mc.Setting(k); ok {
				return k, v
			}
		}
		return "", ""
	}

	tests := []struct {
		name     string
		taskrc   string
		wantKey  string
		wantPath string
	}{
		{
			name:     "canonical only",
			taskrc:   "custom_settings:\n  programs_search_paths: \"/canonical\"\n",
			wantKey:  "programs_search_paths",
			wantPath: "/canonical",
		},
		{
			name:     "dotted only falls through to the second key",
			taskrc:   "custom_settings:\n  programs.search_paths: \"/dotted\"\n",
			wantKey:  "programs.search_paths",
			wantPath: "/dotted",
		},
		{
			name:     "nested spelling resolves like the dotted one",
			taskrc:   "custom_settings:\n  programs:\n    search_paths: \"/nested\"\n",
			wantKey:  "programs.search_paths",
			wantPath: "/nested",
		},
		{
			name: "canonical still wins when both are present",
			taskrc: "custom_settings:\n  programs.search_paths: \"/dotted\"\n" +
				"  programs_search_paths: \"/canonical\"\n",
			wantKey:  "programs_search_paths",
			wantPath: "/canonical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These cases declare no org tier, and ADB_ORG would introduce one.
			t.Setenv("ADB_ORG", "")

			tmpDir := t.TempDir()
			repoPath := filepath.Join(tmpDir, ".taskrc")
			if err := os.WriteFile(repoPath, []byte(tt.taskrc), 0o644); err != nil {
				t.Fatalf("write .taskrc: %v", err)
			}
			cm := NewViperConfigManager(filepath.Join(tmpDir, ".taskconfig"), repoPath)

			merged, err := cm.LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			gotKey, gotPath := resolve(merged)
			if gotKey != tt.wantKey || gotPath != tt.wantPath {
				t.Errorf("resolved (%q, %q), want (%q, %q)", gotKey, gotPath, tt.wantKey, tt.wantPath)
			}
		})
	}
}

// TestCustomSettings_FlattenedKeyTierPrecedence checks that flattening does not disturb
// the documented tier precedence: .taskrc (repo) > orgs/<id>/config.yaml (org) >
// .taskconfig (global), and pins WHICH KEY each tier ends up contributing.
//
// Two distinct things are asserted here, and mixing them up is what made the original
// "mixed spellings" case vacuous:
//
//   - `programs: {search_paths:}` and `programs.search_paths:` are two SPELLINGS of one
//     key — the flattener collapses them, so writing one in each tier only exercises
//     tier precedence, not spelling;
//   - `programs_search_paths` and `programs.search_paths` are two DIFFERENT KEYS that
//     merely mean the same thing to internal/cli/program.go. The config layer must keep
//     them apart and report each one's own winning tier. Choosing between them is the
//     CONSUMER's job (program.go's ordered key list); the config layer only has to be
//     honest about what each tier set.
func TestCustomSettings_FlattenedKeyTierPrecedence(t *testing.T) {
	const (
		// Spellings of the DOTTED key `programs.search_paths`.
		nested = "custom_settings:\n  programs:\n    search_paths: %q\n"
		dotted = "custom_settings:\n  programs.search_paths: %q\n"
		// The CANONICAL underscore key `programs_search_paths`, in-block and hoisted
		// to the top of the file (the shape Finding 1 was about).
		canonical  = "custom_settings:\n  programs_search_paths: %q\n"
		rootDotted = "custom_settings.programs_search_paths: %q\n"
	)

	type sourceWant struct {
		key   string
		value string
		tier  string
	}

	tests := []struct {
		name   string
		global string // format string for .taskconfig, "" to omit the setting
		org    string // format string for orgs/acme/config.yaml, "" to omit the tier
		repo   string // format string for .taskrc, "" to omit the setting
		want   []sourceWant
	}{
		{
			name:   "repo wins over org and global",
			global: nested,
			org:    dotted,
			repo:   nested,
			want:   []sourceWant{{"programs.search_paths", "/repo", "repo"}},
		},
		{
			name:   "org wins over global when repo is silent",
			global: dotted,
			org:    nested,
			want:   []sourceWant{{"programs.search_paths", "/org", "org"}},
		},
		{
			name:   "global applies when neither repo nor org defines it",
			global: nested,
			want:   []sourceWant{{"programs.search_paths", "/global", "global"}},
		},
		{
			name:   "nested and dotted spellings of one key collapse across tiers",
			global: nested,
			org:    nested,
			repo:   dotted,
			want:   []sourceWant{{"programs.search_paths", "/repo", "repo"}},
		},

		// ---- underscore vs dotted: DIFFERENT keys, each with its own winning tier ----
		{
			name:   "canonical in global and dotted in repo stay separate keys",
			global: canonical,
			repo:   dotted,
			want: []sourceWant{
				{"programs_search_paths", "/global", "global"},
				{"programs.search_paths", "/repo", "repo"},
			},
		},
		{
			name:   "canonical in repo and dotted in global stay separate keys",
			global: dotted,
			repo:   canonical,
			want: []sourceWant{
				{"programs_search_paths", "/repo", "repo"},
				{"programs.search_paths", "/global", "global"},
			},
		},
		{
			name:   "three tiers, three spellings, each key resolving to its own tier",
			global: canonical,
			// The org tier writes the canonical key at the TOP of its file, so this
			// also covers a hoisted dotted entry in a non-repo tier.
			org:  rootDotted,
			repo: dotted,
			want: []sourceWant{
				{"programs_search_paths", "/org", "org"},
				{"programs.search_paths", "/repo", "repo"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LoadConfig gives ADB_ORG precedence over .taskrc's `org:` field, so a
			// developer (or CI) with ADB_ORG exported would silently point the org tier
			// somewhere else and fail these cases. Pin it empty — the sibling
			// TestLoadConfig_OrgTier_FromEnvOverride is where the env path is tested.
			// $HOME needs no isolation here: every case passes an explicit
			// globalConfigPath, and NewViperConfigManager only reaches for
			// os.UserHomeDir() when that argument is empty.
			t.Setenv("ADB_ORG", "")

			tmpDir := t.TempDir()
			globalPath := filepath.Join(tmpDir, ".taskconfig")
			repoPath := filepath.Join(tmpDir, ".taskrc")

			globalBody := ""
			if tt.global != "" {
				globalBody = fmt.Sprintf(tt.global, "/global")
			}
			if err := os.WriteFile(globalPath, []byte("task_id_prefix: \"TASK\"\n"+globalBody), 0o644); err != nil {
				t.Fatalf("write .taskconfig: %v", err)
			}

			// The org tier is selected by .taskrc's `org:` field; declare it only when
			// the case exercises that tier so the others keep the two-tier behaviour.
			repoBody := ""
			if tt.org != "" {
				repoBody = "org: \"acme\"\n"
				orgDir := filepath.Join(tmpDir, "orgs", "acme")
				if err := os.MkdirAll(orgDir, 0o755); err != nil {
					t.Fatalf("mkdir org dir: %v", err)
				}
				orgBody := fmt.Sprintf(tt.org, "/org")
				if err := os.WriteFile(filepath.Join(orgDir, "config.yaml"), []byte(orgBody), 0o644); err != nil {
					t.Fatalf("write org config: %v", err)
				}
			}
			if tt.repo != "" {
				repoBody += fmt.Sprintf(tt.repo, "/repo")
			}
			if err := os.WriteFile(repoPath, []byte(repoBody), 0o644); err != nil {
				t.Fatalf("write .taskrc: %v", err)
			}

			cm := NewViperConfigManager(globalPath, repoPath)
			merged, err := cm.LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}

			for _, w := range tt.want {
				value, tier, ok := merged.SettingSource(w.key)
				if !ok {
					t.Errorf("SettingSource(%q) unresolved; global=%#v org=%#v repo=%#v",
						w.key, merged.Global.CustomSettings, merged.Org, merged.Repo.CustomSettings)
					continue
				}
				if value != w.value || tier != w.tier {
					t.Errorf("SettingSource(%q) = (%q, %q), want (%q, %q)",
						w.key, value, tier, w.value, w.tier)
				}
			}
		})
	}
}

// TestFlattenCustomSettings_Unit covers the flattener directly, including shapes a YAML
// file cannot easily produce (map[any]any from a differently-parsed source).
func TestFlattenCustomSettings_Unit(t *testing.T) {
	tests := []struct {
		name        string
		raw         any
		want        map[string]string
		wantReached []string // keys that must appear in the `reached` set
		wantWarn    int
	}{
		{
			name: "already flat",
			raw:  map[string]any{"a": "1", "b": "2"},
			want: map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "nested map[any]any is tolerated",
			raw:  map[any]any{"programs": map[any]any{"search_paths": "/x"}},
			want: map[string]string{"programs.search_paths": "/x"},
		},
		{
			name:     "list leaf is skipped with one warning",
			raw:      map[string]any{"ok": "yes", "bad": []any{"a"}},
			want:     map[string]string{"ok": "yes"},
			wantWarn: 1,
		},
		{
			name:     "non-map block yields one warning and no settings",
			raw:      []any{"a", "b"},
			want:     map[string]string{},
			wantWarn: 1,
		},
		{
			name: "empty map yields nothing",
			raw:  map[string]any{},
			want: map[string]string{},
		},
		{
			// An absent block is not a problem: the tier may still carry top-level
			// `custom_settings.<key>:` entries, which the caller merges in.
			name: "nil block yields nothing and no warning",
			raw:  nil,
			want: map[string]string{},
		},
		{
			name:        "empty nested block warns, like a nil leaf",
			raw:         map[string]any{"ok": "yes", "hollow": map[string]any{}},
			want:        map[string]string{"ok": "yes"},
			wantWarn:    1,
			wantReached: []string{"ok", "hollow"},
		},
		{
			// reached must include a leaf that was SKIPPED, not just the ones that
			// produced a value: mergeRootCustomSettings uses it to avoid reporting the
			// same bad leaf twice.
			name:        "reached covers skipped leaves too",
			raw:         map[string]any{"ok": "yes", "bad": []any{"a"}, "deep": map[string]any{"x": nil}},
			want:        map[string]string{"ok": "yes"},
			wantWarn:    2,
			wantReached: []string{"ok", "bad", "deep.x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reached, warnings := flattenCustomSettings(tt.raw)
			if len(got) != len(tt.want) {
				t.Errorf("flattened = %#v, want %#v", got, tt.want)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("flattened[%q] = %q, want %q", k, got[k], want)
				}
			}
			if len(warnings) != tt.wantWarn {
				t.Errorf("warnings = %#v, want %d", warnings, tt.wantWarn)
			}
			for _, k := range tt.wantReached {
				if !reached[k] {
					t.Errorf("reached[%q] = false, want true; reached = %#v", k, reached)
				}
			}
		})
	}
}
