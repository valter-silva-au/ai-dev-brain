package models

// NotificationConfig holds notification settings
type NotificationConfig struct {
	Enabled  bool     `mapstructure:"enabled" yaml:"enabled"`
	Channels []string `mapstructure:"channels" yaml:"channels,omitempty"`
	OnEvents []string `mapstructure:"on_events" yaml:"on_events,omitempty"`
}

// TeamRoutingConfig holds team routing settings
type TeamRoutingConfig struct {
	Enabled      bool              `mapstructure:"enabled" yaml:"enabled"`
	DefaultTeam  string            `mapstructure:"default_team" yaml:"default_team,omitempty"`
	TeamPatterns map[string]string `mapstructure:"team_patterns" yaml:"team_patterns,omitempty"` // pattern -> team mapping
}

// HookConfig holds hook execution settings
type HookConfig struct {
	Enabled                 bool                   `mapstructure:"enabled" yaml:"enabled"`
	PreToolUse              bool                   `mapstructure:"pre_tool_use" yaml:"pre_tool_use"`
	PostToolUse             bool                   `mapstructure:"post_tool_use" yaml:"post_tool_use"`
	Stop                    bool                   `mapstructure:"stop" yaml:"stop"`
	TaskCompleted           bool                   `mapstructure:"task_completed" yaml:"task_completed"`
	SessionEnd              bool                   `mapstructure:"session_end" yaml:"session_end"`
	KnowledgeExtraction     bool                   `mapstructure:"knowledge_extraction" yaml:"knowledge_extraction"`
	ConflictDetection       bool                   `mapstructure:"conflict_detection" yaml:"conflict_detection"`
	AutoFormat              bool                   `mapstructure:"auto_format" yaml:"auto_format"`
	BlockVendorEdits        bool                   `mapstructure:"block_vendor_edits" yaml:"block_vendor_edits"`
	AllowedVendorPatterns   []string               `mapstructure:"allowed_vendor_patterns" yaml:"allowed_vendor_patterns,omitempty"`
	CustomPreToolUseScript  string                 `mapstructure:"custom_pre_tool_use_script" yaml:"custom_pre_tool_use_script,omitempty"`
	CustomPostToolUseScript string                 `mapstructure:"custom_post_tool_use_script" yaml:"custom_post_tool_use_script,omitempty"`
	EvidenceGate            EvidenceGateHookConfig `mapstructure:"evidence_gate" yaml:"evidence_gate,omitempty"`
	SpecGate                SpecGateHookConfig     `mapstructure:"spec_gate" yaml:"spec_gate,omitempty"`
	OperatorControls        OperatorControlsConfig `mapstructure:"operator_controls" yaml:"operator_controls,omitempty"`
	Memory                  MemoryHookConfig       `mapstructure:"memory" yaml:"memory,omitempty"`
}

// EvidenceGateHookConfig opts into the evidence-read gate, a
// long-running-agent pattern that blocks Write/Edit calls to guarded
// paths until a matching Read has been observed in the same session.
// Inspired by anthropics/cwc-long-running-agents. Defaults to disabled
// for backward compatibility.
type EvidenceGateHookConfig struct {
	Enabled      bool     `mapstructure:"enabled" yaml:"enabled"`
	WritePaths   []string `mapstructure:"write_paths" yaml:"write_paths,omitempty"`
	ReadPatterns []string `mapstructure:"read_patterns" yaml:"read_patterns,omitempty"`
}

// SpecGateHookConfig opts into the spec-gate (#128 step 16): Write/Edit to a
// guarded path is blocked until an accepted architecture decision (ADR) exists.
// Defaults to disabled. The precondition (an accepted ADR) is checked against the
// workspace's ADR registry — record one with `adb adr new` + `set-status`.
type SpecGateHookConfig struct {
	Enabled    bool     `mapstructure:"enabled" yaml:"enabled"`
	WritePaths []string `mapstructure:"write_paths" yaml:"write_paths,omitempty"`
}

// OperatorControlsConfig opts into operator-in-the-loop controls over
// long-running agents: a kill-switch that halts all PreToolUse when a
// sentinel file is present, and mid-run steering that surfaces a
// message from a file to the agent once. Defaults to disabled.
// Inspired by anthropics/cwc-long-running-agents.
type OperatorControlsConfig struct {
	KillSwitchEnabled bool   `mapstructure:"kill_switch_enabled" yaml:"kill_switch_enabled"`
	KillSwitchFile    string `mapstructure:"kill_switch_file" yaml:"kill_switch_file,omitempty"`
	SteerEnabled      bool   `mapstructure:"steer_enabled" yaml:"steer_enabled"`
	SteerFile         string `mapstructure:"steer_file" yaml:"steer_file,omitempty"`
}

// MemoryHookConfig opts into adb's namespaced vector-memory subsystem.
// When Enabled is true, hooks may auto-index ticket knowledge and
// session transcripts, and PreToolUse may surface "similar work" hints.
// The store itself is always reachable via `adb memory …` regardless of
// this flag; Enabled governs only the hook-driven auto-indexing and
// hints.
//
// Defaults to disabled for backward compatibility. See
// .wiki/concepts/Vector Memory in adb.md on the consumer monorepo.
type MemoryHookConfig struct {
	Enabled  bool               `mapstructure:"enabled" yaml:"enabled"`
	DBPath   string             `mapstructure:"db_path" yaml:"db_path,omitempty"`
	Embedder MemoryEmbedderConf `mapstructure:"embedder" yaml:"embedder"`
}

// MemoryEmbedderConf describes which embedding provider the memory
// subsystem should use when opened via config. Provider ∈ {fake, openai,
// ollama}. APIKey supports `$ENV_VAR` interpolation.
type MemoryEmbedderConf struct {
	Provider string `mapstructure:"provider" yaml:"provider"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint,omitempty"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	Dim      int    `mapstructure:"dim" yaml:"dim,omitempty"`
}

// CLIAliasConfig holds CLI alias definitions
type CLIAliasConfig struct {
	Aliases map[string]string `mapstructure:"aliases" yaml:"aliases,omitempty"` // alias -> command mapping
}

// AutomationConfig opts into the declarative rule engine's event-triggered
// dispatch (decision D7). Time-triggered rules run whenever the scheduler is up;
// EVENT-triggered rules only fire when Enabled is true, at which point the
// scheduler drains new .events.jsonl entries every DispatchInterval and fires
// matching rules. Defaults to disabled — event rules are inert until a workspace
// opts in, mirroring the hooks.memory opt-in. DispatchInterval is a Go duration
// (default 30s when empty).
type AutomationConfig struct {
	Enabled          bool   `mapstructure:"enabled" yaml:"enabled"`
	DispatchInterval string `mapstructure:"dispatch_interval" yaml:"dispatch_interval,omitempty"`
}

// GlobalConfig represents the global .taskconfig configuration
type GlobalConfig struct {
	TaskIDPrefix   string             `mapstructure:"task_id_prefix" yaml:"task_id_prefix"`
	BasePath       string             `mapstructure:"base_path" yaml:"base_path,omitempty"`
	Defaults       map[string]string  `mapstructure:"defaults" yaml:"defaults,omitempty"`
	Notifications  NotificationConfig `mapstructure:"notifications" yaml:"notifications"`
	TeamRouting    TeamRoutingConfig  `mapstructure:"team_routing" yaml:"team_routing"`
	Hooks          HookConfig         `mapstructure:"hooks" yaml:"hooks"`
	Aliases        CLIAliasConfig     `mapstructure:"aliases" yaml:"aliases"`
	Automation     AutomationConfig   `mapstructure:"automation" yaml:"automation"`
	MCPServers     map[string]string  `mapstructure:"mcp_servers" yaml:"mcp_servers,omitempty"` // name -> URL mapping
	FeatureFlags   map[string]bool    `mapstructure:"feature_flags" yaml:"feature_flags,omitempty"`
	CustomSettings map[string]string  `mapstructure:"custom_settings" yaml:"custom_settings,omitempty"`
}

// OrgConfig is the per-organization configuration tier, stored at
// orgs/<id>/config.yaml. It sits BETWEEN the Global (.taskconfig) and Repo
// (.taskrc) tiers so a business can set defaults once for all its repos and
// initiatives (decision D2 keys the tier off the org registry). Precedence is
// most-specific-wins: Repo > Org > Global. The tier is OPTIONAL — a workspace
// with no active org behaves exactly like the historical two-tier merge. It
// deliberately carries only the fields that make sense to fix once per business
// (defaults, review policy, hooks, free-form settings); repo-shaped build/branch
// concerns stay on RepoConfig.
type OrgConfig struct {
	OrgID          string            `mapstructure:"org_id" yaml:"org_id,omitempty"`
	Defaults       map[string]string `mapstructure:"defaults" yaml:"defaults,omitempty"`
	Reviewers      []string          `mapstructure:"reviewers" yaml:"reviewers,omitempty"`
	RequiredChecks []string          `mapstructure:"required_checks" yaml:"required_checks,omitempty"`
	Conventions    []string          `mapstructure:"conventions" yaml:"conventions,omitempty"`
	Hooks          HookConfig        `mapstructure:"hooks" yaml:"hooks,omitempty"`
	CustomSettings map[string]string `mapstructure:"custom_settings" yaml:"custom_settings,omitempty"`
}

// RepoConfig represents the per-repository .taskrc configuration
type RepoConfig struct {
	RepoName string `mapstructure:"repo_name" yaml:"repo_name,omitempty"`
	// Org names the active organization tier for this workspace — the id of an
	// entry in orgs/index.yaml whose orgs/<id>/config.yaml is merged between the
	// global and repo tiers. Empty (the default) means no org tier. The ADB_ORG
	// env var overrides this at load time. omitempty keeps pre-tier .taskrc files
	// byte-identical on marshal.
	Org              string            `mapstructure:"org" yaml:"org,omitempty"`
	BuildCommand     string            `mapstructure:"build_command" yaml:"build_command,omitempty"`
	TestCommand      string            `mapstructure:"test_command" yaml:"test_command,omitempty"`
	LintCommand      string            `mapstructure:"lint_command" yaml:"lint_command,omitempty"`
	Reviewers        []string          `mapstructure:"reviewers" yaml:"reviewers,omitempty"`
	RequiredChecks   []string          `mapstructure:"required_checks" yaml:"required_checks,omitempty"`
	Conventions      []string          `mapstructure:"conventions" yaml:"conventions,omitempty"`
	BaseBranch       string            `mapstructure:"base_branch" yaml:"base_branch,omitempty"`
	WorktreeBasePath string            `mapstructure:"worktree_base_path" yaml:"worktree_base_path,omitempty"`
	AutoSync         bool              `mapstructure:"auto_sync" yaml:"auto_sync"`
	CustomSettings   map[string]string `mapstructure:"custom_settings" yaml:"custom_settings,omitempty"`
	// Hooks here are a repo-level override for the global HookConfig.
	// Empty fields fall back to Global.Hooks (see
	// internal/cli/hook_options.go::hookOptionsFromConfig). Lets a
	// workspace enable evidence-gate / operator-controls / memory
	// without editing ~/.taskconfig.
	Hooks HookConfig `mapstructure:"hooks" yaml:"hooks,omitempty"`
}

// MergedConfig represents the combined configuration from the global, org, and
// repo tiers. The three pointers are the raw per-tier values; the Setting and
// ResolvedHooks methods apply the most-specific-wins precedence (Repo > Org >
// Global). Org is nil when no org tier is active (the historical two-tier case).
type MergedConfig struct {
	Global *GlobalConfig `mapstructure:"global" yaml:"global"`
	Org    *OrgConfig    `mapstructure:"org" yaml:"org,omitempty"`
	Repo   *RepoConfig   `mapstructure:"repo" yaml:"repo"`
}

// DefaultGlobalConfig returns a GlobalConfig with sensible defaults
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		TaskIDPrefix: "TASK",
		Defaults: map[string]string{
			"priority": "P2",
			"type":     "feat",
		},
		Notifications: NotificationConfig{
			Enabled:  false,
			Channels: []string{},
			OnEvents: []string{},
		},
		TeamRouting: TeamRoutingConfig{
			Enabled:      false,
			TeamPatterns: make(map[string]string),
		},
		Hooks: DefaultHookConfig(),
		Aliases: CLIAliasConfig{
			Aliases: make(map[string]string),
		},
		MCPServers:     make(map[string]string),
		FeatureFlags:   make(map[string]bool),
		CustomSettings: make(map[string]string),
	}
}

// DefaultHookConfig returns a HookConfig with Phase 1 features enabled
func DefaultHookConfig() HookConfig {
	return HookConfig{
		Enabled:               true,
		PreToolUse:            true,
		PostToolUse:           true,
		Stop:                  true,
		TaskCompleted:         true,
		SessionEnd:            true,
		KnowledgeExtraction:   false, // Phase 2/3 - opt-in
		ConflictDetection:     false, // Phase 2/3 - opt-in
		AutoFormat:            true,
		BlockVendorEdits:      true,
		AllowedVendorPatterns: []string{},
	}
}

// DefaultRepoConfig returns a RepoConfig with sensible defaults
func DefaultRepoConfig() *RepoConfig {
	return &RepoConfig{
		BaseBranch:     "main",
		AutoSync:       false,
		Reviewers:      []string{},
		RequiredChecks: []string{},
		Conventions:    []string{},
		CustomSettings: make(map[string]string),
	}
}

// NewMergedConfig creates a two-tier MergedConfig (no org tier) with optional
// global and repo configs. It is a convenience wrapper over
// NewMergedConfigWithOrg for callers that don't resolve an org.
func NewMergedConfig(global *GlobalConfig, repo *RepoConfig) *MergedConfig {
	return NewMergedConfigWithOrg(global, nil, repo)
}

// NewMergedConfigWithOrg creates a MergedConfig from all three tiers. A nil
// global or repo falls back to its default; a nil org leaves the org tier
// inactive (the historical two-tier precedence, Repo > Global).
func NewMergedConfigWithOrg(global *GlobalConfig, org *OrgConfig, repo *RepoConfig) *MergedConfig {
	if global == nil {
		global = DefaultGlobalConfig()
	}
	if repo == nil {
		repo = DefaultRepoConfig()
	}
	return &MergedConfig{
		Global: global,
		Org:    org,
		Repo:   repo,
	}
}

// Setting resolves a free-form custom setting by key with most-specific-wins
// precedence: Repo.CustomSettings > Org.CustomSettings > Global.CustomSettings.
// ok is false when no tier defines the key. It is the canonical demonstration of
// "set a value once per org, let a repo override it" — CustomSettings is the one
// free-form map present on all three tiers.
func (mc *MergedConfig) Setting(key string) (value string, ok bool) {
	if mc == nil {
		return "", false
	}
	if mc.Repo != nil {
		if v, found := mc.Repo.CustomSettings[key]; found {
			return v, true
		}
	}
	if mc.Org != nil {
		if v, found := mc.Org.CustomSettings[key]; found {
			return v, true
		}
	}
	if mc.Global != nil {
		if v, found := mc.Global.CustomSettings[key]; found {
			return v, true
		}
	}
	return "", false
}

// SettingSource resolves a custom setting and reports which tier ("repo",
// "org", or "global") supplied it. tier is "" when the key is undefined.
func (mc *MergedConfig) SettingSource(key string) (value, tier string, ok bool) {
	if mc == nil {
		return "", "", false
	}
	if mc.Repo != nil {
		if v, found := mc.Repo.CustomSettings[key]; found {
			return v, "repo", true
		}
	}
	if mc.Org != nil {
		if v, found := mc.Org.CustomSettings[key]; found {
			return v, "org", true
		}
	}
	if mc.Global != nil {
		if v, found := mc.Global.CustomSettings[key]; found {
			return v, "global", true
		}
	}
	return "", "", false
}

// ResolvedHooks merges the three tiers' HookConfig with most-specific-wins
// precedence (Global base, then Org, then Repo). It mirrors the shallow,
// per-sub-struct merge historically done in cli.resolvedHookConfig, generalized
// to include the org tier: a tier that enables the whole block replaces the
// base; a tier that enables a sub-feature (evidence gate / operator controls /
// memory) contributes just that sub-block. The most specific tier wins.
func (mc *MergedConfig) ResolvedHooks() HookConfig {
	var result HookConfig
	if mc == nil {
		return result
	}
	if mc.Global != nil {
		result = mc.Global.Hooks
	}
	apply := func(h HookConfig) {
		if h.Enabled {
			result = h
		}
		if h.EvidenceGate.Enabled {
			result.EvidenceGate = h.EvidenceGate
		}
		if h.SpecGate.Enabled {
			result.SpecGate = h.SpecGate
		}
		if h.OperatorControls.KillSwitchEnabled || h.OperatorControls.SteerEnabled {
			result.OperatorControls = h.OperatorControls
		}
		if h.Memory.Enabled {
			result.Memory = h.Memory
		}
	}
	if mc.Org != nil {
		apply(mc.Org.Hooks)
	}
	if mc.Repo != nil {
		apply(mc.Repo.Hooks)
	}
	return result
}
