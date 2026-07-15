package models

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultGlobalConfig(t *testing.T) {
	config := DefaultGlobalConfig()

	tests := []struct {
		name  string
		check func(*GlobalConfig) bool
		desc  string
	}{
		{
			name: "has default task ID prefix",
			check: func(c *GlobalConfig) bool {
				return c.TaskIDPrefix == "TASK"
			},
			desc: "TaskIDPrefix should be 'TASK'",
		},
		{
			name: "has default priority",
			check: func(c *GlobalConfig) bool {
				return c.Defaults["priority"] == "P2"
			},
			desc: "Default priority should be P2",
		},
		{
			name: "has default type",
			check: func(c *GlobalConfig) bool {
				return c.Defaults["type"] == "feat"
			},
			desc: "Default type should be feat",
		},
		{
			name: "notifications disabled by default",
			check: func(c *GlobalConfig) bool {
				return !c.Notifications.Enabled
			},
			desc: "Notifications should be disabled",
		},
		{
			name: "team routing disabled by default",
			check: func(c *GlobalConfig) bool {
				return !c.TeamRouting.Enabled
			},
			desc: "Team routing should be disabled",
		},
		{
			name: "hooks use defaults",
			check: func(c *GlobalConfig) bool {
				return c.Hooks.Enabled && c.Hooks.PreToolUse
			},
			desc: "Hooks should be enabled with PreToolUse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(config) {
				t.Errorf("%s", tt.desc)
			}
		})
	}
}

func TestDefaultHookConfig(t *testing.T) {
	config := DefaultHookConfig()

	tests := []struct {
		name  string
		field func(HookConfig) bool
		want  bool
	}{
		{"enabled", func(c HookConfig) bool { return c.Enabled }, true},
		{"pre_tool_use", func(c HookConfig) bool { return c.PreToolUse }, true},
		{"post_tool_use", func(c HookConfig) bool { return c.PostToolUse }, true},
		{"stop", func(c HookConfig) bool { return c.Stop }, true},
		{"task_completed", func(c HookConfig) bool { return c.TaskCompleted }, true},
		{"session_end", func(c HookConfig) bool { return c.SessionEnd }, true},
		{"knowledge_extraction", func(c HookConfig) bool { return c.KnowledgeExtraction }, false},
		{"conflict_detection", func(c HookConfig) bool { return c.ConflictDetection }, false},
		{"auto_format", func(c HookConfig) bool { return c.AutoFormat }, true},
		{"block_vendor_edits", func(c HookConfig) bool { return c.BlockVendorEdits }, true},
		{"evidence_gate disabled by default", func(c HookConfig) bool { return c.EvidenceGate.Enabled }, false},
		{"operator kill-switch disabled by default", func(c HookConfig) bool { return c.OperatorControls.KillSwitchEnabled }, false},
		{"operator steer disabled by default", func(c HookConfig) bool { return c.OperatorControls.SteerEnabled }, false},
		{"memory disabled by default", func(c HookConfig) bool { return c.Memory.Enabled }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.field(config); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestDefaultRepoConfig(t *testing.T) {
	config := DefaultRepoConfig()

	tests := []struct {
		name  string
		check func(*RepoConfig) bool
		desc  string
	}{
		{
			name: "base branch is main",
			check: func(c *RepoConfig) bool {
				return c.BaseBranch == "main"
			},
			desc: "BaseBranch should be 'main'",
		},
		{
			name: "auto sync disabled",
			check: func(c *RepoConfig) bool {
				return !c.AutoSync
			},
			desc: "AutoSync should be disabled",
		},
		{
			name: "empty reviewers",
			check: func(c *RepoConfig) bool {
				return len(c.Reviewers) == 0
			},
			desc: "Reviewers should be empty",
		},
		{
			name: "custom settings initialized",
			check: func(c *RepoConfig) bool {
				return c.CustomSettings != nil
			},
			desc: "CustomSettings should be initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(config) {
				t.Errorf("%s", tt.desc)
			}
		})
	}
}

func TestNewMergedConfig(t *testing.T) {
	tests := []struct {
		name   string
		global *GlobalConfig
		repo   *RepoConfig
		check  func(*MergedConfig) bool
	}{
		{
			name:   "both configs provided",
			global: DefaultGlobalConfig(),
			repo:   DefaultRepoConfig(),
			check: func(mc *MergedConfig) bool {
				return mc.Global != nil && mc.Repo != nil
			},
		},
		{
			name:   "nil global uses default",
			global: nil,
			repo:   DefaultRepoConfig(),
			check: func(mc *MergedConfig) bool {
				return mc.Global != nil && mc.Global.TaskIDPrefix == "TASK"
			},
		},
		{
			name:   "nil repo uses default",
			global: DefaultGlobalConfig(),
			repo:   nil,
			check: func(mc *MergedConfig) bool {
				return mc.Repo != nil && mc.Repo.BaseBranch == "main"
			},
		},
		{
			name:   "both nil use defaults",
			global: nil,
			repo:   nil,
			check: func(mc *MergedConfig) bool {
				return mc.Global != nil && mc.Repo != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := NewMergedConfig(tt.global, tt.repo)
			if !tt.check(merged) {
				t.Errorf("NewMergedConfig() validation failed for %s", tt.name)
			}
		})
	}
}

func TestOrgConfig_YAMLSerialization(t *testing.T) {
	config := &OrgConfig{
		OrgID:          "acme",
		Defaults:       map[string]string{"priority": "P1"},
		Reviewers:      []string{"lead-a", "lead-b"},
		RequiredChecks: []string{"ci"},
		Conventions:    []string{"conventional-commits"},
		Hooks:          HookConfig{Enabled: true, PreToolUse: true},
		CustomSettings: map[string]string{"deploy_target": "prod"},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal OrgConfig: %v", err)
	}

	var decoded OrgConfig
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal OrgConfig: %v", err)
	}

	if decoded.OrgID != config.OrgID {
		t.Errorf("OrgID = %v, want %v", decoded.OrgID, config.OrgID)
	}
	if decoded.CustomSettings["deploy_target"] != "prod" {
		t.Errorf("CustomSettings[deploy_target] = %v, want prod", decoded.CustomSettings["deploy_target"])
	}
	if len(decoded.Reviewers) != 2 {
		t.Errorf("Reviewers length = %v, want 2", len(decoded.Reviewers))
	}
}

func TestNewMergedConfigWithOrg(t *testing.T) {
	org := &OrgConfig{OrgID: "acme"}
	mc := NewMergedConfigWithOrg(nil, org, nil)
	if mc.Global == nil || mc.Repo == nil {
		t.Fatal("nil global/repo should fall back to defaults")
	}
	if mc.Org == nil || mc.Org.OrgID != "acme" {
		t.Errorf("Org tier not preserved: %+v", mc.Org)
	}

	// A nil org tier is legal (the historical two-tier case) and must not panic.
	mcNoOrg := NewMergedConfigWithOrg(DefaultGlobalConfig(), nil, DefaultRepoConfig())
	if mcNoOrg.Org != nil {
		t.Errorf("expected nil Org tier, got %+v", mcNoOrg.Org)
	}
	// NewMergedConfig stays a two-tier convenience (Org nil).
	if NewMergedConfig(nil, nil).Org != nil {
		t.Error("NewMergedConfig should leave Org nil")
	}
}

func TestMergedConfig_Setting_Precedence(t *testing.T) {
	// A custom setting present in all three tiers resolves most-specific-wins:
	// Repo > Org > Global.
	global := DefaultGlobalConfig()
	global.CustomSettings = map[string]string{"k": "global", "only_global": "g"}
	org := &OrgConfig{CustomSettings: map[string]string{"k": "org", "only_org": "o"}}
	repo := DefaultRepoConfig()
	repo.CustomSettings = map[string]string{"k": "repo"}
	mc := NewMergedConfigWithOrg(global, org, repo)

	tests := []struct {
		key    string
		want   string
		wantOK bool
	}{
		{"k", "repo", true},        // all three set → repo wins
		{"only_org", "o", true},    // only org set → org value
		{"only_global", "g", true}, // only global set → global value
		{"absent", "", false},      // no tier defines it
	}
	for _, tt := range tests {
		got, ok := mc.Setting(tt.key)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("Setting(%q) = (%q, %v), want (%q, %v)", tt.key, got, ok, tt.want, tt.wantOK)
		}
	}

	// With no org tier, precedence collapses to Repo > Global.
	mc2 := NewMergedConfigWithOrg(global, nil, repo)
	if v, _ := mc2.Setting("k"); v != "repo" {
		t.Errorf("no-org Setting(k) = %q, want repo", v)
	}
	if v, _ := mc2.Setting("only_global"); v != "g" {
		t.Errorf("no-org Setting(only_global) = %q, want g", v)
	}
}

func TestMergedConfig_ResolvedHooks_Precedence(t *testing.T) {
	// Global disables memory; Org enables it; Repo enables the evidence gate.
	// The resolved config carries the Org memory block and the Repo evidence
	// block — proving the three-tier shallow merge (Global base → Org → Repo).
	global := DefaultGlobalConfig()
	org := &OrgConfig{Hooks: HookConfig{Memory: MemoryHookConfig{Enabled: true, DBPath: "/org/mem.sqlite"}}}
	repo := DefaultRepoConfig()
	repo.Hooks = HookConfig{EvidenceGate: EvidenceGateHookConfig{Enabled: true, WritePaths: []string{"src/"}}}
	mc := NewMergedConfigWithOrg(global, org, repo)

	resolved := mc.ResolvedHooks()
	if !resolved.Memory.Enabled || resolved.Memory.DBPath != "/org/mem.sqlite" {
		t.Errorf("expected org memory block to survive, got %+v", resolved.Memory)
	}
	if !resolved.EvidenceGate.Enabled || len(resolved.EvidenceGate.WritePaths) != 1 {
		t.Errorf("expected repo evidence-gate block to survive, got %+v", resolved.EvidenceGate)
	}
	// Base flags (auto_format etc.) still come from the global default.
	if !resolved.AutoFormat {
		t.Error("expected global AutoFormat base to survive")
	}

	// Repo overrides Org for the same sub-block (memory): repo wins.
	repo.Hooks.Memory = MemoryHookConfig{Enabled: true, DBPath: "/repo/mem.sqlite"}
	mc = NewMergedConfigWithOrg(global, org, repo)
	if mc.ResolvedHooks().Memory.DBPath != "/repo/mem.sqlite" {
		t.Errorf("expected repo memory to win over org, got %q", mc.ResolvedHooks().Memory.DBPath)
	}
}

func TestGlobalConfig_YAMLSerialization(t *testing.T) {
	config := &GlobalConfig{
		TaskIDPrefix: "PROJ",
		BasePath:     "/home/user/projects",
		Defaults: map[string]string{
			"priority": "P1",
			"type":     "bug",
		},
		Notifications: NotificationConfig{
			Enabled:  true,
			Channels: []string{"email", "slack"},
			OnEvents: []string{"task.created", "task.completed"},
		},
		TeamRouting: TeamRoutingConfig{
			Enabled:     true,
			DefaultTeam: "backend",
			TeamPatterns: map[string]string{
				"api/*": "backend",
				"ui/*":  "frontend",
			},
		},
		Hooks: DefaultHookConfig(),
		Aliases: CLIAliasConfig{
			Aliases: map[string]string{
				"t": "task",
				"s": "status",
			},
		},
		MCPServers: map[string]string{
			"github": "http://localhost:8080",
		},
		FeatureFlags: map[string]bool{
			"experimental": true,
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Unmarshal back
	var decoded GlobalConfig
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify key fields
	if decoded.TaskIDPrefix != config.TaskIDPrefix {
		t.Errorf("TaskIDPrefix = %v, want %v", decoded.TaskIDPrefix, config.TaskIDPrefix)
	}
	if decoded.Notifications.Enabled != config.Notifications.Enabled {
		t.Errorf("Notifications.Enabled = %v, want %v", decoded.Notifications.Enabled, config.Notifications.Enabled)
	}
	if decoded.TeamRouting.DefaultTeam != config.TeamRouting.DefaultTeam {
		t.Errorf("TeamRouting.DefaultTeam = %v, want %v", decoded.TeamRouting.DefaultTeam, config.TeamRouting.DefaultTeam)
	}
}

func TestRepoConfig_YAMLSerialization(t *testing.T) {
	config := &RepoConfig{
		RepoName:         "test-repo",
		BuildCommand:     "make build",
		TestCommand:      "go test ./...",
		LintCommand:      "golangci-lint run",
		Reviewers:        []string{"user1", "user2"},
		RequiredChecks:   []string{"ci", "lint"},
		Conventions:      []string{"conventional-commits"},
		BaseBranch:       "develop",
		WorktreeBasePath: "/path/to/worktrees",
		AutoSync:         true,
		CustomSettings: map[string]string{
			"key": "value",
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Unmarshal back
	var decoded RepoConfig
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify key fields
	if decoded.RepoName != config.RepoName {
		t.Errorf("RepoName = %v, want %v", decoded.RepoName, config.RepoName)
	}
	if decoded.BuildCommand != config.BuildCommand {
		t.Errorf("BuildCommand = %v, want %v", decoded.BuildCommand, config.BuildCommand)
	}
	if decoded.BaseBranch != config.BaseBranch {
		t.Errorf("BaseBranch = %v, want %v", decoded.BaseBranch, config.BaseBranch)
	}
	if decoded.AutoSync != config.AutoSync {
		t.Errorf("AutoSync = %v, want %v", decoded.AutoSync, config.AutoSync)
	}
}

func TestHookConfig_YAMLTags(t *testing.T) {
	config := HookConfig{
		Enabled:               true,
		PreToolUse:            true,
		PostToolUse:           false,
		AllowedVendorPatterns: []string{"*.json"},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal HookConfig: %v", err)
	}

	var decoded HookConfig
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal HookConfig: %v", err)
	}

	if decoded.Enabled != config.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, config.Enabled)
	}
	if decoded.PreToolUse != config.PreToolUse {
		t.Errorf("PreToolUse = %v, want %v", decoded.PreToolUse, config.PreToolUse)
	}
	if decoded.PostToolUse != config.PostToolUse {
		t.Errorf("PostToolUse = %v, want %v", decoded.PostToolUse, config.PostToolUse)
	}
}

func TestNotificationConfig_YAMLSerialization(t *testing.T) {
	config := NotificationConfig{
		Enabled:  true,
		Channels: []string{"email", "slack"},
		OnEvents: []string{"task.created"},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal NotificationConfig: %v", err)
	}

	var decoded NotificationConfig
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal NotificationConfig: %v", err)
	}

	if decoded.Enabled != config.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, config.Enabled)
	}
	if len(decoded.Channels) != len(config.Channels) {
		t.Errorf("Channels length = %v, want %v", len(decoded.Channels), len(config.Channels))
	}
}
