package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// --- Helper ---

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// --- LoadGlobalConfig tests ---

func TestLoadGlobalConfig_Defaults_WhenNoFile(t *testing.T) {
	dir := t.TempDir()
	cm := NewConfigurationManager(dir)

	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DefaultAI != "kiro" {
		t.Errorf("DefaultAI = %q, want %q", cfg.DefaultAI, "kiro")
	}
	if cfg.TaskIDPrefix != "TASK" {
		t.Errorf("TaskIDPrefix = %q, want %q", cfg.TaskIDPrefix, "TASK")
	}
	if cfg.DefaultPriority != models.P2 {
		t.Errorf("DefaultPriority = %q, want %q", cfg.DefaultPriority, models.P2)
	}
	if cfg.OfflineMode != false {
		t.Errorf("OfflineMode = %v, want false", cfg.OfflineMode)
	}
	if cfg.ScreenshotHotkey != "ctrl+shift+s" {
		t.Errorf("ScreenshotHotkey = %q, want %q", cfg.ScreenshotHotkey, "ctrl+shift+s")
	}
	if cfg.TaskIDCounter != 0 {
		t.Errorf("TaskIDCounter = %d, want 0", cfg.TaskIDCounter)
	}
}

func TestLoadGlobalConfig_ReadsTaskconfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
version: "1.0"
defaults:
  ai: claude
  priority: P1
  owner: "@alice"
task_id:
  prefix: "PRJ"
  counter: 42
screenshot:
  hotkey: "ctrl+alt+p"
offline_mode: true
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DefaultAI != "claude" {
		t.Errorf("DefaultAI = %q, want %q", cfg.DefaultAI, "claude")
	}
	if cfg.DefaultPriority != models.P1 {
		t.Errorf("DefaultPriority = %q, want %q", cfg.DefaultPriority, models.P1)
	}
	if cfg.DefaultOwner != "@alice" {
		t.Errorf("DefaultOwner = %q, want %q", cfg.DefaultOwner, "@alice")
	}
	if cfg.TaskIDPrefix != "PRJ" {
		t.Errorf("TaskIDPrefix = %q, want %q", cfg.TaskIDPrefix, "PRJ")
	}
	if cfg.TaskIDCounter != 42 {
		t.Errorf("TaskIDCounter = %d, want 42", cfg.TaskIDCounter)
	}
	if cfg.ScreenshotHotkey != "ctrl+alt+p" {
		t.Errorf("ScreenshotHotkey = %q, want %q", cfg.ScreenshotHotkey, "ctrl+alt+p")
	}
	if cfg.OfflineMode != true {
		t.Errorf("OfflineMode = %v, want true", cfg.OfflineMode)
	}
}

func TestLoadGlobalConfig_PartialConfig_FillsDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: gemini
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DefaultAI != "gemini" {
		t.Errorf("DefaultAI = %q, want %q", cfg.DefaultAI, "gemini")
	}
	// Remaining fields should have defaults.
	if cfg.TaskIDPrefix != "TASK" {
		t.Errorf("TaskIDPrefix = %q, want default %q", cfg.TaskIDPrefix, "TASK")
	}
	if cfg.DefaultPriority != models.P2 {
		t.Errorf("DefaultPriority = %q, want default %q", cfg.DefaultPriority, models.P2)
	}
}

func TestLoadGlobalConfig_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: [invalid yaml
  broken: {
`)

	cm := NewConfigurationManager(dir)
	_, err := cm.LoadGlobalConfig()
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// --- LoadRepoConfig tests ---

func TestLoadRepoConfig_NoFile_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	cm := NewConfigurationManager(dir)

	rc, err := cm.LoadRepoConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc != nil {
		t.Errorf("expected nil RepoConfig when no .taskrc, got %+v", rc)
	}
}

func TestLoadRepoConfig_ReadsTaskrc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskrc.yaml", `
build_command: "make build"
test_command: "make test"
default_reviewers:
  - "@bob"
  - "@carol"
conventions:
  - "Use gofmt"
  - "No globals"
templates:
  feat: "templates/feat.md"
  bug: "templates/bug.md"
`)

	cm := NewConfigurationManager(dir)
	rc, err := cm.LoadRepoConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc == nil {
		t.Fatal("expected non-nil RepoConfig")
	}

	if rc.BuildCommand != "make build" {
		t.Errorf("BuildCommand = %q, want %q", rc.BuildCommand, "make build")
	}
	if rc.TestCommand != "make test" {
		t.Errorf("TestCommand = %q, want %q", rc.TestCommand, "make test")
	}
	if len(rc.DefaultReviewers) != 2 {
		t.Errorf("DefaultReviewers len = %d, want 2", len(rc.DefaultReviewers))
	}
	if len(rc.Conventions) != 2 {
		t.Errorf("Conventions len = %d, want 2", len(rc.Conventions))
	}
	if rc.Templates[models.TaskTypeFeat] != "templates/feat.md" {
		t.Errorf("Templates[feat] = %q, want %q", rc.Templates[models.TaskTypeFeat], "templates/feat.md")
	}
	if rc.Templates[models.TaskTypeBug] != "templates/bug.md" {
		t.Errorf("Templates[bug] = %q, want %q", rc.Templates[models.TaskTypeBug], "templates/bug.md")
	}
}

// --- GetMergedConfig tests ---

func TestGetMergedConfig_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: claude
  priority: P0
`)

	cm := NewConfigurationManager(dir)
	merged, err := cm.GetMergedConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if merged.DefaultAI != "claude" {
		t.Errorf("DefaultAI = %q, want %q", merged.DefaultAI, "claude")
	}
	if merged.DefaultPriority != models.P0 {
		t.Errorf("DefaultPriority = %q, want %q", merged.DefaultPriority, models.P0)
	}
	if merged.Repo != nil {
		t.Errorf("expected nil Repo, got %+v", merged.Repo)
	}
}

func TestGetMergedConfig_WithRepoOverlay(t *testing.T) {
	globalDir := t.TempDir()
	repoDir := t.TempDir()

	writeFile(t, globalDir, ".taskconfig.yaml", `
defaults:
  ai: kiro
  priority: P2
  owner: "@global-user"
`)
	writeFile(t, repoDir, ".taskrc.yaml", `
build_command: "cargo build"
test_command: "cargo test"
default_reviewers:
  - "@repo-reviewer"
`)

	cm := NewConfigurationManager(globalDir)
	merged, err := cm.GetMergedConfig(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Global values should be present.
	if merged.DefaultAI != "kiro" {
		t.Errorf("DefaultAI = %q, want %q", merged.DefaultAI, "kiro")
	}
	if merged.DefaultOwner != "@global-user" {
		t.Errorf("DefaultOwner = %q, want %q", merged.DefaultOwner, "@global-user")
	}

	// Repo overlay should be present.
	if merged.Repo == nil {
		t.Fatal("expected non-nil Repo in merged config")
	}
	if merged.Repo.BuildCommand != "cargo build" {
		t.Errorf("Repo.BuildCommand = %q, want %q", merged.Repo.BuildCommand, "cargo build")
	}
	if len(merged.Repo.DefaultReviewers) != 1 || merged.Repo.DefaultReviewers[0] != "@repo-reviewer" {
		t.Errorf("Repo.DefaultReviewers = %v, want [\"@repo-reviewer\"]", merged.Repo.DefaultReviewers)
	}
}

func TestGetMergedConfig_NoRepoConfig(t *testing.T) {
	globalDir := t.TempDir()
	repoDir := t.TempDir() // No .taskrc here.

	writeFile(t, globalDir, ".taskconfig.yaml", `
defaults:
  ai: claude
`)

	cm := NewConfigurationManager(globalDir)
	merged, err := cm.GetMergedConfig(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if merged.DefaultAI != "claude" {
		t.Errorf("DefaultAI = %q, want %q", merged.DefaultAI, "claude")
	}
	if merged.Repo != nil {
		t.Errorf("expected nil Repo when no .taskrc, got %+v", merged.Repo)
	}
}

// --- ValidateConfig tests ---

func TestValidateConfig_ValidGlobalConfig(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		TaskIDCounter:   0,
		DefaultPriority: models.P2,
	}

	if err := cm.ValidateConfig(cfg); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateConfig_EmptyAI_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.P2,
	}

	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty DefaultAI")
	}
}

func TestValidateConfig_EmptyPrefix_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "",
		DefaultPriority: models.P2,
	}

	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty TaskIDPrefix")
	}
}

func TestValidateConfig_InvalidPriority_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.Priority("P9"),
	}

	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid priority P9")
	}
}

func TestValidateConfig_NegativeCounter_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		TaskIDCounter:   -1,
		DefaultPriority: models.P2,
	}

	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for negative counter")
	}
}

func TestValidateConfig_NilConfig_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())

	err := cm.ValidateConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestValidateConfig_UnsupportedType_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())

	err := cm.ValidateConfig("not a config")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestValidateConfig_ValidRepoConfig(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	rc := &models.RepoConfig{
		BuildCommand: "go build",
		Templates: map[models.TaskType]string{
			models.TaskTypeFeat: "feat.md",
		},
	}

	if err := cm.ValidateConfig(rc); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateConfig_InvalidTemplateType_ReturnsError(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	rc := &models.RepoConfig{
		Templates: map[models.TaskType]string{
			models.TaskType("invalid"): "bad.md",
		},
	}

	err := cm.ValidateConfig(rc)
	if err == nil {
		t.Fatal("expected validation error for invalid template task type")
	}
}

func TestValidateConfig_MergedConfig(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	merged := &models.MergedConfig{
		GlobalConfig: models.GlobalConfig{
			DefaultAI:       "kiro",
			TaskIDPrefix:    "TASK",
			DefaultPriority: models.P2,
		},
		Repo: &models.RepoConfig{
			BuildCommand: "make",
		},
	}

	if err := cm.ValidateConfig(merged); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateConfig_MergedConfig_InvalidGlobal(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	merged := &models.MergedConfig{
		GlobalConfig: models.GlobalConfig{
			DefaultAI:       "",
			TaskIDPrefix:    "TASK",
			DefaultPriority: models.P2,
		},
	}

	err := cm.ValidateConfig(merged)
	if err == nil {
		t.Fatal("expected validation error for invalid global in merged config")
	}
}

func TestValidateConfig_MergedConfig_InvalidRepo(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	merged := &models.MergedConfig{
		GlobalConfig: models.GlobalConfig{
			DefaultAI:       "kiro",
			TaskIDPrefix:    "TASK",
			DefaultPriority: models.P2,
		},
		Repo: &models.RepoConfig{
			Templates: map[models.TaskType]string{
				models.TaskType("nope"): "bad.md",
			},
		},
	}

	err := cm.ValidateConfig(merged)
	if err == nil {
		t.Fatal("expected validation error for invalid repo in merged config")
	}
}

// --- CLI Aliases tests ---

func TestLoadGlobalConfig_ReadsCLIAliases(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
version: "1.0"
defaults:
  ai: kiro
  priority: P2
cli_aliases:
  - name: aws
    command: aws
    default_args: ["--profile", "myprofile", "--region", "us-east-1"]
  - name: gh
    command: gh
  - name: k
    command: kubectl
    default_args: ["--context", "dev-cluster"]
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.CLIAliases) != 3 {
		t.Fatalf("CLIAliases count = %d, want 3", len(cfg.CLIAliases))
	}

	// Check first alias.
	if cfg.CLIAliases[0].Name != "aws" {
		t.Errorf("alias[0].Name = %q, want %q", cfg.CLIAliases[0].Name, "aws")
	}
	if cfg.CLIAliases[0].Command != "aws" {
		t.Errorf("alias[0].Command = %q, want %q", cfg.CLIAliases[0].Command, "aws")
	}
	if len(cfg.CLIAliases[0].DefaultArgs) != 4 {
		t.Errorf("alias[0].DefaultArgs len = %d, want 4", len(cfg.CLIAliases[0].DefaultArgs))
	}

	// Check alias with no default args.
	if cfg.CLIAliases[1].Name != "gh" {
		t.Errorf("alias[1].Name = %q, want %q", cfg.CLIAliases[1].Name, "gh")
	}
	if len(cfg.CLIAliases[1].DefaultArgs) != 0 {
		t.Errorf("alias[1].DefaultArgs len = %d, want 0", len(cfg.CLIAliases[1].DefaultArgs))
	}

	// Check third alias.
	if cfg.CLIAliases[2].Name != "k" {
		t.Errorf("alias[2].Name = %q, want %q", cfg.CLIAliases[2].Name, "k")
	}
	if cfg.CLIAliases[2].Command != "kubectl" {
		t.Errorf("alias[2].Command = %q, want %q", cfg.CLIAliases[2].Command, "kubectl")
	}
}

func TestLoadGlobalConfig_NoCLIAliases_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
version: "1.0"
defaults:
  ai: kiro
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.CLIAliases) != 0 {
		t.Errorf("CLIAliases count = %d, want 0", len(cfg.CLIAliases))
	}
}

func TestLoadGlobalConfig_DefaultsHaveNoCLIAliases(t *testing.T) {
	dir := t.TempDir()
	cm := NewConfigurationManager(dir)

	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.CLIAliases) != 0 {
		t.Errorf("default CLIAliases count = %d, want 0", len(cfg.CLIAliases))
	}
}

func TestLoadRepoConfig_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskrc.yaml", `
build_command: [invalid yaml
broken: {
`)
	cm := NewConfigurationManager(dir)
	_, err := cm.LoadRepoConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML in .taskrc")
	}
	if !strings.Contains(err.Error(), ".taskrc") {
		t.Errorf("expected error to mention .taskrc, got: %v", err)
	}
}

func TestGetMergedConfig_GlobalLoadError(t *testing.T) {
	dir := t.TempDir()
	// Write invalid YAML to cause global config load to fail.
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: [invalid
  broken: {
`)
	cm := NewConfigurationManager(dir)
	_, err := cm.GetMergedConfig("")
	if err == nil {
		t.Fatal("expected error when global config is invalid")
	}
	if !strings.Contains(err.Error(), "loading global config for merge") {
		t.Errorf("expected global config merge error, got: %v", err)
	}
}

func TestGetMergedConfig_RepoLoadError(t *testing.T) {
	globalDir := t.TempDir()
	repoDir := t.TempDir()
	// Write invalid YAML to cause repo config load to fail.
	writeFile(t, repoDir, ".taskrc.yaml", `
build_command: [invalid yaml
`)
	cm := NewConfigurationManager(globalDir)
	_, err := cm.GetMergedConfig(repoDir)
	if err == nil {
		t.Fatal("expected error when repo config is invalid")
	}
	if !strings.Contains(err.Error(), "loading repo config for merge") {
		t.Errorf("expected repo config merge error, got: %v", err)
	}
}

func TestValidateConfig_MergedConfig_NilRepo(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	merged := &models.MergedConfig{
		GlobalConfig: models.GlobalConfig{
			DefaultAI:       "kiro",
			TaskIDPrefix:    "TASK",
			DefaultPriority: models.P2,
		},
		Repo: nil,
	}
	if err := cm.ValidateConfig(merged); err != nil {
		t.Errorf("expected no error for merged config with nil Repo, got: %v", err)
	}
}

func TestValidateGlobalConfig_NilConfig_ReturnsError(t *testing.T) {
	err := validateGlobalConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil global config")
	}
	if !strings.Contains(err.Error(), "global configuration is nil") {
		t.Errorf("expected nil config error, got: %v", err)
	}
}

func TestValidateRepoConfig_NilConfig_ReturnsError(t *testing.T) {
	err := validateRepoConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil repo config")
	}
	if !strings.Contains(err.Error(), "repo configuration is nil") {
		t.Errorf("expected nil config error, got: %v", err)
	}
}

// --- PadWidth and BranchPattern tests ---

func TestLoadGlobalConfig_ReadsPadWidth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: kiro
task_id:
  prefix: "TASK"
  pad_width: 3
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskIDPadWidth != 3 {
		t.Errorf("TaskIDPadWidth = %d, want 3", cfg.TaskIDPadWidth)
	}
}

func TestLoadGlobalConfig_ReadsBranchPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: kiro
branch:
  pattern: "{id}-{description}"
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BranchPattern != "{id}-{description}" {
		t.Errorf("BranchPattern = %q, want %q", cfg.BranchPattern, "{id}-{description}")
	}
}

func TestLoadGlobalConfig_DefaultPadWidth(t *testing.T) {
	dir := t.TempDir()
	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskIDPadWidth != 5 {
		t.Errorf("TaskIDPadWidth = %d, want default 5", cfg.TaskIDPadWidth)
	}
}

func TestLoadGlobalConfig_ExplicitZeroPadWidth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".taskconfig.yaml", `
defaults:
  ai: kiro
task_id:
  prefix: "CCAAS"
  pad_width: 0
`)

	cm := NewConfigurationManager(dir)
	cfg, err := cm.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskIDPadWidth != 0 {
		t.Errorf("TaskIDPadWidth = %d, want 0 (explicitly set)", cfg.TaskIDPadWidth)
	}
}

func TestValidateConfig_InvalidPrefix_SpecialChars(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "MY-TASK",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  5,
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for prefix with special chars")
	}
	if !strings.Contains(err.Error(), "prefix") {
		t.Errorf("expected prefix error, got: %v", err)
	}
}

func TestValidateConfig_InvalidPrefix_Lowercase(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "task",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  5,
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for lowercase prefix")
	}
	if !strings.Contains(err.Error(), "prefix") {
		t.Errorf("expected prefix error, got: %v", err)
	}
}

func TestValidateConfig_InvalidPrefix_TooLong(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "ABCDEFGHIJK",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  5,
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for prefix longer than 10 chars")
	}
	if !strings.Contains(err.Error(), "prefix") {
		t.Errorf("expected prefix error, got: %v", err)
	}
}

func TestValidateConfig_PadWidthNegative(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  -1,
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for negative pad width")
	}
	if !strings.Contains(err.Error(), "pad_width") {
		t.Errorf("expected pad_width error, got: %v", err)
	}
}

func TestValidateConfig_PadWidthTooLarge(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  11,
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for pad width > 10")
	}
	if !strings.Contains(err.Error(), "pad_width") {
		t.Errorf("expected pad_width error, got: %v", err)
	}
}

func TestValidateConfig_BranchPatternMissingID(t *testing.T) {
	cm := NewConfigurationManager(t.TempDir())
	cfg := &models.GlobalConfig{
		DefaultAI:       "kiro",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.P2,
		TaskIDPadWidth:  5,
		BranchPattern:   "{type}/{description}",
	}
	err := cm.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for branch pattern missing {id}")
	}
	if !strings.Contains(err.Error(), "branch.pattern") {
		t.Errorf("expected branch.pattern error, got: %v", err)
	}
}
