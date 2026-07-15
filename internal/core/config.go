package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ConfigurationManager manages configuration loading from multiple sources
type ConfigurationManager interface {
	LoadConfig() (*models.MergedConfig, error)
	GetGlobalConfig() (*models.GlobalConfig, error)
	GetRepoConfig() (*models.RepoConfig, error)
	// GetOrgConfig loads the per-organization tier (orgs/<id>/config.yaml). It
	// returns (nil, nil) for an empty id or a missing file — the tier is optional.
	GetOrgConfig(orgID string) (*models.OrgConfig, error)
}

// ViperConfigManager implements ConfigurationManager using Viper
type ViperConfigManager struct {
	globalConfigPath string
	repoConfigPath   string
	// orgsDir holds the per-org config tree (orgs/<id>/config.yaml). It is
	// derived from the repo config's directory so the org tier lives beside the
	// .taskrc that selects it — matching how the org registry (orgs/index.yaml)
	// is rooted at the workspace.
	orgsDir string
}

// NewViperConfigManager creates a new configuration manager
// If paths are empty, defaults are used:
// - globalConfigPath: ~/.taskconfig
// - repoConfigPath: ./.taskrc
// The org tier root is derived as <dir of repoConfigPath>/orgs.
func NewViperConfigManager(globalConfigPath, repoConfigPath string) *ViperConfigManager {
	if globalConfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			globalConfigPath = filepath.Join(homeDir, ".taskconfig")
		}
	}

	if repoConfigPath == "" {
		repoConfigPath = ".taskrc"
	}

	return &ViperConfigManager{
		globalConfigPath: globalConfigPath,
		repoConfigPath:   repoConfigPath,
		orgsDir:          filepath.Join(filepath.Dir(repoConfigPath), "orgs"),
	}
}

// GetGlobalConfig loads the global configuration from .taskconfig
func (cm *ViperConfigManager) GetGlobalConfig() (*models.GlobalConfig, error) {
	// Check if global config file exists
	if _, err := os.Stat(cm.globalConfigPath); os.IsNotExist(err) {
		// File doesn't exist, return defaults
		return models.DefaultGlobalConfig(), nil
	}

	// Create a new Viper instance for global config
	v := viper.New()
	v.SetConfigFile(cm.globalConfigPath)
	v.SetConfigType("yaml")

	// Read the config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	// Create empty config struct and unmarshal
	var config models.GlobalConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal global config: %w", err)
	}

	// Apply defaults for any empty fields
	defaults := models.DefaultGlobalConfig()

	if config.TaskIDPrefix == "" {
		config.TaskIDPrefix = defaults.TaskIDPrefix
	}

	if config.Defaults == nil {
		config.Defaults = defaults.Defaults
	}

	if config.MCPServers == nil {
		config.MCPServers = defaults.MCPServers
	}

	if config.FeatureFlags == nil {
		config.FeatureFlags = defaults.FeatureFlags
	}

	if config.CustomSettings == nil {
		config.CustomSettings = defaults.CustomSettings
	}

	if config.Aliases.Aliases == nil {
		config.Aliases.Aliases = defaults.Aliases.Aliases
	}

	// Apply the hook defaults when the .taskconfig has no `hooks:` block at all.
	// Without this, a .taskconfig that omits hooks: unmarshalled config.Hooks to
	// the all-false zero value, so `adb config show` reported the Global tier's
	// hooks as disabled — the opposite of the no-file path, which returns
	// DefaultHookConfig() with them enabled (#177). v.IsSet distinguishes an
	// OMITTED block (→ apply defaults) from an explicit `hooks: {enabled: false}`
	// (→ respect the user's choice).
	if !v.IsSet("hooks") {
		config.Hooks = defaults.Hooks
	}

	return &config, nil
}

// GetRepoConfig loads the per-repository configuration from .taskrc
func (cm *ViperConfigManager) GetRepoConfig() (*models.RepoConfig, error) {
	// Check if repo config file exists
	if _, err := os.Stat(cm.repoConfigPath); os.IsNotExist(err) {
		// File doesn't exist, return defaults
		return models.DefaultRepoConfig(), nil
	}

	// Create a new Viper instance for repo config
	v := viper.New()
	v.SetConfigFile(cm.repoConfigPath)
	v.SetConfigType("yaml")

	// Read the config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read repo config: %w", err)
	}

	// Create empty config struct and unmarshal
	var config models.RepoConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal repo config: %w", err)
	}

	// Apply defaults for any empty fields
	defaults := models.DefaultRepoConfig()

	if config.BaseBranch == "" {
		config.BaseBranch = defaults.BaseBranch
	}

	if config.Reviewers == nil {
		config.Reviewers = defaults.Reviewers
	}

	if config.RequiredChecks == nil {
		config.RequiredChecks = defaults.RequiredChecks
	}

	if config.Conventions == nil {
		config.Conventions = defaults.Conventions
	}

	if config.CustomSettings == nil {
		config.CustomSettings = defaults.CustomSettings
	}

	return &config, nil
}

// GetOrgConfig loads the per-organization configuration tier from
// orgs/<orgID>/config.yaml. The org tier is OPTIONAL: an empty orgID or a
// missing file yields (nil, nil), so an absent tier is not an error and leaves
// precedence at the historical Global < Repo behaviour. A present-but-malformed
// file is a real error.
func (cm *ViperConfigManager) GetOrgConfig(orgID string) (*models.OrgConfig, error) {
	if orgID == "" {
		return nil, nil
	}

	path := filepath.Join(cm.orgsDir, orgID, "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read org config: %w", err)
	}

	var config models.OrgConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal org config: %w", err)
	}

	// Stamp the resolved id so callers can report the active org even when the
	// file omits org_id (it usually will — the id is the directory name).
	if config.OrgID == "" {
		config.OrgID = orgID
	}

	return &config, nil
}

// LoadConfig loads the global, org, and repo tiers with proper precedence.
// Precedence (most-specific wins): .taskrc (repo) > orgs/<id>/config.yaml (org)
// > .taskconfig (global) > defaults. The active org is resolved from the ADB_ORG
// env var, falling back to the repo config's `org` field; if neither is set the
// org tier is inactive and behaviour is the historical two-tier merge.
func (cm *ViperConfigManager) LoadConfig() (*models.MergedConfig, error) {
	// Load global config
	globalConfig, err := cm.GetGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Load repo config
	repoConfig, err := cm.GetRepoConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load repo config: %w", err)
	}

	// Resolve the active org for the middle tier: ADB_ORG env wins, else the
	// repo config's `org` field. Empty → no org tier.
	orgID := os.Getenv("ADB_ORG")
	if orgID == "" {
		orgID = repoConfig.Org
	}
	orgConfig, err := cm.GetOrgConfig(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to load org config: %w", err)
	}

	// Create merged config
	merged := models.NewMergedConfigWithOrg(globalConfig, orgConfig, repoConfig)

	return merged, nil
}

// DefaultHookConfig returns a HookConfig with Phase 1 features enabled
// This is a convenience re-export from the models package
func DefaultHookConfig() models.HookConfig {
	return models.DefaultHookConfig()
}
