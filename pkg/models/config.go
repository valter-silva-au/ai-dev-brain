package models

// CLIAliasConfig holds a CLI alias definition from the global config.
type CLIAliasConfig struct {
	Name        string   `yaml:"name" mapstructure:"name"`
	Command     string   `yaml:"command" mapstructure:"command"`
	DefaultArgs []string `yaml:"default_args,omitempty" mapstructure:"default_args"`
}

// GlobalConfig holds system-wide settings read from .taskconfig via Viper.
type GlobalConfig struct {
	DefaultAI        string           `yaml:"default_ai" mapstructure:"default_ai"`
	TaskIDPrefix     string           `yaml:"task_id_prefix" mapstructure:"task_id_prefix"`
	TaskIDCounter    int              `yaml:"task_id_counter" mapstructure:"task_id_counter"`
	DefaultPriority  Priority         `yaml:"default_priority" mapstructure:"default_priority"`
	DefaultOwner     string           `yaml:"default_owner" mapstructure:"default_owner"`
	ScreenshotHotkey string           `yaml:"screenshot_hotkey" mapstructure:"screenshot_hotkey"`
	OfflineMode      bool             `yaml:"offline_mode" mapstructure:"offline_mode"`
	CLIAliases       []CLIAliasConfig `yaml:"cli_aliases,omitempty" mapstructure:"cli_aliases"`
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
