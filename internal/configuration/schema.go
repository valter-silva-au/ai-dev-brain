package configuration

import (
	"errors"
	"fmt"
	"io"
	pathpkg "path"
	"regexp"
	"sort"
	"strings"
	"time"

	profiledomain "github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = "aidb.config/v1"

type MergeStrategy string

const (
	MergeReplace   MergeStrategy = "replace"
	MergeDeepMap   MergeStrategy = "deep_map"
	MergeSetUnion  MergeStrategy = "set_union"
	MergeAppend    MergeStrategy = "append"
	MergeKeyedList MergeStrategy = "keyed_list"
)

type FieldSpec struct {
	Path             string        `json:"path" yaml:"path"`
	Strategy         MergeStrategy `json:"strategy" yaml:"strategy"`
	EnvironmentNames []string      `json:"environment_names" yaml:"environment_names"`
}

type Settings struct {
	Profile     *string           `yaml:"profile,omitempty" json:"profile,omitempty"`
	Policy      PolicySettings    `yaml:"policy,omitempty" json:"policy,omitempty"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	SearchPaths []string          `yaml:"search_paths,omitempty" json:"search_paths,omitempty"`
	Templates   []TemplateSetting `yaml:"templates,omitempty" json:"templates,omitempty"`
}

type PolicySettings struct {
	Conformance *string           `yaml:"conformance,omitempty" json:"conformance,omitempty"`
	Freshness   FreshnessSettings `yaml:"freshness,omitempty" json:"freshness,omitempty"`
}

type FreshnessSettings struct {
	Enabled *bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	TTL     *string `yaml:"ttl,omitempty" json:"ttl,omitempty"`
}

type TemplateSetting struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	Source  string `yaml:"source,omitempty" json:"source,omitempty"`
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

type SecretReference struct {
	Provider string `yaml:"provider" json:"provider"`
	Key      string `yaml:"key" json:"key"`
	Inherit  bool   `yaml:"inherit,omitempty" json:"inherit,omitempty"`
}

type EnvironmentFile struct {
	Path      string   `yaml:"path" json:"path"`
	Allow     []string `yaml:"allow" json:"allow"`
	Inherit   bool     `yaml:"inherit,omitempty" json:"inherit,omitempty"`
	GitIgnore bool     `yaml:"gitignored,omitempty" json:"gitignored,omitempty"`
}

type MachineBinding struct {
	Path       string `yaml:"path,omitempty" json:"path,omitempty"`
	Executable string `yaml:"executable,omitempty" json:"executable,omitempty"`
}

type Document struct {
	SchemaVersion    string                     `yaml:"schema_version" json:"schema_version"`
	Settings         Settings                   `yaml:"settings,omitempty" json:"settings,omitempty"`
	Plugins          map[string]any             `yaml:"plugins,omitempty" json:"plugins,omitempty"`
	Locks            map[string]string          `yaml:"locks,omitempty" json:"locks,omitempty"`
	Unlocks          []string                   `yaml:"unlocks,omitempty" json:"unlocks,omitempty"`
	Reset            []string                   `yaml:"reset,omitempty" json:"reset,omitempty"`
	Delete           []string                   `yaml:"delete,omitempty" json:"delete,omitempty"`
	SecretReferences map[string]SecretReference `yaml:"secret_references,omitempty" json:"secret_references,omitempty"`
	EnvironmentFiles []EnvironmentFile          `yaml:"environment_files,omitempty" json:"environment_files,omitempty"`
	MachineBindings  map[string]MachineBinding  `yaml:"machine_bindings,omitempty" json:"machine_bindings,omitempty"`
}

type Config struct {
	Settings         Settings                   `json:"settings" yaml:"settings"`
	Plugins          map[string]any             `json:"plugins" yaml:"plugins"`
	SecretReferences map[string]SecretReference `json:"secret_references" yaml:"secret_references"`
	EnvironmentFiles []EnvironmentFile          `json:"environment_files" yaml:"environment_files"`
	MachineBindings  map[string]MachineBinding  `json:"machine_bindings" yaml:"machine_bindings"`
}

var pluginNamespacePattern = regexp.MustCompile(
	`^[a-z0-9][a-z0-9-]*(?:\.[a-z0-9][a-z0-9-]*)+$`,
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func DefaultDocument() Document {
	return Document{
		SchemaVersion: SchemaVersion,
		Settings: Settings{
			Profile: stringValue(
				profiledomain.ProfileReference{
					ID:      profiledomain.BuiltinTicketProfileID,
					Version: profiledomain.BuiltinTicketProfileVersion,
				}.String(),
			),
			Policy: PolicySettings{
				Conformance: stringValue("warn"),
				Freshness: FreshnessSettings{
					Enabled: boolValue(true),
					TTL:     stringValue("24h"),
				},
			},
			Tags:        []string{"core"},
			SearchPaths: []string{"docs"},
			Templates: []TemplateSetting{{
				ID:      "ticket",
				Version: "v1",
				Source:  "builtin",
				Enabled: boolValue(true),
			}},
		},
		Plugins:          map[string]any{},
		Locks:            map[string]string{},
		Unlocks:          []string{},
		Reset:            []string{},
		Delete:           []string{},
		SecretReferences: map[string]SecretReference{},
		EnvironmentFiles: []EnvironmentFile{},
		MachineBindings:  map[string]MachineBinding{},
	}
}

func FieldSpecs() []FieldSpec {
	return []FieldSpec{
		{
			Path:             "settings.profile",
			Strategy:         MergeReplace,
			EnvironmentNames: []string{"ADB_PROFILE"},
		},
		{Path: "settings.policy", Strategy: MergeDeepMap},
		{
			Path:             "settings.policy.conformance",
			Strategy:         MergeReplace,
			EnvironmentNames: []string{"ADB_CONFORMANCE"},
		},
		{Path: "settings.policy.freshness", Strategy: MergeDeepMap},
		{
			Path:             "settings.policy.freshness.enabled",
			Strategy:         MergeReplace,
			EnvironmentNames: []string{"ADB_FRESHNESS_ENABLED"},
		},
		{
			Path:             "settings.policy.freshness.ttl",
			Strategy:         MergeReplace,
			EnvironmentNames: []string{"ADB_FRESHNESS_TTL"},
		},
		{Path: "settings.tags", Strategy: MergeSetUnion},
		{Path: "settings.search_paths", Strategy: MergeAppend},
		{Path: "settings.templates", Strategy: MergeKeyedList},
		{Path: "plugins", Strategy: MergeDeepMap},
		{Path: "secret_references", Strategy: MergeKeyedList},
		{Path: "environment_files", Strategy: MergeKeyedList},
		{Path: "machine_bindings", Strategy: MergeKeyedList},
	}
}

func DecodeDocument(reader io.Reader) (Document, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, errors.New(
			"configuration syntax or field types are invalid",
		)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New(
				"configuration contains multiple YAML documents",
			)
		}
		return Document{}, errors.New(
			"configuration trailer is invalid",
		)
	}
	normalizeDocument(&document)
	if err := validateDocument(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func normalizeDocument(document *Document) {
	if document.Plugins == nil {
		document.Plugins = map[string]any{}
	}
	if document.Locks == nil {
		document.Locks = map[string]string{}
	}
	if document.Unlocks == nil {
		document.Unlocks = []string{}
	}
	if document.Reset == nil {
		document.Reset = []string{}
	}
	if document.Delete == nil {
		document.Delete = []string{}
	}
	if document.SecretReferences == nil {
		document.SecretReferences = map[string]SecretReference{}
	}
	if document.EnvironmentFiles == nil {
		document.EnvironmentFiles = []EnvironmentFile{}
	}
	for index := range document.EnvironmentFiles {
		if strings.TrimSpace(document.EnvironmentFiles[index].Path) == "" {
			continue
		}
		normalized := strings.ReplaceAll(
			document.EnvironmentFiles[index].Path,
			"\\",
			"/",
		)
		document.EnvironmentFiles[index].Path = pathpkg.Clean(normalized)
	}
	if document.MachineBindings == nil {
		document.MachineBindings = map[string]MachineBinding{}
	}
}

func validateDocument(document Document) error {
	if document.SchemaVersion != SchemaVersion {
		return errors.New("unsupported configuration schema")
	}
	if document.Settings.Profile != nil &&
		strings.TrimSpace(*document.Settings.Profile) == "" {
		return errors.New("configuration profile cannot be empty")
	}
	if document.Settings.Profile != nil &&
		secretLikeString("settings.profile", *document.Settings.Profile) {
		return errors.New(
			"configuration profile contains a probable literal secret",
		)
	}
	if document.Settings.Profile != nil {
		if _, err := profiledomain.ParseProfileReference(
			*document.Settings.Profile,
		); err != nil {
			return errors.New(
				"configuration profile must select an exact id@version",
			)
		}
	}
	if document.Settings.Policy.Conformance != nil {
		switch *document.Settings.Policy.Conformance {
		case "off", "observe", "warn", "block", "repair-safe":
		default:
			return errors.New("unsupported conformance policy")
		}
	}
	if ttl := document.Settings.Policy.Freshness.TTL; ttl != nil {
		duration, err := time.ParseDuration(*ttl)
		if err != nil || duration < 0 {
			return errors.New("invalid freshness ttl")
		}
	}
	if err := validateUniqueText("tag", document.Settings.Tags); err != nil {
		return err
	}
	for _, tag := range document.Settings.Tags {
		if secretLikeString("settings.tags", tag) {
			return errors.New("configuration tag contains a probable literal secret")
		}
	}
	if err := validateUniqueText(
		"search path",
		document.Settings.SearchPaths,
	); err != nil {
		return err
	}
	for _, searchPath := range document.Settings.SearchPaths {
		if secretLikeString("settings.search_paths", searchPath) {
			return errors.New(
				"configuration search path contains a probable literal secret",
			)
		}
	}
	templateIDs := make(map[string]struct{}, len(document.Settings.Templates))
	for _, item := range document.Settings.Templates {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("template id is required")
		}
		if secretLikeString("settings.templates.id", item.ID) ||
			secretLikeString("settings.templates.version", item.Version) ||
			secretLikeString("settings.templates.source", item.Source) {
			return errors.New(
				"template configuration contains a probable literal secret",
			)
		}
		if _, ok := templateIDs[item.ID]; ok {
			return errors.New("duplicate template id")
		}
		templateIDs[item.ID] = struct{}{}
	}
	pluginNamespaces := sortedKeys(document.Plugins)
	for _, namespace := range pluginNamespaces {
		if !pluginNamespacePattern.MatchString(namespace) {
			return errors.New(
				"plugin namespace must contain dot-separated lowercase names",
			)
		}
		if err := validatePluginValue(
			document.Plugins[namespace],
			"plugins."+namespace,
		); err != nil {
			return err
		}
	}
	lockPaths := sortedKeys(document.Locks)
	for index, path := range lockPaths {
		if !knownOperationPath(path) {
			return errors.New("unsupported lock path")
		}
		if strings.TrimSpace(document.Locks[path]) == "" {
			return errors.New("lock path requires a reason")
		}
		if secretLikeString("locks.reason", document.Locks[path]) {
			return errors.New("lock reason contains a probable literal secret")
		}
		for _, other := range lockPaths[index+1:] {
			if relatedFieldPaths(path, other) {
				return errors.New(
					"configuration contains overlapping lock paths",
				)
			}
		}
	}
	for _, operation := range []struct {
		name  string
		paths []string
	}{
		{name: "unlock", paths: document.Unlocks},
		{name: "reset", paths: document.Reset},
		{name: "delete", paths: document.Delete},
	} {
		if err := validateUniqueText(operation.name, operation.paths); err != nil {
			return err
		}
		for _, path := range operation.paths {
			if !knownOperationPath(path) {
				return fmt.Errorf("unsupported %s path", operation.name)
			}
		}
	}
	for _, name := range sortedKeys(document.SecretReferences) {
		reference := document.SecretReferences[name]
		if strings.TrimSpace(name) == "" ||
			strings.TrimSpace(reference.Provider) == "" ||
			strings.TrimSpace(reference.Key) == "" {
			return errors.New("secret reference requires provider and key")
		}
		if reference.Provider == "environment" &&
			!environmentNamePattern.MatchString(reference.Key) {
			return errors.New(
				"environment secret reference key must be a variable name",
			)
		}
	}
	environmentPaths := make(
		map[string]struct{},
		len(document.EnvironmentFiles),
	)
	for _, environmentFile := range document.EnvironmentFiles {
		if strings.TrimSpace(environmentFile.Path) == "" {
			return errors.New("environment file path is required")
		}
		if environmentFile.Path == "." {
			return errors.New("environment file path must name a file")
		}
		if len(environmentFile.Allow) == 0 {
			return errors.New(
				"environment file requires an explicit variable allowlist",
			)
		}
		if _, ok := environmentPaths[environmentFile.Path]; ok {
			return errors.New("duplicate environment file path")
		}
		environmentPaths[environmentFile.Path] = struct{}{}
		if err := validateUniqueText(
			"environment allowlist entry",
			environmentFile.Allow,
		); err != nil {
			return err
		}
		for _, name := range environmentFile.Allow {
			if !environmentNamePattern.MatchString(name) {
				return errors.New(
					"environment allowlist entries must be variable names",
				)
			}
		}
	}
	for _, name := range sortedKeys(document.MachineBindings) {
		binding := document.MachineBindings[name]
		if strings.TrimSpace(name) == "" ||
			(binding.Path == "" && binding.Executable == "") {
			return errors.New("machine binding requires path or executable")
		}
	}
	return nil
}

func validatePluginValue(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			child := typed[key]
			childPath := path + "." + key
			if key == "secret_reference" {
				reference, ok := child.(string)
				if !ok || strings.TrimSpace(reference) == "" {
					return errors.New(
						"plugin secret_reference must name a reference",
					)
				}
				continue
			}
			if secretShapedKey(key) {
				return errors.New(
					"plugin secret-shaped fields must use secret_reference",
				)
			}
			if err := validatePluginValue(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := validatePluginValue(
				child,
				fmt.Sprintf("%s[%d]", path, index),
			); err != nil {
				return err
			}
		}
	case string:
		if secretLikeString(path, typed) {
			return errors.New(
				"plugin configuration contains a probable literal secret",
			)
		}
	case bool,
		int,
		int8,
		int16,
		int32,
		int64,
		uint,
		uint8,
		uint16,
		uint32,
		uint64,
		float32,
		float64:
	case nil:
		return errors.New("plugin configuration cannot contain null values")
	default:
		return errors.New("plugin configuration uses an unsupported value type")
	}
	return nil
}

func secretShapedKey(key string) bool {
	normalized := strings.ToLower(key)
	segments := strings.FieldsFunc(normalized, func(value rune) bool {
		return value == '-' || value == '_' || value == '.'
	})
	for index, segment := range segments {
		switch segment {
		case "secret", "password", "passwd", "credential", "auth":
			return true
		case "token":
			if index == len(segments)-1 {
				return true
			}
		}
	}
	compacted := strings.NewReplacer("-", "", "_", "", ".", "").Replace(
		normalized,
	)
	for _, marker := range []string{
		"secret",
		"password",
		"passwd",
		"token",
		"credential",
		"apikey",
		"privatekey",
	} {
		if compacted == marker || strings.HasSuffix(compacted, marker) {
			return true
		}
	}
	return false
}

func secretLikeString(path string, value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"-----begin ",
		"top-secret",
		"password=",
		"secret=",
		"token=",
		"github_pat_",
		"ghp_",
		"xoxb-",
		"xoxp-",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if strings.HasPrefix(value, "AKIA") ||
		strings.HasPrefix(value, "ASIA") ||
		strings.HasPrefix(value, "sk-") {
		return true
	}
	return secretShapedKey(lastPathSegment(path)) ||
		looksLikeOpaqueSecret(value)
}

func looksLikeOpaqueSecret(value string) bool {
	if len(value) < 32 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	var lower bool
	var upper bool
	var digit bool
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
			lower = true
		case character >= 'A' && character <= 'Z':
			upper = true
		case character >= '0' && character <= '9':
			digit = true
		}
	}
	return lower && upper && digit
}

func lastPathSegment(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 {
		return path[index+1:]
	}
	return path
}

func validateUniqueText(kind string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", kind)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("duplicate %s", kind)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringValue(value string) *string {
	return &value
}

func boolValue(value bool) *bool {
	return &value
}
