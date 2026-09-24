package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// customSettingsKey names the free-form settings block that every config tier
// (Global/Org/Repo) carries as a map[string]string.
const customSettingsKey = "custom_settings"

// customSettingsPrefix is how Viper spells one leaf of that block once AllKeys has
// flattened the tier: "custom_settings.<dotted key>". It is also, and this is the
// load-bearing part, how a user can spell a setting at the TOP LEVEL of the file
// (`custom_settings.programs_search_paths: "/x"`) — see mergeRootCustomSettings.
const customSettingsPrefix = customSettingsKey + "."

// customSettingShapes is the tail every "this cannot be a custom setting" warning ends
// with. Phrased in YAML terms, not Go terms: the reader is editing a config file, not
// the decoder.
const customSettingShapes = "values must be text, a number, or true/false"

// normalizeCustomSettings rewrites a tier's `custom_settings:` block into the flat,
// dotted-key shape the models declare, so that one mis-spelled key can never take a
// whole workspace down.
//
// WHY this exists: every tier is parsed by Viper, and Viper treats "." as a
// key-NESTING delimiter. Viper.Unmarshal decodes Viper.AllSettings(), which is
// rebuilt from AllKeys() — a flattened list of dotted leaf keys re-split on "." —
// so a plausible-looking
//
//	custom_settings:
//	  programs.search_paths: "/x"
//
// reaches mapstructure as custom_settings[programs] = map[string]any{…} where a
// string is expected, and Unmarshal fails with
//
//	'custom_settings[programs]' expected type 'string', got unconvertible type 'map[string]interface {}'
//
// Config load happens at app init, so that single key used to kill EVERY adb command
// in the workspace rather than degrading to "that one setting is missing".
//
// Viper.Get returns the block BEFORE that re-splitting (a literal dotted key is still
// one key; a genuinely nested map is still nested), so we flatten it ourselves back to
// the dotted form and write the result back with Viper.Set. The override layer shadows
// the config layer in AllKeys(), so Unmarshal then sees a single immediate
// map[string]string value and never re-splits anything. Flattening to the DOTTED form
// (rather than dropping the key) is also what makes the dotted spelling in
// internal/cli/program.go's programSearchPathsKeys a live fallback instead of dead code.
//
// The block is not the only place a setting can live: a tier may also spell one at the
// TOP LEVEL of the file as `custom_settings.<key>:`, which worked before this function
// existed and had to keep working — see mergeRootCustomSettings, which is also why both
// reads below happen before the Set.
//
// Nothing in here is fatal. An entry that cannot be represented as a string is skipped
// with a warning on stderr — the same non-fatal channel the hook engine and task
// manager use — because one unusable custom setting must not cost you the other
// settings, the other tiers, or the command you were running. source is the config
// file's path, so a warning names the file to edit.
func normalizeCustomSettings(v *viper.Viper, source string) {
	// BOTH reads must happen before the Set below. Set installs custom_settings in
	// Viper's override layer, which shadows the whole `custom_settings` prefix in
	// AllKeys() — so afterwards a top-level `custom_settings.<key>:` entry is
	// unreachable. Reading them here, first, is the entire fix for the regression
	// where hoisting a dotted key to the file root silently lost it.
	block := v.Get(customSettingsKey)
	keys := customSettingKeys(v)

	if block == nil && len(keys) == 0 {
		// No `custom_settings` at all (an empty `custom_settings:` block reads as nil)
		// — nothing to normalize, and deliberately no Set, so the tier's zero
		// value/defaults path is untouched.
		return
	}

	flat, reached, warnings := flattenCustomSettings(block)
	warnings = append(warnings, mergeRootCustomSettings(v, flat, reached, keys)...)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", source, w)
	}

	// LATENT TRAP, documented rather than fixed because nothing depends on it today:
	// after this Set, v.Get("custom_settings.<sub>") returns nil and
	// v.IsSet("custom_settings.<sub>") is false, where both worked before it. The
	// override layer holds ONE immediate map[string]string under "custom_settings", and
	// Viper's shadow logic stops descending there. Unmarshal wants exactly that (it
	// takes the whole map), but a later v.GetString("custom_settings.x") added in this
	// file would silently read nil — take it from the decoded cfg.CustomSettings[x]
	// instead, or move the read above this line.
	v.Set(customSettingsKey, flat)
}

// customSettingKeys returns every custom-setting key Viper can see in this tier, as
// dotted names with the "custom_settings." prefix stripped, sorted.
//
// AllKeys() is the ONLY place a top-level `custom_settings.<key>:` entry shows up
// alongside the block's own leaves — Viper.Get("custom_settings") returns just the block
// — so this is what stops a hoisted dotted key from being dropped. Sorted so warnings
// and collision resolution never depend on Go's map iteration order.
func customSettingKeys(v *viper.Viper) []string {
	var out []string
	for _, key := range v.AllKeys() {
		sub, ok := strings.CutPrefix(key, customSettingsPrefix)
		if !ok || sub == "" {
			continue
		}
		out = append(out, sub)
	}
	sort.Strings(out)
	return out
}

// mergeRootCustomSettings folds Viper's own view of this tier's custom-setting keys over
// the block-derived map, so a TOP-LEVEL dotted entry keeps working:
//
//	custom_settings.programs_search_paths: "/x"
//	custom_settings:
//	  other: keep-me
//
// It used to, by accident of the decode path: Unmarshal decoded AllSettings(), which
// merges every AllKeys() leaf, so `other` AND `programs_search_paths` both landed.
// Flattening only the block would drop the top-level sibling — and drop it in silence.
// That shape matters far more than it looks: it is exactly where a user ends up after
// hitting the original nested-key error and trying the obvious workaround of hoisting
// the key out of the block.
//
// Viper resolves a key that is spelled both ways with a longest-prefix-first lookup, so a
// top-level literal beats the block. That is also the PRE-FIX winner (AllSettings called
// the same Get), so we adopt it — and, because two spellings disagreeing is almost
// certainly a mistake rather than an intent, we say so.
//
// reached distinguishes a leaf the block already accounted for (with a value, or with a
// warning of its own) from one only Viper can see, so nothing is reported twice.
func mergeRootCustomSettings(v *viper.Viper, flat map[string]string, reached map[string]bool, keys []string) []string {
	var warnings []string
	for _, sub := range keys {
		raw := v.Get(customSettingsPrefix + sub)
		value, convertible := customSettingValue(raw)
		prev, kept := flat[sub]

		switch {
		case !convertible && kept:
			// A top-level literal shadows a usable block entry with an unusable
			// value. Viper's lookup prefers the literal, so pre-fix the whole tier
			// failed to decode; keep the block's usable value instead of losing the
			// setting, and name the entry that is being ignored.
			warnings = append(warnings, fmt.Sprintf(
				"ignoring the top-level %q (%s): using %q from the custom_settings block instead",
				customSettingsPrefix+sub, describeCustomSettingValue(raw), prev))
		case !convertible && reached[sub]:
			// The block's own pass already warned about this exact leaf.
		case !convertible:
			warnings = append(warnings, customSettingSkipWarning(sub, raw))
		case kept && prev != value:
			warnings = append(warnings, fmt.Sprintf(
				"custom setting %q is set both at the top of the file and inside the custom_settings block; the top-level %q wins",
				sub, value))
			flat[sub] = value
		default:
			flat[sub] = value
		}
	}
	return warnings
}

// flattenCustomSettings flattens a possibly-nested custom_settings value into the
// dotted keys a map[string]string can hold. It returns:
//
//	flat     — the usable settings, keyed by their dotted names
//	reached  — every dotted key the block bottomed out at, INCLUDING the ones flat
//	           omits because they were skipped with a warning; mergeRootCustomSettings
//	           needs this to tell "already reported" from "nobody has reported this yet"
//	warnings — one per skipped entry, so a skipped setting never disappears silently
func flattenCustomSettings(raw any) (flat map[string]string, reached map[string]bool, warnings []string) {
	flat = make(map[string]string)
	reached = make(map[string]bool)

	if raw == nil {
		// No `custom_settings:` block. The tier may still carry top-level
		// `custom_settings.<key>:` entries, which the caller merges in — so this is
		// an empty result, not a warning.
		return flat, reached, nil
	}

	m, ok := asStringMap(raw)
	if !ok {
		// `custom_settings: "oops"` (or a list) is the same class of footgun one level
		// up: fatal before, ignored-with-a-warning now.
		return flat, reached, []string{fmt.Sprintf(
			"ignoring custom_settings: it must be a block of key/value pairs, but this file sets it to %s",
			describeCustomSettingValue(raw))}
	}

	flattenCustomSettingsInto(m, "", flat, reached, &warnings)
	return flat, reached, warnings
}

// flattenCustomSettingsInto walks one level of the block, recursing into nested maps
// and joining the path with "." — the same delimiter Viper split on, so the round trip
// is lossless: `programs: {search_paths: "/x"}` comes back out as the single entry
// `programs.search_paths` → "/x", at any depth.
//
// Within a level, nested maps are flattened BEFORE scalar leaves so that a literal
// dotted key wins over the same key synthesized from nesting (the more explicit
// spelling wins, mirroring how the canonical `programs_search_paths` outranks the
// dotted fallback in internal/cli/program.go). Keys are visited in sorted order so
// warnings and collision resolution never depend on Go's map iteration order.
func flattenCustomSettingsInto(m map[string]any, prefix string, out map[string]string, reached map[string]bool, warnings *[]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	join := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + "." + k
	}

	// Pass 1: nested maps.
	for _, k := range keys {
		nested, ok := asStringMap(m[k])
		if !ok {
			continue
		}
		if len(nested) == 0 {
			// `a: {}` bottoms out at nothing — exactly like `a:` (a nil leaf), which
			// warns in pass 2. The two spellings of the same mistake must not differ
			// in whether they leave a trace, so warn here too rather than dropping
			// the key in silence. Warning is the direction chosen (rather than
			// silencing nil) because the whole point of this function is that a
			// dropped setting is never invisible.
			full := join(k)
			reached[full] = true
			*warnings = append(*warnings, fmt.Sprintf(
				"skipping custom setting %q: an empty block cannot be a custom setting; %s",
				full, customSettingShapes))
			continue
		}
		flattenCustomSettingsInto(nested, join(k), out, reached, warnings)
	}

	// Pass 2: scalar leaves at this level, which overwrite anything pass 1 produced.
	for _, k := range keys {
		if _, isMap := asStringMap(m[k]); isMap {
			continue
		}
		full := join(k)
		reached[full] = true
		value, ok := customSettingValue(m[k])
		if !ok {
			*warnings = append(*warnings, customSettingSkipWarning(full, m[k]))
			continue
		}
		if _, collides := out[full]; collides {
			*warnings = append(*warnings, fmt.Sprintf(
				"custom setting %q is set both literally and by nesting inside custom_settings; the literal value wins", full))
		}
		out[full] = value
	}
}

// asStringMap reports whether v is a mapping and returns it keyed by string. Viper
// normalizes YAML maps to map[string]any on read, but map[any]any is accepted too so a
// hand-built or differently-parsed value cannot make this panic or mis-classify.
func asStringMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	}
	return nil, false
}

// customSettingValue stringifies a scalar leaf, ok=false meaning "not representable as
// a custom setting" (a list, or anything else with no single-value spelling).
//
// The conversions deliberately match what mapstructure's weakly-typed decode already
// does for a FLAT non-string leaf today — notably bool → "1"/"0", not "true"/"false" —
// so flattening a nested key never changes the spelling of a value that a flat key
// would have produced. A nil leaf (`some_key:` with nothing after it) is skipped
// rather than stored as "", which is what Viper's own AllSettings does with a nil.
func customSettingValue(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		if t {
			return "1", true
		}
		return "0", true
	case int:
		return strconv.Itoa(t), true
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", t), true
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", t), true
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	}
	return "", false
}

// customSettingSkipWarning is the single wording for "this entry cannot be a custom
// setting", shared by the block pass and the top-level pass so the two can never drift
// into describing the same problem two different ways. The caller prefixes the config
// file's path, so the reader always knows which file to open.
func customSettingSkipWarning(key string, raw any) string {
	advice := customSettingShapes
	if hint := customSettingHint(raw); hint != "" {
		advice = hint
	}
	return fmt.Sprintf("skipping custom setting %q: %s cannot be a custom setting; %s",
		key, describeCustomSettingValue(raw), advice)
}

// describeCustomSettingValue names a value the way the person who wrote the YAML would
// recognize it.
//
// Deliberately NOT the Go type. `%T` leaked `[]interface {}`, `time.Time` and `int` into
// warnings aimed at someone deciding which LINE OF THEIR CONFIG FILE to edit — and none
// of those spellings appear anywhere in that file, so they carried no information the
// reader could act on.
func describeCustomSettingValue(v any) string {
	switch v.(type) {
	case nil:
		return "an empty value"
	case string:
		return "a single text value"
	case bool:
		return "a true/false value"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64:
		return "a number"
	case time.Time:
		// An unquoted 2026-01-01 is a YAML *timestamp*, not a string, so this is what
		// a plain date in a config file actually arrives as.
		return "a date"
	case []any, []string:
		return "a list"
	case map[string]any, map[any]any:
		return "a nested block"
	}
	return "a value adb cannot read as text"
}

// customSettingHint returns the concrete fix for the value kinds that have one, or ""
// when the generic list of accepted shapes is the only useful advice. It exists so the
// date case can say what to actually DO — quote it — instead of restating the rule the
// reader has already broken.
func customSettingHint(v any) string {
	if _, isTime := v.(time.Time); isTime {
		return `wrap it in quotes to keep it text, e.g. "2026-01-01"`
	}
	return ""
}

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
	// pinnedOrg, when non-empty, is the active org for the middle tier and
	// suppresses both $ADB_ORG and the repo config's `org:` field.
	// ignoreOrgEnv suppresses only $ADB_ORG, leaving `org:` to select the tier.
	// Both are set via NewViperConfigManagerWithOptions; see ConfigManagerOptions
	// for why the two ambient sources are treated differently.
	pinnedOrg    string
	ignoreOrgEnv bool
}

// NewViperConfigManager creates a new configuration manager
// If paths are empty, defaults are used:
// - globalConfigPath: ~/.taskconfig
// - repoConfigPath: ./.taskrc
// The org tier root is derived as <dir of repoConfigPath>/orgs.
//
// The active org is resolved at LoadConfig time from $ADB_ORG, then the repo
// config's `org:` field. Use NewViperConfigManagerWithOptions to pin it, or to
// ignore the environment, and take ambient state out of the picture.
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

// ConfigManagerOptions controls how the ACTIVE ORG for the middle tier is chosen.
//
// It exists to separate two things that look alike but are not: the org id in a
// workspace's own `.taskrc` is workspace DATA, while $ADB_ORG is process-wide
// AMBIENT state. A caller that wants to be hermetic needs to drop the second
// without losing the first.
type ConfigManagerOptions struct {
	// Org pins the active org outright, suppressing BOTH $ADB_ORG and the repo
	// config's `org:` field. Empty means "not pinned".
	Org string
	// IgnoreOrgEnv suppresses $ADB_ORG only; the repo config's `org:` field still
	// selects the tier. This is what an isolated caller wants: the workspace it was
	// handed still describes itself, but the surrounding process cannot vote.
	IgnoreOrgEnv bool
}

// NewViperConfigManagerWithOptions is NewViperConfigManager plus explicit control
// over org resolution (see ConfigManagerOptions).
//
// Pinning or ignoring the env here, rather than having the caller set $ADB_ORG, is
// also what lets a test run in parallel: t.Setenv is incompatible with t.Parallel
// by design, so env-based isolation forces hermetic tests to be serial.
func NewViperConfigManagerWithOptions(globalConfigPath, repoConfigPath string, opts ConfigManagerOptions) *ViperConfigManager {
	cm := NewViperConfigManager(globalConfigPath, repoConfigPath)
	cm.pinnedOrg = opts.Org
	cm.ignoreOrgEnv = opts.IgnoreOrgEnv
	return cm
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

	// Flatten any Viper-nested custom_settings before decoding — see
	// normalizeCustomSettings for why a dotted key would otherwise be fatal here.
	normalizeCustomSettings(v, cm.globalConfigPath)

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

	// Flatten any Viper-nested custom_settings before decoding — see
	// normalizeCustomSettings for why a dotted key would otherwise be fatal here.
	normalizeCustomSettings(v, cm.repoConfigPath)

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
	// gosec flags orgID as tainted input reaching a file path. There is no trust
	// boundary to cross here: orgID comes from the invoking user's own $ADB_ORG,
	// their own `.taskrc`, or an explicit AppOptions.Org, and adb runs with that
	// same user's privileges — a traversing value could only name a config file
	// they can already read (and only a *read*; nothing here writes). The tier is
	// also fail-open by contract, so a bogus id resolves to "no org tier".
	//nolint:gosec // G703: orgID is the invoking user's own config/env value, read-only, no privilege boundary
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read org config: %w", err)
	}

	// Flatten any Viper-nested custom_settings before decoding — see
	// normalizeCustomSettings for why a dotted key would otherwise be fatal here.
	normalizeCustomSettings(v, path)

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

	// Resolve the active org for the middle tier, most explicit first:
	//   1. an org pinned on the manager (suppresses both ambient sources);
	//   2. $ADB_ORG, unless the manager was told to ignore the environment;
	//   3. the repo config's `org:` field — workspace data, so an isolated caller
	//      still honours it.
	// Empty → no org tier.
	orgID := cm.pinnedOrg
	if orgID == "" {
		if !cm.ignoreOrgEnv {
			orgID = os.Getenv("ADB_ORG")
		}
		if orgID == "" {
			orgID = repoConfig.Org
		}
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
