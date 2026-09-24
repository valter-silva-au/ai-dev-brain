package configuration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

func TestServiceResolvesExplainsValidatesAndListsSources(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	writeConfiguration(t, paths.UserGlobalPath, `
schema_version: aidb.config/v1
settings:
  profile: user@v1
machine_bindings:
  editor:
    executable: editor
secret_references:
  github:
    provider: environment
    key: GITHUB_TOKEN
    inherit: true
plugins:
  acme.example:
    arbitrary:
      nested: accepted
`)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
settings:
  profile: workspace@v1
  tags: [workspace]
`)
	writeConfiguration(t, configPath(paths.RepositoryRoot), `
schema_version: aidb.config/v1
settings:
  profile: repository@v1
  tags: [repository]
`)
	before := snapshotConfigurationTree(t, filepath.Dir(paths.WorkspaceRoot))

	request := ResolveRequest{
		Scopes: paths,
		Environment: []Override{{
			Name:  "ADB_PROFILE",
			Path:  "settings.profile",
			Value: "environment@v1",
		}},
		Flags: []Override{{
			Name:  "profile",
			Path:  "settings.profile",
			Value: "request@v1",
		}},
	}
	service := NewService()
	resolved, err := service.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("resolve service: %v", err)
	}
	if resolved.Capability != ResolveDescriptor.Capability ||
		resolved.Version != "v1" ||
		resolved.Outcome != capability.OutcomeHealthy {
		t.Fatalf("resolve envelope = %#v", resolved)
	}
	if !resolved.Data.Complete {
		t.Fatalf("resolve unexpectedly incomplete: %#v", resolved.Data.Findings)
	}
	if resolved.Data.Config == nil ||
		resolved.Data.Config.Settings.Profile == nil ||
		*resolved.Data.Config.Settings.Profile != "request@v1" {
		t.Fatalf("resolved config = %#v", resolved.Data.Config)
	}
	gotSourceKinds := make([]ScopeKind, 0, len(resolved.Data.Sources))
	for _, status := range resolved.Data.Sources {
		gotSourceKinds = append(gotSourceKinds, status.Source.Kind)
	}
	wantSourceKinds := []ScopeKind{
		ScopeBuiltin,
		ScopeUserGlobal,
		ScopeWorkspace,
		ScopeOrganization,
		ScopeHost,
		ScopeOwner,
		ScopeRepository,
		ScopeTicket,
		ScopeEnvironment,
		ScopeRequest,
	}
	if !reflect.DeepEqual(gotSourceKinds, wantSourceKinds) {
		t.Fatalf("source order = %v, want %v", gotSourceKinds, wantSourceKinds)
	}

	explained, err := service.Explain(context.Background(), ExplainRequest{
		Scopes:      request.Scopes,
		Environment: request.Environment,
		Flags:       request.Flags,
		Path:        "settings.profile",
	})
	if err != nil {
		t.Fatalf("explain service: %v", err)
	}
	if explained.Data.Value != "request@v1" {
		t.Fatalf("explained value = %#v", explained.Data.Value)
	}
	gotContributors := make([]ScopeKind, 0, len(explained.Data.Contributions))
	for _, contribution := range explained.Data.Contributions {
		if contribution.Applied {
			gotContributors = append(gotContributors, contribution.Source.Kind)
		}
	}
	wantContributors := []ScopeKind{
		ScopeBuiltin,
		ScopeUserGlobal,
		ScopeWorkspace,
		ScopeRepository,
		ScopeEnvironment,
		ScopeRequest,
	}
	if !reflect.DeepEqual(gotContributors, wantContributors) {
		t.Fatalf(
			"explain contributors = %v, want %v",
			gotContributors,
			wantContributors,
		)
	}
	pluginExplanation, err := service.Explain(
		context.Background(),
		ExplainRequest{
			Scopes: request.Scopes,
			Path:   "plugins.acme.example.arbitrary.nested",
		},
	)
	if err != nil {
		t.Fatalf("explain nested plugin setting: %v", err)
	}
	if pluginExplanation.Data.Value != "accepted" {
		t.Fatalf("plugin explanation = %#v", pluginExplanation)
	}

	validated, err := service.Validate(
		context.Background(),
		ValidateRequest(request),
	)
	if err != nil {
		t.Fatalf("validate service: %v", err)
	}
	if !validated.Data.Valid ||
		validated.Outcome != capability.OutcomeHealthy {
		t.Fatalf("validation = %#v", validated)
	}

	sources, err := service.Sources(
		context.Background(),
		SourcesRequest{Scopes: request.Scopes},
	)
	if err != nil {
		t.Fatalf("sources service: %v", err)
	}
	if sources.Outcome != capability.OutcomeHealthy ||
		len(sources.Data.Sources) != len(wantSourceKinds)-2 {
		t.Fatalf("sources result = %#v", sources)
	}
	after := snapshotConfigurationTree(t, filepath.Dir(paths.WorkspaceRoot))
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"configuration services mutated their sources\nbefore=%#v\nafter=%#v",
			before,
			after,
		)
	}
}

func TestServiceReportsInvalidSourcesWithoutLeakingContent(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	const secret = "top-secret-value"
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
plugins:
  acme.example:
    api_token: `+secret+`
`)

	service := NewService()
	resolved, err := service.Resolve(
		context.Background(),
		ResolveRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("resolve invalid source: %v", err)
	}
	if resolved.Outcome != capability.OutcomeAttention ||
		resolved.Data.Complete ||
		resolved.Data.Config != nil ||
		!hasFinding(resolved.Data.Findings, "configuration.source.invalid") {
		t.Fatalf("invalid resolve result = %#v", resolved)
	}

	validated, err := service.Validate(
		context.Background(),
		ValidateRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("validate invalid source: %v", err)
	}
	if validated.Data.Valid ||
		validated.Outcome != capability.OutcomeAttention ||
		!hasFinding(validated.Data.Findings, "configuration.source.invalid") {
		t.Fatalf("invalid validation result = %#v", validated)
	}

	sources, err := service.Sources(
		context.Background(),
		SourcesRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("list invalid source: %v", err)
	}
	if sources.Outcome != capability.OutcomeAttention ||
		!hasFinding(sources.Data.Findings, "configuration.source.invalid") {
		t.Fatalf("invalid sources result = %#v", sources)
	}

	explained, err := service.Explain(context.Background(), ExplainRequest{
		Scopes: paths,
		Path:   "settings.profile",
	})
	if err != nil {
		t.Fatalf("explain invalid source: %v", err)
	}
	if explained.Outcome != capability.OutcomeAttention ||
		explained.Data.Complete {
		t.Fatalf("invalid explain result = %#v", explained)
	}

	results := []struct {
		name   string
		result any
	}{
		{name: "resolve", result: resolved},
		{name: "validate", result: validated},
		{name: "sources", result: sources},
		{name: "explain", result: explained},
	}
	for _, test := range results {
		test := test
		t.Run(test.name+" response redacts source content", func(t *testing.T) {
			raw, marshalErr := json.Marshal(test.result)
			if marshalErr != nil {
				t.Fatalf("marshal result: %v", marshalErr)
			}
			if strings.Contains(string(raw), secret) {
				t.Fatalf("result leaked source content: %s", raw)
			}
		})
	}

	_, err = service.Resolve(context.Background(), ResolveRequest{
		Scopes: paths,
		Environment: []Override{{
			Name:  "ADB_CONFORMANCE",
			Path:  "settings.policy.conformance",
			Value: secret,
		}},
	})
	if err == nil {
		t.Fatal("invalid environment policy was accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("environment validation leaked value: %v", err)
	}
}

func TestServiceReturnsTrustworthyLockedResolutionWithAttention(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	writeConfiguration(t, configPath(paths.WorkspaceRoot), `
schema_version: aidb.config/v1
settings:
  policy:
    conformance: block
locks:
  settings.policy.conformance: workspace policy
`)
	writeConfiguration(t, configPath(paths.RepositoryRoot), `
schema_version: aidb.config/v1
settings:
  policy:
    conformance: warn
`)

	service := NewService()
	resolved, err := service.Resolve(
		context.Background(),
		ResolveRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("resolve locked configuration: %v", err)
	}
	if resolved.Outcome != capability.OutcomeAttention ||
		!resolved.Data.Complete ||
		resolved.Data.Config == nil ||
		resolved.Data.Config.Settings.Policy.Conformance == nil ||
		*resolved.Data.Config.Settings.Policy.Conformance != "block" {
		t.Fatalf("locked resolution = %#v", resolved)
	}

	validated, err := service.Validate(
		context.Background(),
		ValidateRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("validate locked configuration: %v", err)
	}
	if validated.Data.Valid ||
		!hasFinding(
			validated.Data.Findings,
			"configuration.override_locked",
		) {
		t.Fatalf("locked validation = %#v", validated)
	}
}

func TestServiceRejectsConfigurationSourceSymlinkEscape(t *testing.T) {
	t.Parallel()

	paths := createScopePaths(t)
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	writeConfiguration(t, outside, `
schema_version: aidb.config/v1
settings:
  profile: escaped@v1
`)
	workspaceConfig := configPath(paths.WorkspaceRoot)
	if err := os.MkdirAll(filepath.Dir(workspaceConfig), 0o755); err != nil {
		t.Fatalf("create workspace config directory: %v", err)
	}
	if err := os.Symlink(outside, workspaceConfig); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	result, err := NewService().Sources(
		context.Background(),
		SourcesRequest{Scopes: paths},
	)
	if err != nil {
		t.Fatalf("list escaped source: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention ||
		!hasFinding(
			result.Data.Findings,
			"configuration.source.outside_scope",
		) {
		t.Fatalf("escaped source result = %#v", result)
	}
}

func TestConfigurationCapabilityDescriptorsAreReadOnlyV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		descriptor capability.Descriptor
		capability string
		command    string
		tool       string
	}{
		{
			name:       "resolve",
			descriptor: ResolveDescriptor,
			capability: "configuration.resolve",
			command:    "show",
			tool:       "adb_configuration_resolve",
		},
		{
			name:       "explain",
			descriptor: ExplainDescriptor,
			capability: "configuration.explain",
			command:    "explain",
			tool:       "adb_configuration_explain",
		},
		{
			name:       "validate",
			descriptor: ValidateDescriptor,
			capability: "configuration.validate",
			command:    "validate",
			tool:       "adb_configuration_validate",
		},
		{
			name:       "sources",
			descriptor: SourcesDescriptor,
			capability: "configuration.sources",
			command:    "sources",
			tool:       "adb_configuration_sources",
		},
	}
	seen := map[string]struct{}{}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			descriptor := test.descriptor
			if descriptor.Version != "v1" {
				t.Errorf(
					"%s version = %q",
					descriptor.Capability,
					descriptor.Version,
				)
			}
			if descriptor.Mutating {
				t.Errorf("%s unexpectedly mutates", descriptor.Capability)
			}
			if descriptor.Capability != test.capability ||
				descriptor.Command != test.command ||
				descriptor.Tool != test.tool ||
				descriptor.Summary == "" {
				t.Errorf("descriptor = %#v, contract = %#v", descriptor, test)
			}
			if _, ok := seen[descriptor.Capability]; ok {
				t.Errorf("duplicate capability %q", descriptor.Capability)
			}
			seen[descriptor.Capability] = struct{}{}
		})
	}
}

func createScopePaths(t *testing.T) ScopePaths {
	t.Helper()

	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "workspace")
	organizationRoot := filepath.Join(workspaceRoot, "organizations", "amazon")
	hostRoot := filepath.Join(organizationRoot, "repos", "github.com")
	ownerRoot := filepath.Join(hostRoot, "valter-silva-au")
	repositoryRoot := filepath.Join(ownerRoot, "ai-dev-brain")
	ticketRoot := filepath.Join(repositoryRoot, "tickets", "TASK-00039")
	for _, directory := range []string{
		workspaceRoot,
		organizationRoot,
		hostRoot,
		ownerRoot,
		repositoryRoot,
		ticketRoot,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create scope directory %q: %v", directory, err)
		}
	}
	return ScopePaths{
		WorkspaceRoot:    workspaceRoot,
		UserGlobalPath:   filepath.Join(base, "user-config.yaml"),
		OrganizationRoot: organizationRoot,
		HostRoot:         hostRoot,
		OwnerRoot:        ownerRoot,
		RepositoryRoot:   repositoryRoot,
		TicketRoot:       ticketRoot,
	}
}

func writeConfiguration(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create configuration directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write configuration %q: %v", path, err)
	}
}

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.ID == code {
			return true
		}
	}
	return false
}

type configurationSnapshot struct {
	Mode    os.FileMode
	Content string
	Target  string
}

func snapshotConfigurationTree(
	t *testing.T,
	root string,
) map[string]configurationSnapshot {
	t.Helper()

	result := map[string]configurationSnapshot{}
	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			snapshot := configurationSnapshot{Mode: info.Mode()}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				snapshot.Target, err = os.Readlink(path)
			case info.Mode().IsRegular():
				var content []byte
				content, err = os.ReadFile(path)
				snapshot.Content = string(content)
			}
			if err != nil {
				return err
			}
			result[relative] = snapshot
			return nil
		},
	)
	if err != nil {
		t.Fatalf("snapshot configuration tree: %v", err)
	}
	return result
}
