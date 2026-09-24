package configuration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestResolveUsesAcceptedPrecedenceAndOrderedProvenance(t *testing.T) {
	t.Parallel()

	layer := func(kind ScopeKind, value string) Layer {
		return Layer{
			Source: Source{
				Kind:     kind,
				Name:     string(kind),
				Portable: kind != ScopeUserGlobal,
			},
			Document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					Profile: stringPointer(value + "@v1"),
				},
			},
		}
	}
	layers := []Layer{
		layer(ScopeTicket, "ticket"),
		layer(ScopeWorkspace, "workspace"),
		layer(ScopeRepository, "repository"),
		layer(ScopeUserGlobal, "user"),
		layer(ScopeOwner, "owner"),
		layer(ScopeOrganization, "organization"),
		layer(ScopeHost, "host"),
	}
	semanticScopes := []ScopeKind{
		ScopeBuiltin,
		ScopeUserGlobal,
		ScopeWorkspace,
		ScopeOrganization,
		ScopeHost,
		ScopeOwner,
		ScopeRepository,
		ScopeTicket,
	}
	environment := []Override{{
		Name:  "ADB_PROFILE",
		Path:  "settings.profile",
		Value: "environment@v1",
	}}
	flags := []Override{{
		Path:  "settings.profile",
		Value: "request@v1",
	}}
	tests := []struct {
		name        string
		environment []Override
		flags       []Override
		wantProfile string
		wantScopes  []ScopeKind
	}{
		{
			name:        "semantic scopes ignore input order",
			wantProfile: "ticket@v1",
			wantScopes:  semanticScopes,
		},
		{
			name:        "environment overrides semantic scopes",
			environment: environment,
			wantProfile: "environment@v1",
			wantScopes: append(
				append([]ScopeKind(nil), semanticScopes...),
				ScopeEnvironment,
			),
		},
		{
			name:        "request overrides environment",
			environment: environment,
			flags:       flags,
			wantProfile: "request@v1",
			wantScopes: append(
				append([]ScopeKind(nil), semanticScopes...),
				ScopeEnvironment,
				ScopeRequest,
			),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result, err := Resolve(LayerResolveRequest{
				Layers:      layers,
				Environment: test.environment,
				Flags:       test.flags,
			})
			if err != nil {
				t.Fatalf("resolve configuration: %v", err)
			}

			if result.Config.Settings.Profile == nil ||
				*result.Config.Settings.Profile != test.wantProfile {
				t.Fatalf(
					"resolved profile = %#v, want %q",
					result.Config.Settings.Profile,
					test.wantProfile,
				)
			}
			gotScopes := make([]ScopeKind, 0, len(test.wantScopes))
			for _, contribution := range result.Provenance["settings.profile"] {
				if !contribution.Applied {
					continue
				}
				gotScopes = append(gotScopes, contribution.Source.Kind)
			}
			if !reflect.DeepEqual(gotScopes, test.wantScopes) {
				t.Fatalf(
					"profile provenance = %v, want %v",
					gotScopes,
					test.wantScopes,
				)
			}
			if len(result.Sources) != len(test.wantScopes) {
				t.Fatalf(
					"resolved sources = %d, want %d",
					len(result.Sources),
					len(test.wantScopes),
				)
			}
		})
	}
}

func TestPortableSourceRejectsMachineSpecificOperationsAndPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document Document
	}{
		{
			name: "windows drive path with backslashes",
			document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{`C:\machine\path`},
				},
			},
		},
		{
			name: "windows drive path with slashes",
			document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{"C:/machine/path"},
				},
			},
		},
		{
			name: "unc search path",
			document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{`\\server\share`},
				},
			},
		},
		{
			name: "parent traversal search path",
			document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{"../outside"},
				},
			},
		},
		{
			name: "absolute unix search path",
			document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{"/machine/path"},
				},
			},
		},
		{
			name: "machine binding value",
			document: Document{
				SchemaVersion: SchemaVersion,
				MachineBindings: map[string]MachineBinding{
					"editor": {Executable: "editor"},
				},
			},
		},
		{
			name: "machine binding reset",
			document: Document{
				SchemaVersion: SchemaVersion,
				Reset:         []string{"machine_bindings"},
			},
		},
		{
			name: "machine binding delete",
			document: Document{
				SchemaVersion: SchemaVersion,
				Delete:        []string{"machine_bindings"},
			},
		},
		{
			name: "machine binding lock",
			document: Document{
				SchemaVersion: SchemaVersion,
				Locks: map[string]string{
					"machine_bindings": "portable machine policy",
				},
			},
		},
		{
			name: "absolute environment file",
			document: Document{
				SchemaVersion: SchemaVersion,
				EnvironmentFiles: []EnvironmentFile{{
					Path:      "/machine/.env",
					Allow:     []string{"TOKEN"},
					GitIgnore: true,
				}},
			},
		},
		{
			name: "environment file not declared gitignored",
			document: Document{
				SchemaVersion: SchemaVersion,
				EnvironmentFiles: []EnvironmentFile{{
					Path:  ".env",
					Allow: []string{"TOKEN"},
				}},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := Resolve(LayerResolveRequest{
				Layers: []Layer{{
					Source: Source{
						Kind:     ScopeWorkspace,
						Name:     "workspace",
						Portable: true,
					},
					Document: test.document,
				}},
			})
			if err == nil {
				t.Fatalf(
					"portable machine-specific configuration was accepted: %#v",
					test.document,
				)
			}
		})
	}
}

func TestResolveRejectsUnrecognizedDuplicateOrSpoofedLayerSources(t *testing.T) {
	t.Parallel()

	validDocument := Document{SchemaVersion: SchemaVersion}
	for _, test := range []struct {
		name   string
		layers []Layer
	}{
		{
			name: "unknown kind",
			layers: []Layer{{
				Source:   Source{Kind: "invented", Name: "invented"},
				Document: validDocument,
			}},
		},
		{
			name: "portable scope marked machine local",
			layers: []Layer{{
				Source: Source{
					Kind: ScopeWorkspace,
					Name: "workspace",
				},
				Document: validDocument,
			}},
		},
		{
			name: "user global marked portable",
			layers: []Layer{{
				Source: Source{
					Kind:     ScopeUserGlobal,
					Name:     "user",
					Portable: true,
				},
				Document: validDocument,
			}},
		},
		{
			name: "duplicate semantic scope",
			layers: []Layer{
				{
					Source: Source{
						Kind:     ScopeWorkspace,
						Name:     "workspace one",
						Portable: true,
					},
					Document: validDocument,
				},
				{
					Source: Source{
						Kind:     ScopeWorkspace,
						Name:     "workspace two",
						Portable: true,
					},
					Document: validDocument,
				},
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Resolve(LayerResolveRequest{
				Layers: test.layers,
			}); err == nil {
				t.Fatal("invalid layer sources were accepted")
			}
		})
	}
}

func TestResolveAppliesSchemaMergeResetAndDeleteStrategies(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{
			{
				Source: Source{
					Kind: ScopeUserGlobal,
					Name: "user",
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Settings: Settings{
						Policy: PolicySettings{
							Conformance: stringPointer("observe"),
							Freshness: FreshnessSettings{
								TTL: stringPointer("12h"),
							},
						},
						Tags:        []string{"user", "shared"},
						SearchPaths: []string{"user"},
						Templates: []TemplateSetting{
							{
								ID:      "ticket",
								Version: "v2",
								Source:  "user_global",
							},
							{
								ID:      "design",
								Version: "v1",
								Source:  "user_global",
							},
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
					Settings: Settings{
						Policy: PolicySettings{
							Freshness: FreshnessSettings{
								Enabled: boolPointer(false),
							},
						},
						Tags:        []string{"shared", "workspace"},
						SearchPaths: []string{"workspace"},
						Templates: []TemplateSetting{{
							ID:     "ticket",
							Source: "workspace",
						}},
					},
				},
			},
			{
				Source: Source{
					Kind:     ScopeTicket,
					Name:     "ticket",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Settings: Settings{
						SearchPaths: []string{"ticket"},
					},
					Reset:  []string{"settings.search_paths"},
					Delete: []string{"settings.policy.freshness.ttl"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve configuration: %v", err)
	}

	settings := result.Config.Settings
	if settings.Policy.Conformance == nil ||
		*settings.Policy.Conformance != "observe" {
		t.Fatalf("conformance = %#v, want observe", settings.Policy.Conformance)
	}
	if settings.Policy.Freshness.Enabled == nil ||
		*settings.Policy.Freshness.Enabled {
		t.Fatalf(
			"freshness enabled = %#v, want false",
			settings.Policy.Freshness.Enabled,
		)
	}
	if settings.Policy.Freshness.TTL != nil {
		t.Fatalf("freshness ttl = %#v, want deleted", settings.Policy.Freshness.TTL)
	}
	if !reflect.DeepEqual(
		settings.Tags,
		[]string{"core", "user", "shared", "workspace"},
	) {
		t.Fatalf("tags = %v", settings.Tags)
	}
	if !reflect.DeepEqual(
		settings.SearchPaths,
		[]string{"docs", "ticket"},
	) {
		t.Fatalf("search paths = %v", settings.SearchPaths)
	}
	wantTemplates := []TemplateSetting{
		{
			ID:      "ticket",
			Version: "v2",
			Source:  "workspace",
			Enabled: boolPointer(true),
		},
		{
			ID:      "design",
			Version: "v1",
			Source:  "user_global",
		},
	}
	if !reflect.DeepEqual(settings.Templates, wantTemplates) {
		t.Fatalf("templates = %#v, want %#v", settings.Templates, wantTemplates)
	}

	operations := make([]string, 0)
	for _, contribution := range result.Provenance["settings.search_paths"] {
		if contribution.Source.Kind == ScopeTicket && contribution.Applied {
			operations = append(operations, contribution.Operation)
		}
	}
	if !reflect.DeepEqual(operations, []string{"reset", "append"}) {
		t.Fatalf("ticket search path operations = %v", operations)
	}
}

func TestResolveEnforcesPolicyLocksByAuthority(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{
			{
				Source: Source{
					Kind: ScopeUserGlobal,
					Name: "user",
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Settings: Settings{
						Policy: PolicySettings{
							Freshness: FreshnessSettings{
								TTL: stringPointer("12h"),
							},
						},
					},
					Locks: map[string]string{
						"settings.policy.freshness.ttl": "user preference",
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
					Settings: Settings{
						Policy: PolicySettings{
							Conformance: stringPointer("block"),
							Freshness: FreshnessSettings{
								TTL: stringPointer("6h"),
							},
						},
					},
					Unlocks: []string{
						"settings.policy.freshness.ttl",
					},
					Locks: map[string]string{
						"settings.policy.conformance": "workspace policy",
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
					Settings: Settings{
						Policy: PolicySettings{
							Conformance: stringPointer("warn"),
						},
					},
					Unlocks: []string{
						"settings.policy.conformance",
					},
				},
			},
		},
		Flags: []Override{{
			Path:  "settings.policy.conformance",
			Value: "off",
		}},
	})
	if err != nil {
		t.Fatalf("resolve configuration: %v", err)
	}

	if result.Config.Settings.Policy.Conformance == nil ||
		*result.Config.Settings.Policy.Conformance != "block" {
		t.Fatalf(
			"conformance = %#v, want workspace block",
			result.Config.Settings.Policy.Conformance,
		)
	}
	if result.Config.Settings.Policy.Freshness.TTL == nil ||
		*result.Config.Settings.Policy.Freshness.TTL != "6h" {
		t.Fatalf(
			"freshness ttl = %#v, want workspace 6h",
			result.Config.Settings.Policy.Freshness.TTL,
		)
	}
	lock, ok := result.Locks["settings.policy.conformance"]
	if !ok || lock.Source.Kind != ScopeWorkspace {
		t.Fatalf("conformance lock = %#v", lock)
	}
	if _, ok := result.Locks["settings.policy.freshness.ttl"]; ok {
		t.Fatalf(
			"freshness ttl remained locked: %#v",
			result.Locks["settings.policy.freshness.ttl"],
		)
	}

	gotCodes := make([]string, 0, len(result.Findings))
	for _, finding := range result.Findings {
		gotCodes = append(gotCodes, finding.ID)
	}
	wantCodes := []string{
		"configuration.unlock_forbidden",
		"configuration.override_locked",
		"configuration.override_locked",
	}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("lock findings = %v, want %v", gotCodes, wantCodes)
	}
}

func TestResolveProtectsChildLocksFromParentResetAndDelete(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"reset", "delete"} {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			t.Parallel()

			repositoryDocument := Document{SchemaVersion: SchemaVersion}
			if operation == "reset" {
				repositoryDocument.Reset = []string{"settings.policy"}
			} else {
				repositoryDocument.Delete = []string{"settings.policy"}
			}
			result, err := Resolve(LayerResolveRequest{
				Layers: []Layer{
					{
						Source: Source{
							Kind:     ScopeWorkspace,
							Name:     "workspace",
							Portable: true,
						},
						Document: Document{
							SchemaVersion: SchemaVersion,
							Settings: Settings{
								Policy: PolicySettings{
									Conformance: stringPointer("block"),
								},
							},
							Locks: map[string]string{
								"settings.policy.conformance": "workspace policy",
							},
						},
					},
					{
						Source: Source{
							Kind:     ScopeRepository,
							Name:     "repository",
							Portable: true,
						},
						Document: repositoryDocument,
					},
				},
			})
			if err != nil {
				t.Fatalf("%s parent policy: %v", operation, err)
			}
			if result.Config.Settings.Policy.Conformance == nil ||
				*result.Config.Settings.Policy.Conformance != "block" {
				t.Fatalf(
					"%s bypassed child lock: %#v",
					operation,
					result.Config.Settings.Policy,
				)
			}
			if !hasFinding(
				result.Findings,
				"configuration.override_locked",
			) {
				t.Fatalf("%s findings = %#v", operation, result.Findings)
			}
		})
	}
}

func TestResolveRecordsRejectedLockReplacementInProvenance(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{
			{
				Source: Source{
					Kind:     ScopeWorkspace,
					Name:     "workspace",
					Portable: true,
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					Locks: map[string]string{
						"settings.profile": "workspace policy",
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
						"settings.profile": "repository policy",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve rejected lock replacement: %v", err)
	}
	contributions := result.Provenance["settings.profile"]
	last := contributions[len(contributions)-1]
	if last.Source.Kind != ScopeRepository ||
		last.Operation != "lock" ||
		last.Applied ||
		last.Reason == "" {
		t.Fatalf("rejected lock provenance = %#v", contributions)
	}
}

func TestResolveSeparatesPortableMachineAndSecretConfiguration(t *testing.T) {
	t.Parallel()

	result, err := Resolve(LayerResolveRequest{
		Layers: []Layer{
			{
				Source: Source{
					Kind: ScopeUserGlobal,
					Name: "user",
				},
				Document: Document{
					SchemaVersion: SchemaVersion,
					MachineBindings: map[string]MachineBinding{
						"editor": {
							Path:       "/Applications/Editor.app",
							Executable: "editor",
						},
					},
					SecretReferences: map[string]SecretReference{
						"github": {
							Provider: "environment",
							Key:      "GITHUB_TOKEN",
						},
					},
					Plugins: map[string]any{
						"acme.example": map[string]any{
							"secret_reference": "github",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve machine and secret metadata: %v", err)
	}
	if result.Config.MachineBindings["editor"].Executable != "editor" {
		t.Fatalf("machine bindings = %#v", result.Config.MachineBindings)
	}
	if result.Config.SecretReferences["github"].Key != "GITHUB_TOKEN" {
		t.Fatalf("secret references = %#v", result.Config.SecretReferences)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal resolution: %v", err)
	}
	if strings.Contains(string(raw), "top-secret-value") {
		t.Fatalf("resolution leaked secret value: %s", raw)
	}

	_, err = Resolve(LayerResolveRequest{
		Layers: []Layer{{
			Source: Source{
				Kind:     ScopeWorkspace,
				Name:     "workspace",
				Portable: true,
			},
			Document: Document{
				SchemaVersion: SchemaVersion,
				Settings: Settings{
					SearchPaths: []string{"/machine/local/path"},
				},
				MachineBindings: map[string]MachineBinding{
					"editor": {Path: "/Applications/Editor.app"},
				},
			},
		}},
	})
	if err == nil {
		t.Fatal("portable machine configuration was accepted")
	}

	const secret = "top-secret-value"
	_, err = Resolve(LayerResolveRequest{
		Environment: []Override{{
			Name:   "GITHUB_TOKEN",
			Path:   "settings.profile",
			Value:  secret,
			Secret: true,
		}},
	})
	if err == nil {
		t.Fatal("secret environment value was accepted as ordinary configuration")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("environment error leaked secret value: %v", err)
	}

	_, err = Resolve(LayerResolveRequest{
		Environment: []Override{{
			Name:  "ADB_SEARCH_PATHS",
			Path:  "settings.search_paths",
			Value: []string{"ambient"},
		}},
	})
	if err == nil {
		t.Fatal("non-allowlisted environment override was accepted")
	}
}

func TestResolveSupportsResetAndDeleteForEveryDeclaredField(t *testing.T) {
	t.Parallel()

	for _, specification := range FieldSpecs() {
		specification := specification
		for _, operation := range []string{"reset", "delete"} {
			operation := operation
			t.Run(operation+"/"+specification.Path, func(t *testing.T) {
				t.Parallel()

				document := Document{SchemaVersion: SchemaVersion}
				if operation == "reset" {
					document.Reset = []string{specification.Path}
				} else {
					document.Delete = []string{specification.Path}
				}
				result, err := Resolve(LayerResolveRequest{
					Layers: []Layer{{
						Source: Source{
							Kind: ScopeUserGlobal,
							Name: "user",
						},
						Document: document,
					}},
				})
				if err != nil {
					t.Fatalf("%s %s: %v", operation, specification.Path, err)
				}
				contributions := result.Provenance[specification.Path]
				if len(contributions) == 0 ||
					contributions[len(contributions)-1].Operation != operation {
					t.Fatalf(
						"%s provenance for %s = %#v",
						operation,
						specification.Path,
						contributions,
					)
				}
			})
		}
	}
}

func stringPointer(value string) *string {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}
