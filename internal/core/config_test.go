package core

import (
	"os"
	"path/filepath"
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

	if !config.Notifications.Enabled {
		t.Error("expected notifications to be enabled")
	}

	if len(config.Notifications.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(config.Notifications.Channels))
	}

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
