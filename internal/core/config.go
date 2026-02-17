// Package core contains the business logic for AI Dev Brain,
// including task management, bootstrap, configuration, knowledge extraction,
// conflict detection, and AI context generation.
package core

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/viper"
)

// validPrefixPattern matches uppercase alphanumeric prefixes between 1 and 10 characters.
var validPrefixPattern = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)

// ConfigurationManager defines the interface for loading, merging, and
// validating configuration from global (.taskconfig) and per-repo (.taskrc) files.
type ConfigurationManager interface {
	LoadGlobalConfig() (*models.GlobalConfig, error)
	LoadRepoConfig(repoPath string) (*models.RepoConfig, error)
	GetMergedConfig(repoPath string) (*models.MergedConfig, error)
	ValidateConfig(config interface{}) error
}

// viperConfigManager implements ConfigurationManager using Viper for
// reading YAML configuration files.
type viperConfigManager struct {
	// basePath is the root directory where .taskconfig resides.
	basePath string
}

// NewConfigurationManager creates a new ConfigurationManager that reads
// configuration files relative to basePath.
func NewConfigurationManager(basePath string) ConfigurationManager {
	return &viperConfigManager{basePath: basePath}
}

// defaultGlobalConfig returns a GlobalConfig populated with sensible defaults.
func defaultGlobalConfig() *models.GlobalConfig {
	return &models.GlobalConfig{
		DefaultAI:        "kiro",
		TaskIDPrefix:     "TASK",
		TaskIDCounter:    0,
		TaskIDPadWidth:   5,
		BranchPattern:    "{type}/{id}-{description}",
		DefaultPriority:  models.P2,
		DefaultOwner:     "",
		ScreenshotHotkey: "ctrl+shift+s",
		OfflineMode:      false,
	}
}

// LoadGlobalConfig reads the .taskconfig file from the base path using Viper.
// If the file does not exist, sensible defaults are returned.
func (cm *viperConfigManager) LoadGlobalConfig() (*models.GlobalConfig, error) {
	cfg := defaultGlobalConfig()

	v := viper.New()
	v.SetConfigName(".taskconfig")
	v.SetConfigType("yaml")
	v.AddConfigPath(cm.basePath)

	// Set Viper defaults so missing keys fall back gracefully.
	v.SetDefault("defaults.ai", cfg.DefaultAI)
	v.SetDefault("defaults.priority", string(cfg.DefaultPriority))
	v.SetDefault("defaults.owner", cfg.DefaultOwner)
	v.SetDefault("task_id.prefix", cfg.TaskIDPrefix)
	v.SetDefault("task_id.counter", cfg.TaskIDCounter)
	v.SetDefault("task_id.pad_width", cfg.TaskIDPadWidth)
	v.SetDefault("branch.pattern", cfg.BranchPattern)
	v.SetDefault("screenshot.hotkey", cfg.ScreenshotHotkey)
	v.SetDefault("offline_mode", cfg.OfflineMode)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// No config file found — return defaults.
			return cfg, nil
		}
		return nil, fmt.Errorf("reading .taskconfig: %w", err)
	}

	// Map nested YAML keys to flat GlobalConfig fields.
	cfg.DefaultAI = v.GetString("defaults.ai")
	cfg.DefaultPriority = models.Priority(v.GetString("defaults.priority"))
	cfg.DefaultOwner = v.GetString("defaults.owner")
	cfg.TaskIDPrefix = v.GetString("task_id.prefix")
	cfg.TaskIDCounter = v.GetInt("task_id.counter")
	cfg.ScreenshotHotkey = v.GetString("screenshot.hotkey")
	cfg.OfflineMode = v.GetBool("offline_mode")

	// Use IsSet to distinguish "not set" (use default 5) from "explicitly set to 0".
	if v.IsSet("task_id.pad_width") {
		cfg.TaskIDPadWidth = v.GetInt("task_id.pad_width")
	}
	cfg.BranchPattern = v.GetString("branch.pattern")

	// Parse cli_aliases section.
	var aliases []models.CLIAliasConfig
	rawAliases := v.Get("cli_aliases")
	if rawAliases != nil {
		if aliasSlice, ok := rawAliases.([]interface{}); ok {
			for _, item := range aliasSlice {
				if m, ok := item.(map[string]interface{}); ok {
					alias := models.CLIAliasConfig{}
					if name, ok := m["name"].(string); ok {
						alias.Name = name
					}
					if cmd, ok := m["command"].(string); ok {
						alias.Command = cmd
					}
					if args, ok := m["default_args"].([]interface{}); ok {
						for _, a := range args {
							if s, ok := a.(string); ok {
								alias.DefaultArgs = append(alias.DefaultArgs, s)
							}
						}
					}
					aliases = append(aliases, alias)
				}
			}
		}
	}
	cfg.CLIAliases = aliases

	return cfg, nil
}

// LoadRepoConfig reads a .taskrc file from the given repository path.
// If the file does not exist, nil is returned (no repo-specific config).
func (cm *viperConfigManager) LoadRepoConfig(repoPath string) (*models.RepoConfig, error) {
	v := viper.New()
	v.SetConfigName(".taskrc")
	v.SetConfigType("yaml")
	v.AddConfigPath(repoPath)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil, nil
		}
		return nil, fmt.Errorf("reading .taskrc in %s: %w", repoPath, err)
	}

	rc := &models.RepoConfig{
		BuildCommand:     v.GetString("build_command"),
		TestCommand:      v.GetString("test_command"),
		DefaultReviewers: v.GetStringSlice("default_reviewers"),
		Conventions:      v.GetStringSlice("conventions"),
	}

	// Parse templates map if present.
	templatesRaw := v.GetStringMapString("templates")
	if len(templatesRaw) > 0 {
		rc.Templates = make(map[models.TaskType]string, len(templatesRaw))
		for k, val := range templatesRaw {
			rc.Templates[models.TaskType(k)] = val
		}
	}

	return rc, nil
}

// GetMergedConfig loads the global config and overlays any repo-specific
// settings from .taskrc. Precedence: .taskrc > .taskconfig > defaults.
func (cm *viperConfigManager) GetMergedConfig(repoPath string) (*models.MergedConfig, error) {
	globalCfg, err := cm.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("loading global config for merge: %w", err)
	}

	merged := &models.MergedConfig{
		GlobalConfig: *globalCfg,
	}

	if repoPath == "" {
		return merged, nil
	}

	repoCfg, err := cm.LoadRepoConfig(repoPath)
	if err != nil {
		return nil, fmt.Errorf("loading repo config for merge: %w", err)
	}

	if repoCfg != nil {
		merged.Repo = repoCfg
	}

	return merged, nil
}

// ValidateConfig checks the provided configuration for invalid values and
// returns a clear error message identifying the problem.
// It accepts *GlobalConfig, *RepoConfig, or *MergedConfig.
func (cm *viperConfigManager) ValidateConfig(config interface{}) error {
	if config == nil {
		return fmt.Errorf("configuration is nil")
	}

	switch cfg := config.(type) {
	case *models.GlobalConfig:
		return validateGlobalConfig(cfg)
	case *models.RepoConfig:
		return validateRepoConfig(cfg)
	case *models.MergedConfig:
		if err := validateGlobalConfig(&cfg.GlobalConfig); err != nil {
			return err
		}
		if cfg.Repo != nil {
			return validateRepoConfig(cfg.Repo)
		}
		return nil
	default:
		return fmt.Errorf("unsupported configuration type: %T", config)
	}
}

// validPriorities is the set of allowed Priority values.
var validPriorities = map[models.Priority]bool{
	models.P0: true,
	models.P1: true,
	models.P2: true,
	models.P3: true,
}

// validTaskTypes is the set of allowed TaskType values.
var validTaskTypes = map[models.TaskType]bool{
	models.TaskTypeFeat:     true,
	models.TaskTypeBug:      true,
	models.TaskTypeSpike:    true,
	models.TaskTypeRefactor: true,
}

// validateGlobalConfig checks a GlobalConfig for invalid field values.
func validateGlobalConfig(cfg *models.GlobalConfig) error {
	if cfg == nil {
		return fmt.Errorf("global configuration is nil")
	}

	var errs []string

	if cfg.DefaultAI == "" {
		errs = append(errs, "defaults.ai must not be empty")
	}

	if cfg.TaskIDPrefix == "" {
		errs = append(errs, "task_id.prefix must not be empty")
	}

	if cfg.TaskIDCounter < 0 {
		errs = append(errs, fmt.Sprintf("task_id.counter must be non-negative, got %d", cfg.TaskIDCounter))
	}

	if cfg.DefaultPriority != "" && !validPriorities[cfg.DefaultPriority] {
		errs = append(errs, fmt.Sprintf(
			"defaults.priority %q is invalid, must be one of: P0, P1, P2, P3",
			cfg.DefaultPriority,
		))
	}

	if cfg.TaskIDPrefix != "" && !validPrefixPattern.MatchString(cfg.TaskIDPrefix) {
		errs = append(errs, fmt.Sprintf(
			"task_id.prefix %q is invalid, must match [A-Z0-9]{1,10}",
			cfg.TaskIDPrefix,
		))
	}

	if cfg.TaskIDPadWidth < 0 || cfg.TaskIDPadWidth > 10 {
		errs = append(errs, fmt.Sprintf(
			"task_id.pad_width %d is invalid, must be between 0 and 10",
			cfg.TaskIDPadWidth,
		))
	}

	if cfg.BranchPattern != "" && !strings.Contains(cfg.BranchPattern, "{id}") {
		errs = append(errs, fmt.Sprintf(
			"branch.pattern %q must contain {id} placeholder",
			cfg.BranchPattern,
		))
	}

	if len(errs) > 0 {
		return fmt.Errorf("global config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// validateRepoConfig checks a RepoConfig for invalid field values.
func validateRepoConfig(cfg *models.RepoConfig) error {
	if cfg == nil {
		return fmt.Errorf("repo configuration is nil")
	}

	var errs []string

	// Validate template task types if templates are specified.
	for taskType := range cfg.Templates {
		if !validTaskTypes[taskType] {
			errs = append(errs, fmt.Sprintf(
				"templates key %q is not a valid task type, must be one of: feat, bug, spike, refactor",
				taskType,
			))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("repo config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
