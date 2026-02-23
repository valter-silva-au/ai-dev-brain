package models

// CLIAliasConfig holds a CLI alias definition from the global config.
type CLIAliasConfig struct {
	Name        string   `yaml:"name" mapstructure:"name"`
	Command     string   `yaml:"command" mapstructure:"command"`
	DefaultArgs []string `yaml:"default_args,omitempty" mapstructure:"default_args"`
}

// SlackConfig holds Slack notification settings.
type SlackConfig struct {
	WebhookURL string `yaml:"webhook_url" mapstructure:"webhook_url"`
}

// AlertThresholds configures when alerts should fire.
type AlertThresholds struct {
	BlockedHours   int `yaml:"blocked_threshold_hours" mapstructure:"blocked_threshold_hours"`
	StaleDays      int `yaml:"stale_threshold_days" mapstructure:"stale_threshold_days"`
	ReviewDays     int `yaml:"review_threshold_days" mapstructure:"review_threshold_days"`
	MaxBacklogSize int `yaml:"max_backlog_size" mapstructure:"max_backlog_size"`
}

// NotificationConfig holds notification and alerting settings.
type NotificationConfig struct {
	Enabled bool            `yaml:"enabled" mapstructure:"enabled"`
	Slack   SlackConfig     `yaml:"slack,omitempty" mapstructure:"slack"`
	Alerts  AlertThresholds `yaml:"alerts,omitempty" mapstructure:"alerts"`
}

// TeamRoutingRule maps task tags to team configurations for automatic team assignment.
type TeamRoutingRule struct {
	Tags     []string `yaml:"tags" mapstructure:"tags"`
	TeamName string   `yaml:"team_name" mapstructure:"team_name"`
	Members  []string `yaml:"members,omitempty" mapstructure:"members"`
}

// TeamRoutingConfig holds team routing configuration for multi-agent orchestration.
type TeamRoutingConfig struct {
	Enabled bool              `yaml:"enabled" mapstructure:"enabled"`
	Rules   []TeamRoutingRule `yaml:"rules,omitempty" mapstructure:"rules"`
}

// HookConfig defines metadata for a hook execution policy.
type HookConfig struct {
	Name       string `yaml:"name" mapstructure:"name"`
	TimeoutSec int    `yaml:"timeout_sec,omitempty" mapstructure:"timeout_sec"`
	Retries    int    `yaml:"retries,omitempty" mapstructure:"retries"`
	OnFailure  string `yaml:"on_failure,omitempty" mapstructure:"on_failure"` // warn, block, ignore
}

// GlobalConfig holds system-wide settings read from .taskconfig via Viper.
type GlobalConfig struct {
	DefaultAI        string               `yaml:"default_ai" mapstructure:"default_ai"`
	TaskIDPrefix     string               `yaml:"task_id_prefix" mapstructure:"task_id_prefix"`
	TaskIDCounter    int                  `yaml:"task_id_counter" mapstructure:"task_id_counter"`
	TaskIDPadWidth   int                  `yaml:"task_id_pad_width" mapstructure:"task_id_pad_width"`
	BranchPattern    string               `yaml:"branch_pattern" mapstructure:"branch_pattern"`
	DefaultPriority  Priority             `yaml:"default_priority" mapstructure:"default_priority"`
	DefaultOwner     string               `yaml:"default_owner" mapstructure:"default_owner"`
	ScreenshotHotkey string               `yaml:"screenshot_hotkey" mapstructure:"screenshot_hotkey"`
	OfflineMode      bool                 `yaml:"offline_mode" mapstructure:"offline_mode"`
	CLIAliases       []CLIAliasConfig     `yaml:"cli_aliases,omitempty" mapstructure:"cli_aliases"`
	Notifications    NotificationConfig   `yaml:"notifications,omitempty" mapstructure:"notifications"`
	SessionCapture   SessionCaptureConfig `yaml:"session_capture,omitempty" mapstructure:"session_capture"`
	TeamRouting      TeamRoutingConfig    `yaml:"team_routing,omitempty" mapstructure:"team_routing"`
	Hooks            []HookConfig         `yaml:"hooks,omitempty" mapstructure:"hooks"`
}

// RepoConfig holds per-repository settings read from .taskrc files.
type RepoConfig struct {
	BuildCommand     string              `yaml:"build_command,omitempty" mapstructure:"build_command"`
	TestCommand      string              `yaml:"test_command,omitempty" mapstructure:"test_command"`
	DefaultReviewers []string            `yaml:"default_reviewers,omitempty" mapstructure:"default_reviewers"`
	Conventions      []string            `yaml:"conventions,omitempty" mapstructure:"conventions"`
	Templates        map[TaskType]string `yaml:"templates,omitempty" mapstructure:"templates"`
}

// MergedConfig combines global and repository-specific configuration,
// with repository settings taking precedence over global defaults.
type MergedConfig struct {
	GlobalConfig `yaml:",inline" mapstructure:",squash"`
	Repo         *RepoConfig `yaml:"repo,omitempty" mapstructure:"repo"`
}
