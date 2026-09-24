package configuration

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

func TestEnvironmentOverridesUseExactSchemaAllowlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override Override
	}{
		{
			name: "rejects unrecognized environment variable",
			override: Override{
				Name:  "GITHUB_TOKEN",
				Path:  "settings.profile",
				Value: "must-not-become-config",
			},
		},
		{
			name: "rejects unnamed environment variable",
			override: Override{
				Path:  "settings.profile",
				Value: "unnamed",
			},
		},
		{
			name: "rejects allowlisted name paired with wrong field",
			override: Override{
				Name:  "ADB_PROFILE",
				Path:  "settings.policy.conformance",
				Value: "warn",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Resolve(LayerResolveRequest{
				Environment: []Override{test.override},
			}); err == nil {
				t.Fatalf(
					"unauthorized environment override was accepted: %#v",
					test.override,
				)
			}
		})
	}

	t.Run("accepts exact allowlisted name and field", func(t *testing.T) {
		t.Parallel()

		result, err := Resolve(LayerResolveRequest{
			Environment: []Override{{
				Name:  "ADB_PROFILE",
				Path:  "settings.profile",
				Value: "engineering@v1",
			}},
		})
		if err != nil {
			t.Fatalf("resolve allowlisted environment override: %v", err)
		}
		if result.Config.Settings.Profile == nil ||
			*result.Config.Settings.Profile != "engineering@v1" {
			t.Fatalf(
				"environment profile = %#v",
				result.Config.Settings.Profile,
			)
		}
	})
}

func TestResolvedTemplatesRequireExactVersionAndSemanticSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template TemplateSetting
	}{
		{
			name:     "rejects missing exact version",
			template: TemplateSetting{ID: "custom", Source: "workspace"},
		},
		{
			name: "rejects filesystem source instead of semantic scope",
			template: TemplateSetting{
				ID:      "custom",
				Version: "v1",
				Source:  "/arbitrary/ancestor",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Resolve(LayerResolveRequest{
				Layers: []Layer{{
					Source: Source{
						Kind: ScopeWorkspace, Name: "workspace", Portable: true,
					},
					Document: Document{
						SchemaVersion: SchemaVersion,
						Settings: Settings{
							Templates: []TemplateSetting{test.template},
						},
					},
				}},
			}); err == nil {
				t.Fatalf(
					"inexact template resolution was accepted: %#v",
					test.template,
				)
			}
		})
	}
}

func TestEnvironmentFilesRequireExplicitAllowlistAndInheritance(t *testing.T) {
	t.Parallel()

	invalidDocuments := []struct {
		name    string
		content string
	}{
		{
			name: "rejects missing variable allowlist",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    gitignored: true
`,
		},
		{
			name: "rejects duplicate normalized paths",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    allow: [ONE]
    gitignored: true
  - path: ./.env
    allow: [TWO]
    gitignored: true
`,
		},
	}
	for _, test := range invalidDocuments {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := DecodeDocument(
				strings.NewReader(test.content),
			); err == nil {
				t.Fatal("invalid environment-file declaration was accepted")
			}
		})
	}

	result, err := Resolve(LayerResolveRequest{
		Target: ScopeOrganization,
		Layers: []Layer{
			{
				Source: Source{
					Kind: ScopeUserGlobal,
					Name: "user",
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					EnvironmentFiles: []EnvironmentFile{{
						Path:    "user.env",
						Allow:   []string{"USER_TOKEN"},
						Inherit: true,
					}},
				},
			},
			{
				Source: Source{
					Kind:     ScopeWorkspace,
					Name:     "workspace",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					EnvironmentFiles: []EnvironmentFile{{
						Path:      "workspace.env",
						Allow:     []string{"WORKSPACE_TOKEN"},
						GitIgnore: true,
					}},
				},
			},
			{
				Source: Source{
					Kind:     ScopeOrganization,
					Name:     "organization",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					EnvironmentFiles: []EnvironmentFile{{
						Path:      "organization.env",
						Allow:     []string{"ORGANIZATION_TOKEN"},
						GitIgnore: true,
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve environment-file inheritance: %v", err)
	}
	gotPaths := make([]string, 0, len(result.Config.EnvironmentFiles))
	for _, item := range result.Config.EnvironmentFiles {
		gotPaths = append(gotPaths, item.Path)
	}
	if !reflect.DeepEqual(gotPaths, []string{
		"user.env",
		"organization.env",
	}) {
		t.Fatalf("active environment files = %v", gotPaths)
	}
}

func TestSecretReferencesRequireExplicitInheritance(t *testing.T) {
	t.Parallel()

	layer := func(inherit bool) Layer {
		return Layer{
			Source: Source{Kind: ScopeUserGlobal, Name: "user"},
			Document: Document{
				SchemaVersion: SchemaVersion,
				Plugins: map[string]any{
					"acme.example": map[string]any{
						"secret_reference": "github",
					},
				},
				SecretReferences: map[string]SecretReference{
					"github": {
						Provider: "environment",
						Key:      "GITHUB_TOKEN",
						Inherit:  inherit,
					},
				},
			},
		}
	}

	withoutInheritance, err := Resolve(LayerResolveRequest{
		Target: ScopeWorkspace,
		Layers: []Layer{layer(false)},
	})
	if err != nil {
		t.Fatalf("resolve non-inherited reference: %v", err)
	}
	if !hasFinding(
		withoutInheritance.Findings,
		"configuration.secret_reference.missing",
	) {
		t.Fatalf(
			"non-inherited secret reference became active: %#v",
			withoutInheritance,
		)
	}

	withInheritance, err := Resolve(LayerResolveRequest{
		Target: ScopeWorkspace,
		Layers: []Layer{layer(true)},
	})
	if err != nil {
		t.Fatalf("resolve inherited reference: %v", err)
	}
	if hasFinding(
		withInheritance.Findings,
		"configuration.secret_reference.missing",
	) {
		t.Fatalf("inherited reference was unavailable: %#v", withInheritance)
	}
}

func TestPluginValuesAreClosedSecretSafeAndReferenceChecked(t *testing.T) {
	t.Parallel()

	const secret = "top-secret-value"
	secretDocuments := []struct {
		name    string
		content string
	}{
		{
			name: "rejects probable secret in plugin payload",
			content: `
schema_version: aidb.config/v1
plugins:
  acme.example:
    payload: ` + secret + `
`,
		},
		{
			name: "rejects probable secret in typed core field",
			content: `
schema_version: aidb.config/v1
settings:
  policy:
    freshness:
      enabled: ` + secret + `
`,
		},
	}
	for _, test := range secretDocuments {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeDocument(strings.NewReader(test.content))
			if err == nil {
				t.Fatal("secret-bearing configuration was accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("decode error leaked scalar value: %v", err)
			}
		})
	}

	_, err := Resolve(LayerResolveRequest{
		Layers: []Layer{{
			Source: Source{Kind: ScopeUserGlobal, Name: "user"},
			Document: Document{
				SchemaVersion: SchemaVersion,
				Plugins: map[string]any{
					"acme.example": map[string]string{
						"mutable": "alias",
					},
				},
			},
		}},
	})
	if err == nil {
		t.Fatal("unsupported plugin composite type was accepted")
	}

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{{
			Source: Source{Kind: ScopeUserGlobal, Name: "user"},
			Document: Document{
				SchemaVersion: SchemaVersion,
				Plugins: map[string]any{
					"acme.example": map[string]any{
						"secret_reference": "missing",
					},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("resolve missing secret reference: %v", err)
	}
	if !hasFinding(
		result.Findings,
		"configuration.secret_reference.missing",
	) {
		t.Fatalf("missing-reference findings = %#v", result.Findings)
	}

	paths := createScopePaths(t)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
plugins:
  acme.example:
    secret_reference: missing
`)
	serviceResult, err := NewService().Resolve(
		context.Background(),
		ResolveRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("resolve missing service reference: %v", err)
	}
	if serviceResult.Data.Complete ||
		serviceResult.Data.Config != nil ||
		!hasFinding(
			serviceResult.Data.Findings,
			"configuration.secret_reference.missing",
		) {
		t.Fatalf("missing service reference result = %#v", serviceResult)
	}
}

func TestOrdinaryCoreFieldsRejectProbableSecretValues(t *testing.T) {
	t.Parallel()

	const secret = "top-secret-value"
	_, err := DecodeDocument(strings.NewReader(`
schema_version: aidb.config/v1
settings:
  profile: ` + secret + `
`))
	if err == nil {
		t.Fatal("probable secret was accepted as a profile")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("profile validation leaked probable secret: %v", err)
	}

	_, err = Resolve(LayerResolveRequest{
		Environment: []Override{{
			Name:  "ADB_PROFILE",
			Path:  "settings.profile",
			Value: secret,
		}},
	})
	if err == nil {
		t.Fatal("probable secret environment value became ordinary config")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("environment validation leaked probable secret: %v", err)
	}
}

func TestPortableEnvironmentFilesAreActuallyGitIgnored(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    allow: [WORKSPACE_TOKEN]
    gitignored: true
`)
	command := exec.Command("git", "init", "--quiet", paths.WorkspaceRoot)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize git repository: %v: %s", err, output)
	}

	service := NewService()
	unignored, err := service.Sources(
		context.Background(),
		SourcesRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("inspect unignored environment file: %v", err)
	}
	if !hasFinding(
		unignored.Data.Findings,
		"configuration.environment_file.not_ignored",
	) {
		t.Fatalf("unignored environment file was trusted: %#v", unignored)
	}

	writeConfiguration(
		t,
		filepath.Join(paths.WorkspaceRoot, ".gitignore"),
		".env\n",
	)
	ignored, err := service.Sources(
		context.Background(),
		SourcesRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("inspect ignored environment file: %v", err)
	}
	if ignored.Outcome != capability.OutcomeHealthy {
		t.Fatalf("ignored environment file result = %#v", ignored)
	}
}

func TestExplainIncludesStrategyAndCompositeOrKeyedProvenance(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
settings:
  policy:
    conformance: block
  templates:
    - id: design
      version: v1
      source: workspace
plugins:
  acme.example:
    retries: 3
`)
	service := NewService()

	policy, err := service.Explain(context.Background(), ExplainRequest{
		Scopes: paths,
		Path:   "settings.policy",
	})
	if err != nil {
		t.Fatalf("explain policy: %v", err)
	}
	if policy.Data.Strategy != MergeDeepMap ||
		len(policy.Data.Contributions) == 0 {
		t.Fatalf("policy explanation = %#v", policy.Data)
	}

	template, err := service.Explain(context.Background(), ExplainRequest{
		Scopes: paths,
		Path:   "settings.templates.design",
	})
	if err != nil {
		t.Fatalf("explain keyed template: %v", err)
	}
	if template.Data.Strategy != MergeKeyedList ||
		template.Data.Value.(TemplateSetting).ID != "design" ||
		len(template.Data.Contributions) == 0 {
		t.Fatalf("template explanation = %#v", template.Data)
	}

	plugin, err := service.Explain(context.Background(), ExplainRequest{
		Scopes: paths,
		Path:   "plugins.acme.example.retries",
	})
	if err != nil {
		t.Fatalf("explain plugin setting: %v", err)
	}
	if plugin.Data.Strategy != MergeDeepMap ||
		plugin.Data.Value != 3 {
		t.Fatalf("plugin explanation = %#v", plugin.Data)
	}
}

func TestInvalidSourceSuppressesPartialResolutionDetails(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
settings:
  profile: trustworthy-looking-but-partial@v1
locks:
  settings.profile: workspace policy
`)
	writeConfiguration(t, configPath(paths.RepositoryRoot), `
schema_version: aidb.config/v1
unknown_core_field: invalid
`)

	service := NewService()
	resolved, err := service.Resolve(
		context.Background(),
		ResolveRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("resolve invalid source: %v", err)
	}
	if resolved.Data.Complete ||
		resolved.Data.Config != nil ||
		len(resolved.Data.Provenance) != 0 ||
		len(resolved.Data.Locks) != 0 {
		t.Fatalf("partial resolution details escaped: %#v", resolved.Data)
	}

	explained, err := service.Explain(context.Background(), ExplainRequest{
		Scopes: paths,
		Path:   "settings.profile",
	})
	if err != nil {
		t.Fatalf("explain invalid source: %v", err)
	}
	if explained.Data.Complete ||
		explained.Data.Value != nil ||
		len(explained.Data.Contributions) != 0 ||
		explained.Data.Lock != nil {
		t.Fatalf("partial explanation escaped: %#v", explained.Data)
	}
}

func TestDeleteAndResetAddressIndividualKeyedValues(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Target: ScopeWorkspace,
		Layers: []Layer{
			{
				Source: Source{Kind: ScopeUserGlobal, Name: "user"},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Settings: Settings{
						Templates: []TemplateSetting{
							{ID: "ticket", Version: "custom"},
							{ID: "design", Version: "v1"},
						},
					},
					Plugins: map[string]any{
						"acme.one": map[string]any{"enabled": true},
						"acme.two": map[string]any{"enabled": true},
					},
					SecretReferences: map[string]SecretReference{
						"one": {
							Provider: "environment",
							Key:      "ONE",
							Inherit:  true,
						},
						"two": {
							Provider: "environment",
							Key:      "TWO",
							Inherit:  true,
						},
					},
					EnvironmentFiles: []EnvironmentFile{
						{
							Path:    "one.env",
							Allow:   []string{"ONE"},
							Inherit: true,
						},
						{
							Path:    "two.env",
							Allow:   []string{"TWO"},
							Inherit: true,
						},
					},
				},
			},
			{
				Source: Source{
					Kind:     ScopeWorkspace,
					Name:     "workspace",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Reset: []string{
						"settings.templates.ticket",
					},
					Delete: []string{
						"settings.templates.design",
						"plugins.acme.one",
						"secret_references.one",
						"environment_files.one.env",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve keyed reset/delete: %v", err)
	}
	if len(result.Config.Settings.Templates) != 1 ||
		result.Config.Settings.Templates[0].ID != "ticket" ||
		result.Config.Settings.Templates[0].Version != "v1" {
		t.Fatalf("templates = %#v", result.Config.Settings.Templates)
	}
	if _, ok := result.Config.Plugins["acme.one"]; ok {
		t.Fatalf("deleted plugin remained: %#v", result.Config.Plugins)
	}
	if _, ok := result.Config.Plugins["acme.two"]; !ok {
		t.Fatalf("sibling plugin was removed: %#v", result.Config.Plugins)
	}
	if _, ok := result.Config.SecretReferences["one"]; ok {
		t.Fatalf("deleted secret reference remained")
	}
	if _, ok := result.Config.SecretReferences["two"]; !ok {
		t.Fatalf("sibling secret reference was removed")
	}
	if len(result.Config.EnvironmentFiles) != 1 ||
		result.Config.EnvironmentFiles[0].Path != "two.env" {
		t.Fatalf("environment files = %#v", result.Config.EnvironmentFiles)
	}
}

func TestOverlappingLocksPreserveHighestAuthority(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{
			{
				Source: Source{Kind: ScopeUserGlobal, Name: "user"},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Locks: map[string]string{
						"settings.policy.conformance": "user child",
					},
				},
			},
			{
				Source: Source{
					Kind:     ScopeWorkspace,
					Name:     "workspace",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Locks: map[string]string{
						"settings.policy": "workspace parent",
					},
				},
			},
			{
				Source: Source{
					Kind:     ScopeRepository,
					Name:     "repository",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Locks: map[string]string{
						"settings.policy.freshness.ttl": "repository child",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve overlapping locks: %v", err)
	}
	if len(result.Locks) != 1 ||
		result.Locks["settings.policy"].Source.Kind != ScopeWorkspace {
		t.Fatalf("effective locks = %#v", result.Locks)
	}
	if !hasFinding(result.Findings, "configuration.lock_forbidden") {
		t.Fatalf("overlap findings = %#v", result.Findings)
	}
}

func TestConfigurationSourceSizeIsBounded(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	content := "schema_version: aidb.config/v1\nplugins:\n  acme.example:\n    payload: " +
		strings.Repeat("a", 2<<20)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), content)

	result, err := NewService().Sources(
		context.Background(),
		SourcesRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("inspect oversized source: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention ||
		!hasFinding(
			result.Data.Findings,
			"configuration.source.too_large",
		) {
		t.Fatalf("oversized source result = %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal oversized result: %v", err)
	}
	if strings.Contains(string(raw), strings.Repeat("a", 64)) {
		t.Fatal("oversized source content leaked through diagnostics")
	}
}

func TestEnvironmentFilePathsNormalizeBeforeKeyedMerge(t *testing.T) {
	t.Parallel()

	document, err := DecodeDocument(strings.NewReader(`
schema_version: aidb.config/v1
environment_files:
  - path: secrets/../.env
    allow: [TOKEN]
    gitignored: true
`))
	if err != nil {
		t.Fatalf("decode normalized environment file: %v", err)
	}
	if document.EnvironmentFiles[0].Path != ".env" {
		t.Fatalf(
			"normalized path = %q",
			document.EnvironmentFiles[0].Path,
		)
	}
	if filepath.IsAbs(document.EnvironmentFiles[0].Path) {
		t.Fatalf("normalized portable path became absolute")
	}
}

func TestResetAndDeleteProduceDefaultAndEmptyValuesForEveryField(t *testing.T) {
	t.Parallel()

	defaults := configFromDocument(DefaultDocument())
	seed := configFromDocument(Document{
		SchemaVersion: SchemaVersion,
		Settings: Settings{
			Profile: stringPointer("custom@v1"),
			Policy: PolicySettings{
				Conformance: stringPointer("block"),
				Freshness: FreshnessSettings{
					Enabled: boolPointer(false),
					TTL:     stringPointer("1h"),
				},
			},
			Tags:        []string{"custom"},
			SearchPaths: []string{"custom"},
			Templates: []TemplateSetting{{
				ID:      "custom",
				Version: "v9",
			}},
		},
		Plugins: map[string]any{
			"acme.example": map[string]any{"enabled": true},
		},
		SecretReferences: map[string]SecretReference{
			"custom": {Provider: "environment", Key: "CUSTOM_TOKEN"},
		},
		EnvironmentFiles: []EnvironmentFile{{
			Path:  ".env",
			Allow: []string{"CUSTOM_TOKEN"},
		}},
		MachineBindings: map[string]MachineBinding{
			"editor": {Executable: "editor"},
		},
	})

	for _, specification := range FieldSpecs() {
		specification := specification
		t.Run("reset/"+specification.Path, func(t *testing.T) {
			config := cloneConfig(seed)
			if err := resetPath(&config, defaults, specification.Path); err != nil {
				t.Fatalf("reset %s: %v", specification.Path, err)
			}
			if !reflect.DeepEqual(
				fieldValue(config, specification.Path),
				fieldValue(defaults, specification.Path),
			) {
				t.Fatalf(
					"reset %s = %#v, want %#v",
					specification.Path,
					fieldValue(config, specification.Path),
					fieldValue(defaults, specification.Path),
				)
			}
		})
		t.Run("delete/"+specification.Path, func(t *testing.T) {
			config := cloneConfig(seed)
			if err := deletePath(&config, specification.Path); err != nil {
				t.Fatalf("delete %s: %v", specification.Path, err)
			}
			if !fieldValueDeleted(config, specification.Path) {
				t.Fatalf(
					"delete %s left %#v",
					specification.Path,
					fieldValue(config, specification.Path),
				)
			}
		})
	}
}

func cloneConfig(config Config) Config {
	return Config{
		Settings: Settings{
			Profile:     cloneString(config.Settings.Profile),
			Policy:      clonePolicy(config.Settings.Policy),
			Tags:        append([]string(nil), config.Settings.Tags...),
			SearchPaths: append([]string(nil), config.Settings.SearchPaths...),
			Templates:   cloneTemplates(config.Settings.Templates),
		},
		Plugins:          cloneMap(config.Plugins),
		SecretReferences: cloneSecretReferences(config.SecretReferences),
		EnvironmentFiles: cloneEnvironmentFiles(config.EnvironmentFiles),
		MachineBindings:  cloneMachineBindings(config.MachineBindings),
	}
}

func fieldValue(config Config, path string) any {
	switch path {
	case "settings.profile":
		return config.Settings.Profile
	case "settings.policy":
		return config.Settings.Policy
	case "settings.policy.conformance":
		return config.Settings.Policy.Conformance
	case "settings.policy.freshness":
		return config.Settings.Policy.Freshness
	case "settings.policy.freshness.enabled":
		return config.Settings.Policy.Freshness.Enabled
	case "settings.policy.freshness.ttl":
		return config.Settings.Policy.Freshness.TTL
	case "settings.tags":
		return config.Settings.Tags
	case "settings.search_paths":
		return config.Settings.SearchPaths
	case "settings.templates":
		return config.Settings.Templates
	case "plugins":
		return config.Plugins
	case "secret_references":
		return config.SecretReferences
	case "environment_files":
		return config.EnvironmentFiles
	case "machine_bindings":
		return config.MachineBindings
	default:
		return nil
	}
}

func fieldValueDeleted(config Config, path string) bool {
	value := reflect.ValueOf(fieldValue(config, path))
	// A nilable container counts as deleted when it is nil OR empty; everything
	// else falls back to the zero value. Written as an if rather than a switch
	// on reflect.Kind because the struct arm was identical to the default, and
	// only these three kinds are special.
	kind := value.Kind()
	if kind == reflect.Pointer || kind == reflect.Map || kind == reflect.Slice {
		return value.IsNil() || value.Len() == 0
	}
	return value.IsZero()
}
