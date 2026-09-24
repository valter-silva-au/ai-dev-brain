package configuration

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	profiledomain "github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

type LayerResolveRequest struct {
	Layers      []Layer    `json:"layers" yaml:"layers"`
	Environment []Override `json:"environment" yaml:"environment"`
	Flags       []Override `json:"flags" yaml:"flags"`
	Target      ScopeKind  `json:"target,omitempty" yaml:"target,omitempty"`
}

type Contribution struct {
	Path      string `json:"path" yaml:"path"`
	Source    Source `json:"source" yaml:"source"`
	Operation string `json:"operation" yaml:"operation"`
	Applied   bool   `json:"applied" yaml:"applied"`
	Reason    string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type Finding struct {
	ID          string   `json:"id" yaml:"id"`
	Severity    string   `json:"severity" yaml:"severity"`
	Summary     string   `json:"summary" yaml:"summary"`
	Path        string   `json:"path,omitempty" yaml:"path,omitempty"`
	Source      *Source  `json:"source,omitempty" yaml:"source,omitempty"`
	Evidence    []string `json:"evidence" yaml:"evidence"`
	Remediation string   `json:"remediation" yaml:"remediation"`
}

type Lock struct {
	Path      string `json:"path" yaml:"path"`
	Source    Source `json:"source" yaml:"source"`
	Reason    string `json:"reason" yaml:"reason"`
	Authority int    `json:"authority" yaml:"authority"`
}

type Resolution struct {
	Config     Config                    `json:"config" yaml:"config"`
	Provenance map[string][]Contribution `json:"provenance" yaml:"provenance"`
	Findings   []Finding                 `json:"findings" yaml:"findings"`
	Sources    []Source                  `json:"sources" yaml:"sources"`
	Locks      map[string]Lock           `json:"locks" yaml:"locks"`
}

func Resolve(request LayerResolveRequest) (Resolution, error) {
	layers := append([]Layer(nil), request.Layers...)
	seenScopes := make(map[ScopeKind]struct{}, len(layers))
	for _, layer := range layers {
		if err := validateLayerSource(layer.Source); err != nil {
			return Resolution{}, err
		}
		if _, exists := seenScopes[layer.Source.Kind]; exists {
			return Resolution{}, fmt.Errorf(
				"duplicate %s configuration layer",
				layer.Source.Kind,
			)
		}
		seenScopes[layer.Source.Kind] = struct{}{}
	}
	sort.SliceStable(layers, func(left int, right int) bool {
		return scopePrecedence(layers[left].Source.Kind) <
			scopePrecedence(layers[right].Source.Kind)
	})
	target, err := resolveTargetScope(request.Target, layers)
	if err != nil {
		return Resolution{}, err
	}

	result := Resolution{
		Config: Config{
			Plugins:          map[string]any{},
			SecretReferences: map[string]SecretReference{},
			EnvironmentFiles: []EnvironmentFile{},
			MachineBindings:  map[string]MachineBinding{},
		},
		Provenance: map[string][]Contribution{},
		Findings:   []Finding{},
		Sources:    []Source{},
		Locks:      map[string]Lock{},
	}
	defaults := configFromDocument(DefaultDocument())
	allLayers := make([]Layer, 0, len(layers)+3)
	allLayers = append(allLayers, Layer{
		Source: Source{
			Kind: ScopeBuiltin,
			Name: "built-in defaults",
		},
		Document: DefaultDocument(),
	})
	allLayers = append(allLayers, layers...)
	if len(request.Environment) > 0 {
		document, err := overrideDocument(request.Environment, true)
		if err != nil {
			return Resolution{}, fmt.Errorf(
				"resolve environment configuration: %w",
				err,
			)
		}
		allLayers = append(allLayers, Layer{
			Source: Source{
				Kind: ScopeEnvironment,
				Name: "allowlisted environment",
			},
			Document: document,
		})
	}
	if len(request.Flags) > 0 {
		document, err := overrideDocument(request.Flags, false)
		if err != nil {
			return Resolution{}, fmt.Errorf(
				"resolve request configuration: %w",
				err,
			)
		}
		allLayers = append(allLayers, Layer{
			Source: Source{
				Kind: ScopeRequest,
				Name: "explicit request flags",
			},
			Document: document,
		})
	}

	for _, layer := range allLayers {
		if err := validateDocument(layer.Document); err != nil {
			return Resolution{}, fmt.Errorf(
				"validate %s configuration: %w",
				layer.Source.Kind,
				err,
			)
		}
		if err := validateSourceDocument(layer.Source, layer.Document); err != nil {
			return Resolution{}, err
		}
		result.Sources = append(result.Sources, layer.Source)
		if err := applyLayer(&result, defaults, layer, target); err != nil {
			return Resolution{}, fmt.Errorf(
				"apply %s configuration: %w",
				layer.Source.Kind,
				err,
			)
		}
	}
	if err := validateResolvedConfig(result.Config); err != nil {
		return Resolution{}, err
	}
	recordMissingSecretReferences(&result)
	return result, nil
}

func validateResolvedConfig(config Config) error {
	if config.Settings.Profile != nil {
		if _, err := profiledomain.ParseProfileReference(
			*config.Settings.Profile,
		); err != nil {
			return fmt.Errorf(
				"resolved profile must select an exact id@version",
			)
		}
	}
	for _, template := range config.Settings.Templates {
		if strings.TrimSpace(template.Version) == "" {
			return fmt.Errorf(
				"resolved template %q requires an exact version",
				template.ID,
			)
		}
		switch template.Source {
		case "builtin",
			"user_global",
			"workspace",
			"organization",
			"host",
			"owner",
			"repository",
			"ticket":
		default:
			return fmt.Errorf(
				"resolved template %q has an unrecognized source scope",
				template.ID,
			)
		}
	}
	return nil
}

func resolveTargetScope(requested ScopeKind, layers []Layer) (ScopeKind, error) {
	target := requested
	if target == "" {
		target = ScopeBuiltin
		for _, layer := range layers {
			if scopePrecedence(layer.Source.Kind) > scopePrecedence(target) {
				target = layer.Source.Kind
			}
		}
	}
	if target != ScopeBuiltin && !semanticScope(target) {
		return "", fmt.Errorf("unsupported configuration target scope %q", target)
	}
	for _, layer := range layers {
		if scopePrecedence(layer.Source.Kind) > scopePrecedence(target) {
			return "", fmt.Errorf(
				"%s configuration is below target scope %s",
				layer.Source.Kind,
				target,
			)
		}
	}
	return target, nil
}

// semanticScope reports whether a scope can contribute a configuration layer of
// its own. Builtin defaults, process environment, and request flags are resolved
// separately and are listed explicitly so that adding a scope to the enum trips
// the exhaustive linter here instead of silently defaulting to "not semantic".
func semanticScope(kind ScopeKind) bool {
	switch kind {
	case ScopeUserGlobal,
		ScopeWorkspace,
		ScopeOrganization,
		ScopeHost,
		ScopeOwner,
		ScopeRepository,
		ScopeTicket:
		return true
	case ScopeBuiltin,
		ScopeEnvironment,
		ScopeRequest:
		return false
	}
	return false
}

func validateLayerSource(source Source) error {
	if strings.TrimSpace(source.Name) == "" {
		return fmt.Errorf("%s configuration source name is required", source.Kind)
	}
	switch source.Kind {
	case ScopeUserGlobal:
		if source.Portable {
			return fmt.Errorf(
				"%s configuration cannot be marked portable",
				source.Kind,
			)
		}
		return nil
	case ScopeWorkspace,
		ScopeOrganization,
		ScopeHost,
		ScopeOwner,
		ScopeRepository,
		ScopeTicket:
		if !source.Portable {
			return fmt.Errorf(
				"%s configuration must be marked portable",
				source.Kind,
			)
		}
		return nil
	case ScopeBuiltin,
		ScopeEnvironment,
		ScopeRequest:
		// Not layer scopes: they are resolved outside the layer stack, so a
		// document claiming one is rejected exactly like an unknown scope.
	}
	return fmt.Errorf(
		"unsupported configuration layer scope %q",
		source.Kind,
	)
}

func applyLayer(
	result *Resolution,
	defaults Config,
	layer Layer,
	target ScopeKind,
) error {
	for _, path := range layer.Document.Unlocks {
		unlockPath(result, layer.Source, path)
	}
	for _, path := range layer.Document.Reset {
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(result, layer.Source, path, "reset", lock)
			continue
		}
		if err := resetPath(&result.Config, defaults, path); err != nil {
			return err
		}
		addContribution(result, layer.Source, path, "reset")
	}
	for _, path := range layer.Document.Delete {
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(result, layer.Source, path, "delete", lock)
			continue
		}
		if err := deletePath(&result.Config, path); err != nil {
			return err
		}
		addContribution(result, layer.Source, path, "delete")
	}

	settings := layer.Document.Settings
	if settings.Profile != nil {
		const path = "settings.profile"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeReplace),
				lock,
			)
		} else {
			result.Config.Settings.Profile = stringValue(*settings.Profile)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeReplace),
			)
		}
	}
	if settings.Policy.Conformance != nil {
		const path = "settings.policy.conformance"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeReplace),
				lock,
			)
		} else {
			result.Config.Settings.Policy.Conformance = stringValue(
				*settings.Policy.Conformance,
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeReplace),
			)
		}
	}
	if settings.Policy.Freshness.Enabled != nil {
		const path = "settings.policy.freshness.enabled"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeReplace),
				lock,
			)
		} else {
			result.Config.Settings.Policy.Freshness.Enabled = boolValue(
				*settings.Policy.Freshness.Enabled,
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeReplace),
			)
		}
	}
	if settings.Policy.Freshness.TTL != nil {
		const path = "settings.policy.freshness.ttl"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeReplace),
				lock,
			)
		} else {
			result.Config.Settings.Policy.Freshness.TTL = stringValue(
				*settings.Policy.Freshness.TTL,
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeReplace),
			)
		}
	}
	if settings.Tags != nil {
		const path = "settings.tags"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeSetUnion),
				lock,
			)
		} else {
			result.Config.Settings.Tags = setUnion(
				result.Config.Settings.Tags,
				settings.Tags,
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeSetUnion),
			)
		}
	}
	if settings.SearchPaths != nil {
		const path = "settings.search_paths"
		if lock, blocked := blockingLock(result, path); blocked {
			recordLockedOverride(
				result,
				layer.Source,
				path,
				string(MergeAppend),
				lock,
			)
		} else {
			result.Config.Settings.SearchPaths = append(
				result.Config.Settings.SearchPaths,
				settings.SearchPaths...,
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeAppend),
			)
		}
	}
	if settings.Templates != nil {
		for _, item := range settings.Templates {
			path := "settings.templates." + item.ID
			if lock, blocked := blockingLock(result, path); blocked {
				recordLockedOverride(
					result,
					layer.Source,
					path,
					string(MergeKeyedList),
					lock,
				)
				continue
			}
			result.Config.Settings.Templates = mergeTemplates(
				result.Config.Settings.Templates,
				[]TemplateSetting{item},
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeKeyedList),
			)
		}
	}
	if len(layer.Document.Plugins) > 0 {
		for _, namespace := range sortedKeys(layer.Document.Plugins) {
			path := "plugins." + namespace
			if lock, blocked := blockingLock(result, path); blocked {
				recordLockedOverride(
					result,
					layer.Source,
					path,
					string(MergeDeepMap),
					lock,
				)
				continue
			}
			result.Config.Plugins = deepMergeMap(
				result.Config.Plugins,
				map[string]any{
					namespace: layer.Document.Plugins[namespace],
				},
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeDeepMap),
			)
		}
	}
	if len(layer.Document.SecretReferences) > 0 {
		for _, name := range sortedKeys(layer.Document.SecretReferences) {
			path := "secret_references." + name
			reference := layer.Document.SecretReferences[name]
			if layer.Source.Kind != target && !reference.Inherit {
				result.Provenance[path] = append(
					result.Provenance[path],
					Contribution{
						Path:      path,
						Source:    layer.Source,
						Operation: string(MergeKeyedList),
						Applied:   false,
						Reason:    "not inherited into target scope",
					},
				)
				continue
			}
			if lock, blocked := blockingLock(result, path); blocked {
				recordLockedOverride(
					result,
					layer.Source,
					path,
					string(MergeKeyedList),
					lock,
				)
				continue
			}
			result.Config.SecretReferences[name] = reference
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeKeyedList),
			)
		}
	}
	if len(layer.Document.EnvironmentFiles) > 0 {
		for _, item := range layer.Document.EnvironmentFiles {
			path := "environment_files." + item.Path
			if layer.Source.Kind != target && !item.Inherit {
				result.Provenance[path] = append(
					result.Provenance[path],
					Contribution{
						Path:      path,
						Source:    layer.Source,
						Operation: string(MergeKeyedList),
						Applied:   false,
						Reason:    "not inherited into target scope",
					},
				)
				continue
			}
			if lock, blocked := blockingLock(result, path); blocked {
				recordLockedOverride(
					result,
					layer.Source,
					path,
					string(MergeKeyedList),
					lock,
				)
				continue
			}
			result.Config.EnvironmentFiles = mergeEnvironmentFiles(
				result.Config.EnvironmentFiles,
				[]EnvironmentFile{item},
			)
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeKeyedList),
			)
		}
	}
	if len(layer.Document.MachineBindings) > 0 {
		for _, name := range sortedKeys(layer.Document.MachineBindings) {
			path := "machine_bindings." + name
			if lock, blocked := blockingLock(result, path); blocked {
				recordLockedOverride(
					result,
					layer.Source,
					path,
					string(MergeKeyedList),
					lock,
				)
				continue
			}
			binding := result.Config.MachineBindings[name]
			addition := layer.Document.MachineBindings[name]
			if addition.Path != "" {
				binding.Path = addition.Path
			}
			if addition.Executable != "" {
				binding.Executable = addition.Executable
			}
			result.Config.MachineBindings[name] = binding
			addContribution(
				result,
				layer.Source,
				path,
				string(MergeKeyedList),
			)
		}
	}

	lockPaths := make([]string, 0, len(layer.Document.Locks))
	for path := range layer.Document.Locks {
		lockPaths = append(lockPaths, path)
	}
	sort.Strings(lockPaths)
	for _, path := range lockPaths {
		if !knownOperationPath(path) {
			return fmt.Errorf("unsupported lock path %q", path)
		}
		authority := policyAuthority(layer.Source.Kind)
		existing, exists := strongestOverlappingLock(result.Locks, path)
		if exists && authority < existing.Authority {
			result.Findings = append(result.Findings, Finding{
				ID:       "configuration.lock_forbidden",
				Severity: "error",
				Summary:  "A lower-authority source cannot replace this lock.",
				Path:     path,
				Source:   sourcePointer(layer.Source),
				Evidence: []string{fmt.Sprintf(
					"%s cannot replace lock owned by %s",
					layer.Source.Name,
					existing.Source.Name,
				)},
				Remediation: "Remove the lower-authority lock or change it at the owning scope.",
			})
			result.Provenance[path] = append(
				result.Provenance[path],
				Contribution{
					Path:      path,
					Source:    layer.Source,
					Operation: "lock",
					Applied:   false,
					Reason:    "insufficient policy authority",
				},
			)
			continue
		}
		for existingPath, existingLock := range result.Locks {
			if relatedFieldPaths(path, existingPath) &&
				authority > existingLock.Authority {
				delete(result.Locks, existingPath)
			}
		}
		result.Locks[path] = Lock{
			Path:      path,
			Source:    layer.Source,
			Reason:    layer.Document.Locks[path],
			Authority: authority,
		}
		addContribution(result, layer.Source, path, "lock")
	}
	return nil
}

func validateSourceDocument(source Source, document Document) error {
	if !source.Portable {
		return nil
	}
	if len(document.MachineBindings) > 0 {
		return fmt.Errorf(
			"portable %s configuration cannot contain machine bindings",
			source.Kind,
		)
	}
	for path := range document.Locks {
		if machineSpecificPath(path) {
			return fmt.Errorf(
				"portable %s configuration cannot lock machine bindings",
				source.Kind,
			)
		}
	}
	for _, operation := range [][]string{
		document.Unlocks,
		document.Reset,
		document.Delete,
	} {
		for _, path := range operation {
			if machineSpecificPath(path) {
				return fmt.Errorf(
					"portable %s configuration cannot operate on machine bindings",
					source.Kind,
				)
			}
		}
	}
	for _, path := range document.Settings.SearchPaths {
		if !portableRelativePath(path) {
			return fmt.Errorf(
				"portable %s search path must be relative",
				source.Kind,
			)
		}
	}
	for _, environmentFile := range document.EnvironmentFiles {
		if !portableRelativePath(environmentFile.Path) {
			return fmt.Errorf(
				"portable %s environment file path must be relative",
				source.Kind,
			)
		}
		if !environmentFile.GitIgnore {
			return fmt.Errorf(
				"portable %s environment file must be declared gitignored",
				source.Kind,
			)
		}
	}
	return nil
}

func portableRelativePath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if filepath.IsAbs(path) ||
		filepath.VolumeName(path) != "" ||
		strings.HasPrefix(normalized, "//") ||
		windowsDrivePath(normalized) {
		return false
	}
	cleaned := filepath.Clean(normalized)
	return cleaned != ".." &&
		!strings.HasPrefix(cleaned, "../")
}

func windowsDrivePath(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	first := path[0]
	return first >= 'A' && first <= 'Z' ||
		first >= 'a' && first <= 'z'
}

func machineSpecificPath(path string) bool {
	return path == "machine_bindings" ||
		strings.HasPrefix(path, "machine_bindings.")
}

func unlockPath(result *Resolution, source Source, path string) {
	lock, exists := result.Locks[path]
	if !exists {
		addContribution(result, source, path, "unlock")
		return
	}
	if policyAuthority(source.Kind) < lock.Authority {
		result.Findings = append(result.Findings, Finding{
			ID:       "configuration.unlock_forbidden",
			Severity: "error",
			Summary:  "A lower-authority source cannot remove this lock.",
			Path:     path,
			Source:   sourcePointer(source),
			Evidence: []string{fmt.Sprintf(
				"%s cannot unlock policy owned by %s",
				source.Name,
				lock.Source.Name,
			)},
			Remediation: "Remove the unlock or change the lock at its owning scope.",
		})
		result.Provenance[path] = append(
			result.Provenance[path],
			Contribution{
				Path:      path,
				Source:    source,
				Operation: "unlock",
				Applied:   false,
				Reason:    "insufficient policy authority",
			},
		)
		return
	}
	delete(result.Locks, path)
	addContribution(result, source, path, "unlock")
}

// blockingLock returns the lock that refuses an override of path, if any.
//
// It deliberately does not consider the overriding source: a lock refuses every
// later layer regardless of that layer's own authority. Authority is compared at
// the two points where a lock itself changes — unlockPath, and lock replacement
// in applyLayer — so raising a locked field is a matter of unlocking or replacing
// the lock at sufficient authority first, never of overriding around it.
func blockingLock(result *Resolution, path string) (Lock, bool) {
	return strongestOverlappingLock(result.Locks, path)
}

func strongestOverlappingLock(
	locks map[string]Lock,
	path string,
) (Lock, bool) {
	var strongest Lock
	found := false
	for lockPath, lock := range locks {
		if !relatedFieldPaths(path, lockPath) {
			continue
		}
		if !found ||
			lock.Authority > strongest.Authority ||
			lock.Authority == strongest.Authority &&
				len(lockPath) > len(strongest.Path) {
			strongest = lock
			found = true
		}
	}
	return strongest, found
}

func relatedFieldPaths(left string, right string) bool {
	return left == right ||
		strings.HasPrefix(left, right+".") ||
		strings.HasPrefix(right, left+".")
}

func recordLockedOverride(
	result *Resolution,
	source Source,
	path string,
	operation string,
	lock Lock,
) {
	result.Findings = append(result.Findings, Finding{
		ID:       "configuration.override_locked",
		Severity: "error",
		Summary:  "A lower-authority override was refused.",
		Path:     path,
		Source:   sourcePointer(source),
		Evidence: []string{fmt.Sprintf(
			"%s cannot override policy locked by %s",
			source.Name,
			lock.Source.Name,
		)},
		Remediation: "Remove the override or change the lock at its owning scope.",
	})
	result.Provenance[path] = append(
		result.Provenance[path],
		Contribution{
			Path:      path,
			Source:    source,
			Operation: operation,
			Applied:   false,
			Reason:    "field is locked",
		},
	)
}

func knownFieldPath(path string) bool {
	for _, spec := range FieldSpecs() {
		if spec.Path == path {
			return true
		}
	}
	return false
}

func knownOperationPath(path string) bool {
	if knownFieldPath(path) {
		return true
	}
	base, key, ok := keyedFieldPath(path)
	if !ok || strings.TrimSpace(key) == "" {
		return false
	}
	switch base {
	case "plugins":
		return pluginNamespacePattern.MatchString(key)
	case "environment_files":
		normalized := strings.ReplaceAll(key, "\\", "/")
		return key == filepath.ToSlash(filepath.Clean(normalized)) &&
			key != "."
	default:
		return strings.TrimSpace(key) == key
	}
}

func keyedFieldPath(path string) (string, string, bool) {
	for _, base := range []string{
		"settings.templates",
		"plugins",
		"secret_references",
		"environment_files",
		"machine_bindings",
	} {
		prefix := base + "."
		if strings.HasPrefix(path, prefix) {
			return base, strings.TrimPrefix(path, prefix), true
		}
	}
	return "", "", false
}

func policyAuthority(kind ScopeKind) int {
	switch kind {
	case ScopeBuiltin:
		return 1000
	case ScopeWorkspace:
		return 900
	case ScopeOrganization:
		return 800
	case ScopeHost:
		return 700
	case ScopeOwner:
		return 600
	case ScopeRepository:
		return 500
	case ScopeTicket:
		return 400
	case ScopeUserGlobal:
		return 100
	case ScopeEnvironment, ScopeRequest:
		return 50
	default:
		return 0
	}
}

func configFromDocument(document Document) Config {
	return Config{
		Settings: Settings{
			Profile: cloneString(document.Settings.Profile),
			Policy: PolicySettings{
				Conformance: cloneString(
					document.Settings.Policy.Conformance,
				),
				Freshness: FreshnessSettings{
					Enabled: cloneBool(
						document.Settings.Policy.Freshness.Enabled,
					),
					TTL: cloneString(
						document.Settings.Policy.Freshness.TTL,
					),
				},
			},
			Tags: append([]string(nil), document.Settings.Tags...),
			SearchPaths: append(
				[]string(nil),
				document.Settings.SearchPaths...,
			),
			Templates: cloneTemplates(document.Settings.Templates),
		},
		Plugins:          cloneMap(document.Plugins),
		SecretReferences: cloneSecretReferences(document.SecretReferences),
		EnvironmentFiles: cloneEnvironmentFiles(document.EnvironmentFiles),
		MachineBindings:  cloneMachineBindings(document.MachineBindings),
	}
}

func resetPath(config *Config, defaults Config, path string) error {
	if base, key, ok := keyedFieldPath(path); ok {
		switch base {
		case "settings.templates":
			if item, exists := findTemplate(defaults.Settings.Templates, key); exists {
				config.Settings.Templates = mergeTemplates(
					config.Settings.Templates,
					[]TemplateSetting{item},
				)
			} else {
				config.Settings.Templates = removeTemplate(
					config.Settings.Templates,
					key,
				)
			}
		case "plugins":
			if value, exists := defaults.Plugins[key]; exists {
				config.Plugins[key] = cloneValue(value)
			} else {
				delete(config.Plugins, key)
			}
		case "secret_references":
			if reference, exists := defaults.SecretReferences[key]; exists {
				config.SecretReferences[key] = reference
			} else {
				delete(config.SecretReferences, key)
			}
		case "environment_files":
			if item, exists := findEnvironmentFile(
				defaults.EnvironmentFiles,
				key,
			); exists {
				config.EnvironmentFiles = mergeEnvironmentFiles(
					config.EnvironmentFiles,
					[]EnvironmentFile{item},
				)
			} else {
				config.EnvironmentFiles = removeEnvironmentFile(
					config.EnvironmentFiles,
					key,
				)
			}
		case "machine_bindings":
			if binding, exists := defaults.MachineBindings[key]; exists {
				config.MachineBindings[key] = binding
			} else {
				delete(config.MachineBindings, key)
			}
		default:
			return fmt.Errorf("unsupported reset path %q", path)
		}
		return nil
	}
	switch path {
	case "settings.profile":
		config.Settings.Profile = cloneString(defaults.Settings.Profile)
	case "settings.policy":
		config.Settings.Policy = clonePolicy(defaults.Settings.Policy)
	case "settings.policy.conformance":
		config.Settings.Policy.Conformance = cloneString(
			defaults.Settings.Policy.Conformance,
		)
	case "settings.policy.freshness":
		config.Settings.Policy.Freshness = cloneFreshness(
			defaults.Settings.Policy.Freshness,
		)
	case "settings.policy.freshness.enabled":
		config.Settings.Policy.Freshness.Enabled = cloneBool(
			defaults.Settings.Policy.Freshness.Enabled,
		)
	case "settings.policy.freshness.ttl":
		config.Settings.Policy.Freshness.TTL = cloneString(
			defaults.Settings.Policy.Freshness.TTL,
		)
	case "settings.tags":
		config.Settings.Tags = append(
			[]string(nil),
			defaults.Settings.Tags...,
		)
	case "settings.search_paths":
		config.Settings.SearchPaths = append(
			[]string(nil),
			defaults.Settings.SearchPaths...,
		)
	case "settings.templates":
		config.Settings.Templates = cloneTemplates(
			defaults.Settings.Templates,
		)
	case "plugins":
		config.Plugins = cloneMap(defaults.Plugins)
	case "secret_references":
		config.SecretReferences = cloneSecretReferences(
			defaults.SecretReferences,
		)
	case "environment_files":
		config.EnvironmentFiles = cloneEnvironmentFiles(
			defaults.EnvironmentFiles,
		)
	case "machine_bindings":
		config.MachineBindings = cloneMachineBindings(
			defaults.MachineBindings,
		)
	default:
		return fmt.Errorf("unsupported reset path %q", path)
	}
	return nil
}

func deletePath(config *Config, path string) error {
	if base, key, ok := keyedFieldPath(path); ok {
		switch base {
		case "settings.templates":
			config.Settings.Templates = removeTemplate(
				config.Settings.Templates,
				key,
			)
		case "plugins":
			delete(config.Plugins, key)
		case "secret_references":
			delete(config.SecretReferences, key)
		case "environment_files":
			config.EnvironmentFiles = removeEnvironmentFile(
				config.EnvironmentFiles,
				key,
			)
		case "machine_bindings":
			delete(config.MachineBindings, key)
		default:
			return fmt.Errorf("unsupported delete path %q", path)
		}
		return nil
	}
	switch path {
	case "settings.profile":
		config.Settings.Profile = nil
	case "settings.policy":
		config.Settings.Policy = PolicySettings{}
	case "settings.policy.conformance":
		config.Settings.Policy.Conformance = nil
	case "settings.policy.freshness":
		config.Settings.Policy.Freshness = FreshnessSettings{}
	case "settings.policy.freshness.enabled":
		config.Settings.Policy.Freshness.Enabled = nil
	case "settings.policy.freshness.ttl":
		config.Settings.Policy.Freshness.TTL = nil
	case "settings.tags":
		config.Settings.Tags = []string{}
	case "settings.search_paths":
		config.Settings.SearchPaths = []string{}
	case "settings.templates":
		config.Settings.Templates = []TemplateSetting{}
	case "plugins":
		config.Plugins = map[string]any{}
	case "secret_references":
		config.SecretReferences = map[string]SecretReference{}
	case "environment_files":
		config.EnvironmentFiles = []EnvironmentFile{}
	case "machine_bindings":
		config.MachineBindings = map[string]MachineBinding{}
	default:
		return fmt.Errorf("unsupported delete path %q", path)
	}
	return nil
}

func setUnion(current []string, additions []string) []string {
	result := append([]string(nil), current...)
	seen := make(map[string]struct{}, len(current)+len(additions))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mergeTemplates(
	current []TemplateSetting,
	additions []TemplateSetting,
) []TemplateSetting {
	result := cloneTemplates(current)
	indexByID := make(map[string]int, len(result))
	for index, item := range result {
		indexByID[item.ID] = index
	}
	for _, addition := range additions {
		index, ok := indexByID[addition.ID]
		if !ok {
			indexByID[addition.ID] = len(result)
			result = append(result, cloneTemplate(addition))
			continue
		}
		item := result[index]
		if addition.Version != "" {
			item.Version = addition.Version
		}
		if addition.Source != "" {
			item.Source = addition.Source
		}
		if addition.Enabled != nil {
			item.Enabled = boolValue(*addition.Enabled)
		}
		result[index] = item
	}
	return result
}

func findTemplate(
	items []TemplateSetting,
	id string,
) (TemplateSetting, bool) {
	for _, item := range items {
		if item.ID == id {
			return cloneTemplate(item), true
		}
	}
	return TemplateSetting{}, false
}

func removeTemplate(
	items []TemplateSetting,
	id string,
) []TemplateSetting {
	result := make([]TemplateSetting, 0, len(items))
	for _, item := range items {
		if item.ID != id {
			result = append(result, cloneTemplate(item))
		}
	}
	return result
}

func deepMergeMap(
	current map[string]any,
	additions map[string]any,
) map[string]any {
	result := cloneMap(current)
	for key, addition := range additions {
		additionMap, additionIsMap := addition.(map[string]any)
		currentMap, currentIsMap := result[key].(map[string]any)
		if additionIsMap && currentIsMap {
			result[key] = deepMergeMap(currentMap, additionMap)
			continue
		}
		result[key] = cloneValue(addition)
	}
	return result
}

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = cloneValue(value)
	}
	return result
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, cloneValue(item))
		}
		return result
	default:
		return value
	}
}

func mergeEnvironmentFiles(
	current []EnvironmentFile,
	additions []EnvironmentFile,
) []EnvironmentFile {
	result := append([]EnvironmentFile(nil), current...)
	indexByPath := make(map[string]int, len(result))
	for index, item := range result {
		indexByPath[item.Path] = index
	}
	for _, addition := range additions {
		if index, ok := indexByPath[addition.Path]; ok {
			addition.Allow = append([]string(nil), addition.Allow...)
			result[index] = addition
			continue
		}
		addition.Allow = append([]string(nil), addition.Allow...)
		indexByPath[addition.Path] = len(result)
		result = append(result, addition)
	}
	return result
}

func findEnvironmentFile(
	items []EnvironmentFile,
	path string,
) (EnvironmentFile, bool) {
	for _, item := range items {
		if item.Path == path {
			item.Allow = append([]string(nil), item.Allow...)
			return item, true
		}
	}
	return EnvironmentFile{}, false
}

func removeEnvironmentFile(
	items []EnvironmentFile,
	path string,
) []EnvironmentFile {
	result := make([]EnvironmentFile, 0, len(items))
	for _, item := range items {
		if item.Path == path {
			continue
		}
		item.Allow = append([]string(nil), item.Allow...)
		result = append(result, item)
	}
	return result
}

func clonePolicy(policy PolicySettings) PolicySettings {
	return PolicySettings{
		Conformance: cloneString(policy.Conformance),
		Freshness:   cloneFreshness(policy.Freshness),
	}
}

func cloneFreshness(freshness FreshnessSettings) FreshnessSettings {
	return FreshnessSettings{
		Enabled: cloneBool(freshness.Enabled),
		TTL:     cloneString(freshness.TTL),
	}
}

func cloneSecretReferences(
	input map[string]SecretReference,
) map[string]SecretReference {
	result := make(map[string]SecretReference, len(input))
	for name, reference := range input {
		result[name] = reference
	}
	return result
}

func cloneEnvironmentFiles(input []EnvironmentFile) []EnvironmentFile {
	result := make([]EnvironmentFile, 0, len(input))
	for _, item := range input {
		item.Allow = append([]string(nil), item.Allow...)
		result = append(result, item)
	}
	return result
}

func cloneMachineBindings(
	input map[string]MachineBinding,
) map[string]MachineBinding {
	result := make(map[string]MachineBinding, len(input))
	for name, binding := range input {
		result[name] = binding
	}
	return result
}

func cloneTemplates(items []TemplateSetting) []TemplateSetting {
	result := make([]TemplateSetting, 0, len(items))
	for _, item := range items {
		result = append(result, cloneTemplate(item))
	}
	return result
}

func cloneTemplate(item TemplateSetting) TemplateSetting {
	item.Enabled = cloneBool(item.Enabled)
	return item
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	return stringValue(*value)
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	return boolValue(*value)
}

func addContribution(
	result *Resolution,
	source Source,
	path string,
	operation string,
) {
	result.Provenance[path] = append(
		result.Provenance[path],
		Contribution{
			Path:      path,
			Source:    source,
			Operation: operation,
			Applied:   true,
		},
	)
}

func scopePrecedence(kind ScopeKind) int {
	switch kind {
	case ScopeBuiltin:
		return 0
	case ScopeUserGlobal:
		return 1
	case ScopeWorkspace:
		return 2
	case ScopeOrganization:
		return 3
	case ScopeHost:
		return 4
	case ScopeOwner:
		return 5
	case ScopeRepository:
		return 6
	case ScopeTicket:
		return 7
	case ScopeEnvironment:
		return 8
	case ScopeRequest:
		return 9
	default:
		return 100
	}
}

func overrideDocument(
	overrides []Override,
	environment bool,
) (Document, error) {
	document := Document{SchemaVersion: SchemaVersion}
	for _, override := range overrides {
		if override.Secret {
			return Document{}, fmt.Errorf(
				"secret input cannot be ordinary configuration",
			)
		}
		if environment && !environmentField(override.Name, override.Path) {
			return Document{}, fmt.Errorf(
				"environment override is not allowlisted",
			)
		}
		switch override.Path {
		case "settings.profile":
			value, ok := override.Value.(string)
			if !ok {
				return Document{}, fmt.Errorf(
					"%s override must be text",
					override.Path,
				)
			}
			document.Settings.Profile = stringValue(value)
		case "settings.policy.conformance":
			value, ok := override.Value.(string)
			if !ok {
				return Document{}, fmt.Errorf(
					"%s override must be text",
					override.Path,
				)
			}
			document.Settings.Policy.Conformance = stringValue(value)
		case "settings.policy.freshness.enabled":
			value, ok := override.Value.(bool)
			if !ok {
				return Document{}, fmt.Errorf(
					"%s override must be true or false",
					override.Path,
				)
			}
			document.Settings.Policy.Freshness.Enabled = boolValue(value)
		case "settings.policy.freshness.ttl":
			value, ok := override.Value.(string)
			if !ok {
				return Document{}, fmt.Errorf(
					"%s override must be text",
					override.Path,
				)
			}
			document.Settings.Policy.Freshness.TTL = stringValue(value)
		default:
			return Document{}, fmt.Errorf(
				"unsupported override path %q",
				override.Path,
			)
		}
	}
	return document, nil
}

func environmentField(name string, path string) bool {
	for _, spec := range FieldSpecs() {
		if spec.Path != path {
			continue
		}
		for _, allowedName := range spec.EnvironmentNames {
			if name == allowedName {
				return true
			}
		}
	}
	return false
}

func recordMissingSecretReferences(result *Resolution) {
	references := make(map[string][]string)
	collectPluginSecretReferences(result.Config.Plugins, "plugins", references)
	names := sortedKeys(references)
	for _, name := range names {
		if _, exists := result.Config.SecretReferences[name]; exists {
			continue
		}
		paths := append([]string(nil), references[name]...)
		sort.Strings(paths)
		for _, path := range paths {
			result.Findings = append(result.Findings, Finding{
				ID:          "configuration.secret_reference.missing",
				Severity:    "error",
				Summary:     "A plugin references an undefined secret reference.",
				Path:        path,
				Source:      contributionSource(result.Provenance, path),
				Evidence:    []string{path},
				Remediation: "Declare the reference at an authorized scope or remove the plugin reference.",
			})
		}
	}
}

func collectPluginSecretReferences(
	value any,
	path string,
	references map[string][]string,
) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			childPath := path + "." + key
			if key == "secret_reference" {
				if reference, ok := typed[key].(string); ok {
					references[reference] = append(
						references[reference],
						childPath,
					)
				}
				continue
			}
			collectPluginSecretReferences(typed[key], childPath, references)
		}
	case []any:
		for index, child := range typed {
			collectPluginSecretReferences(
				child,
				fmt.Sprintf("%s[%d]", path, index),
				references,
			)
		}
	}
}

func contributionSource(
	provenance map[string][]Contribution,
	path string,
) *Source {
	bestPath := ""
	var source *Source
	for contributionPath, contributions := range provenance {
		if path != contributionPath &&
			!strings.HasPrefix(path, contributionPath+".") {
			continue
		}
		if len(contributionPath) < len(bestPath) || len(contributions) == 0 {
			continue
		}
		last := contributions[len(contributions)-1]
		bestPath = contributionPath
		source = sourcePointer(last.Source)
	}
	return source
}
