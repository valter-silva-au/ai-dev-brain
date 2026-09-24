package configuration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

var (
	ResolveDescriptor = capability.Descriptor{
		Capability: "configuration.resolve",
		Version:    "v1",
		Command:    "show",
		Tool:       "adb_configuration_resolve",
		Summary:    "Resolve effective configuration with provenance.",
		Mutating:   false,
	}
	ExplainDescriptor = capability.Descriptor{
		Capability: "configuration.explain",
		Version:    "v1",
		Command:    "explain",
		Tool:       "adb_configuration_explain",
		Summary:    "Explain the effective value and provenance of one field.",
		Mutating:   false,
	}
	ValidateDescriptor = capability.Descriptor{
		Capability: "configuration.validate",
		Version:    "v1",
		Command:    "validate",
		Tool:       "adb_configuration_validate",
		Summary:    "Validate recognized configuration sources without mutation.",
		Mutating:   false,
	}
	SourcesDescriptor = capability.Descriptor{
		Capability: "configuration.sources",
		Version:    "v1",
		Command:    "sources",
		Tool:       "adb_configuration_sources",
		Summary:    "List recognized configuration sources and their state.",
		Mutating:   false,
	}
)

type SourceState string

const (
	SourceStateEmbedded SourceState = "embedded"
	SourceStateMissing  SourceState = "missing"
	SourceStateValid    SourceState = "valid"
	SourceStateInvalid  SourceState = "invalid"
	SourceStateProvided SourceState = "provided"
)

type ResolveRequest struct {
	Scopes      ScopePaths `json:"scopes" yaml:"scopes"`
	Environment []Override `json:"environment,omitempty" yaml:"environment,omitempty"`
	Flags       []Override `json:"flags,omitempty" yaml:"flags,omitempty"`
}

type ExplainRequest struct {
	Scopes      ScopePaths `json:"scopes" yaml:"scopes"`
	Environment []Override `json:"environment,omitempty" yaml:"environment,omitempty"`
	Flags       []Override `json:"flags,omitempty" yaml:"flags,omitempty"`
	Path        string     `json:"path" yaml:"path"`
}

type ValidateRequest struct {
	Scopes      ScopePaths `json:"scopes" yaml:"scopes"`
	Environment []Override `json:"environment,omitempty" yaml:"environment,omitempty"`
	Flags       []Override `json:"flags,omitempty" yaml:"flags,omitempty"`
}

type SourcesRequest struct {
	Scopes ScopePaths `json:"scopes" yaml:"scopes"`
}

type SourceStatus struct {
	Source Source      `json:"source" yaml:"source"`
	State  SourceState `json:"state" yaml:"state"`
}

type ResolveData struct {
	Complete   bool                      `json:"complete" yaml:"complete"`
	Config     *Config                   `json:"config" yaml:"config"`
	Provenance map[string][]Contribution `json:"provenance" yaml:"provenance"`
	Findings   []Finding                 `json:"findings" yaml:"findings"`
	Sources    []SourceStatus            `json:"sources" yaml:"sources"`
	Locks      map[string]Lock           `json:"locks" yaml:"locks"`
}

type ExplainData struct {
	Complete      bool           `json:"complete" yaml:"complete"`
	Path          string         `json:"path" yaml:"path"`
	Strategy      MergeStrategy  `json:"strategy" yaml:"strategy"`
	Value         any            `json:"value" yaml:"value"`
	Contributions []Contribution `json:"contributions" yaml:"contributions"`
	Lock          *Lock          `json:"lock,omitempty" yaml:"lock,omitempty"`
	Findings      []Finding      `json:"findings" yaml:"findings"`
	Sources       []SourceStatus `json:"sources" yaml:"sources"`
}

type ValidateData struct {
	Valid    bool           `json:"valid" yaml:"valid"`
	Findings []Finding      `json:"findings" yaml:"findings"`
	Sources  []SourceStatus `json:"sources" yaml:"sources"`
}

type SourcesData struct {
	Findings []Finding      `json:"findings" yaml:"findings"`
	Sources  []SourceStatus `json:"sources" yaml:"sources"`
}

type Service struct{}

type evaluation struct {
	resolution Resolution
	statuses   []SourceStatus
	findings   []Finding
	complete   bool
}

type query struct {
	scopes      ScopePaths
	environment []Override
	flags       []Override
}

const maxConfigurationBytes int64 = 1 << 20

func NewService() *Service {
	return &Service{}
}

func (service *Service) Resolve(
	ctx context.Context,
	request ResolveRequest,
) (capability.Result[ResolveData], error) {
	evaluated, err := service.evaluate(ctx, query{
		scopes:      request.Scopes,
		environment: request.Environment,
		flags:       request.Flags,
	})
	if err != nil {
		return emptyResolveResult(), err
	}
	var config *Config
	provenance := map[string][]Contribution{}
	locks := map[string]Lock{}
	if evaluated.complete {
		value := evaluated.resolution.Config
		config = &value
		provenance = evaluated.resolution.Provenance
		locks = evaluated.resolution.Locks
	}
	return readOnlyResult(
		ResolveDescriptor,
		ResolveData{
			Complete:   evaluated.complete,
			Config:     config,
			Provenance: provenance,
			Findings:   evaluated.findings,
			Sources:    evaluated.statuses,
			Locks:      locks,
		},
		evaluated.findings,
	), nil
}

func (service *Service) Explain(
	ctx context.Context,
	request ExplainRequest,
) (capability.Result[ExplainData], error) {
	if strings.TrimSpace(request.Path) == "" {
		return emptyExplainResult(), errors.New(
			"configuration field path is required",
		)
	}
	evaluated, err := service.evaluate(ctx, query{
		scopes:      request.Scopes,
		environment: request.Environment,
		flags:       request.Flags,
	})
	if err != nil {
		return emptyExplainResult(), err
	}
	var value any
	strategy, strategyKnown := strategyForPath(request.Path)
	contributions := []Contribution{}
	var lock *Lock
	valueFound := false
	if evaluated.complete {
		value, valueFound = explainValue(
			evaluated.resolution.Config,
			request.Path,
		)
		contributions = contributionsForPath(
			evaluated.resolution.Provenance,
			request.Path,
		)
		lock = matchingLock(evaluated.resolution.Locks, request.Path)
	}
	_, exactProvenance := evaluated.resolution.Provenance[request.Path]
	known := strategyKnown &&
		(knownFieldPath(request.Path) || valueFound || exactProvenance)
	if evaluated.complete && !known {
		evaluated.findings = append(evaluated.findings, Finding{
			ID:       "configuration.field.unknown",
			Severity: "error",
			Summary:  "The requested configuration field is not defined.",
			Path:     request.Path,
			Source: sourcePointer(Source{
				Kind: ScopeRequest,
				Name: "explain request",
			}),
			Evidence:    []string{request.Path},
			Remediation: "Choose a field declared by aidb.config/v1.",
		})
		evaluated.complete = false
		value = nil
		contributions = []Contribution{}
		lock = nil
	}
	return readOnlyResult(
		ExplainDescriptor,
		ExplainData{
			Complete:      evaluated.complete,
			Path:          request.Path,
			Strategy:      strategy,
			Value:         value,
			Contributions: contributions,
			Lock:          lock,
			Findings:      evaluated.findings,
			Sources:       evaluated.statuses,
		},
		evaluated.findings,
	), nil
}

func (service *Service) Validate(
	ctx context.Context,
	request ValidateRequest,
) (capability.Result[ValidateData], error) {
	evaluated, err := service.evaluate(ctx, query{
		scopes:      request.Scopes,
		environment: request.Environment,
		flags:       request.Flags,
	})
	if err != nil {
		return emptyValidateResult(), err
	}
	valid := !hasErrorFinding(evaluated.findings)
	return readOnlyResult(
		ValidateDescriptor,
		ValidateData{
			Valid:    valid,
			Findings: evaluated.findings,
			Sources:  evaluated.statuses,
		},
		evaluated.findings,
	), nil
}

func (service *Service) Sources(
	ctx context.Context,
	request SourcesRequest,
) (capability.Result[SourcesData], error) {
	statuses, _, findings, err := loadQuery(ctx, query{
		scopes: request.Scopes,
	})
	if err != nil {
		return emptySourcesResult(), err
	}
	return readOnlyResult(
		SourcesDescriptor,
		SourcesData{
			Findings: findings,
			Sources:  statuses,
		},
		findings,
	), nil
}

func (service *Service) evaluate(
	ctx context.Context,
	request query,
) (evaluation, error) {
	statuses, layers, findings, err := loadQuery(ctx, request)
	if err != nil {
		return evaluation{}, err
	}
	complete := !hasErrorFinding(findings)
	resolved, err := Resolve(LayerResolveRequest{
		Layers:      layers,
		Environment: request.environment,
		Flags:       request.flags,
		Target:      targetScope(request.scopes),
	})
	if err != nil {
		return evaluation{}, err
	}
	findings = append(findings, resolved.Findings...)
	for _, finding := range resolved.Findings {
		if findingInvalidatesResolution(finding) {
			complete = false
		}
	}
	return evaluation{
		resolution: resolved,
		statuses:   statuses,
		findings:   findings,
		complete:   complete,
	}, nil
}

func loadQuery(
	ctx context.Context,
	request query,
) ([]SourceStatus, []Layer, []Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	sources, err := DiscoverSources(request.scopes)
	if err != nil {
		return nil, nil, nil, err
	}
	statuses := []SourceStatus{{
		Source: Source{
			Kind: ScopeBuiltin,
			Name: "built-in defaults",
		},
		State: SourceStateEmbedded,
	}}
	layers := make([]Layer, 0, len(sources))
	findings := make([]Finding, 0)
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		status, layer, finding := loadSource(source)
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		statuses = append(statuses, status)
		if layer != nil {
			layers = append(layers, *layer)
		}
		if finding != nil {
			findings = append(findings, *finding)
		}
	}
	if len(request.environment) > 0 {
		statuses = append(statuses, SourceStatus{
			Source: Source{
				Kind: ScopeEnvironment,
				Name: "allowlisted environment",
			},
			State: SourceStateProvided,
		})
	}
	if len(request.flags) > 0 {
		statuses = append(statuses, SourceStatus{
			Source: Source{
				Kind: ScopeRequest,
				Name: "explicit request flags",
			},
			State: SourceStateProvided,
		})
	}
	return statuses, layers, findings, nil
}

func loadSource(source Source) (SourceStatus, *Layer, *Finding) {
	status := SourceStatus{
		Source: source,
		State:  SourceStateMissing,
	}
	_, err := os.Lstat(source.Path)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil, nil
	}
	if err != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source cannot be inspected.",
		)
	}
	readPath, resolveErr := filepath.EvalSymlinks(source.Path)
	if resolveErr != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source cannot be resolved.",
		)
	}
	if source.Portable {
		scopeRoot := filepath.Dir(filepath.Dir(source.Path))
		if !pathContained(scopeRoot, readPath) {
			return invalidSource(
				status,
				"configuration.source.outside_scope",
				"The configuration source resolves outside its semantic scope.",
			)
		}
	}
	info, err := os.Stat(readPath)
	if err != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source target cannot be inspected.",
		)
	}
	if !info.Mode().IsRegular() {
		return invalidSource(
			status,
			"configuration.source.invalid_type",
			"The configuration source is not a regular file.",
		)
	}
	if info.Size() > maxConfigurationBytes {
		return invalidSource(
			status,
			"configuration.source.too_large",
			"The configuration source exceeds the supported size limit.",
		)
	}
	file, err := openConfigurationFile(readPath)
	if err != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source cannot be read.",
		)
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return invalidSource(
			status,
			"configuration.source.invalid_type",
			"The opened configuration source is not a regular file.",
		)
	}
	if openedInfo.Size() > maxConfigurationBytes {
		_ = file.Close()
		return invalidSource(
			status,
			"configuration.source.too_large",
			"The configuration source exceeds the supported size limit.",
		)
	}
	resolvedAfterOpen, resolveAfterErr := filepath.EvalSymlinks(readPath)
	var currentInfo os.FileInfo
	if resolveAfterErr == nil {
		currentInfo, err = os.Stat(resolvedAfterOpen)
	}
	if resolveAfterErr != nil ||
		err != nil ||
		source.Portable &&
			!pathContained(
				filepath.Dir(filepath.Dir(source.Path)),
				resolvedAfterOpen,
			) ||
		!os.SameFile(openedInfo, currentInfo) {
		_ = file.Close()
		return invalidSource(
			status,
			"configuration.source.changed",
			"The configuration source changed while it was being inspected.",
		)
	}
	content, readErr := io.ReadAll(
		io.LimitReader(file, maxConfigurationBytes+1),
	)
	closeErr := file.Close()
	if readErr != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source cannot be read.",
		)
	}
	if int64(len(content)) > maxConfigurationBytes {
		return invalidSource(
			status,
			"configuration.source.too_large",
			"The configuration source exceeds the supported size limit.",
		)
	}
	document, decodeErr := DecodeDocument(bytes.NewReader(content))
	if decodeErr != nil {
		return invalidSource(
			status,
			"configuration.source.invalid",
			"The configuration source does not satisfy aidb.config/v1.",
		)
	}
	if closeErr != nil {
		return invalidSource(
			status,
			"configuration.source.unreadable",
			"The configuration source could not be closed safely.",
		)
	}
	if err := validateSourceDocument(source, document); err != nil {
		return invalidSource(
			status,
			"configuration.source.invalid",
			"The configuration source is invalid for its semantic scope.",
		)
	}
	if err := verifyPortableEnvironmentFiles(source, document); err != nil {
		return invalidSource(
			status,
			"configuration.environment_file.not_ignored",
			"A portable environment file is not ignored by Git.",
		)
	}
	status.State = SourceStateValid
	return status, &Layer{Source: source, Document: document}, nil
}

func verifyPortableEnvironmentFiles(
	source Source,
	document Document,
) error {
	if !source.Portable {
		return nil
	}
	scopeRoot := filepath.Dir(filepath.Dir(source.Path))
	for _, environmentFile := range document.EnvironmentFiles {
		// The declared path is attacker-supplied — a portable config document
		// travels with a clone — but it cannot reach a shell or git's option
		// parser: the program name is constant, every argument is a separate
		// argv element, and the path is passed after "--". validateSourceDocument
		// has also already rejected any path that is absolute, drive-qualified,
		// or escapes the scope root (portableRelativePath), and it runs before
		// this function on the same document.
		//nolint:gosec // G204: fixed argv, path validated and passed after "--".
		command := exec.Command(
			"git",
			"-c",
			"core.excludesFile="+os.DevNull,
			"-C",
			scopeRoot,
			"check-ignore",
			"--no-index",
			"--quiet",
			"--",
			filepath.FromSlash(environmentFile.Path),
		)
		if err := command.Run(); err != nil {
			return err
		}
	}
	return nil
}

func invalidSource(
	status SourceStatus,
	code string,
	message string,
) (SourceStatus, *Layer, *Finding) {
	status.State = SourceStateInvalid
	return status, nil, &Finding{
		ID:          code,
		Severity:    "error",
		Summary:     message,
		Path:        status.Source.Path,
		Source:      sourcePointer(status.Source),
		Evidence:    []string{status.Source.Path},
		Remediation: "Repair or remove the invalid source before relying on resolved configuration.",
	}
}

func pathContained(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func explainValue(config Config, path string) (any, bool) {
	switch path {
	case "settings.profile":
		return dereferenceString(config.Settings.Profile), true
	case "settings.policy":
		return config.Settings.Policy, true
	case "settings.policy.conformance":
		return dereferenceString(config.Settings.Policy.Conformance), true
	case "settings.policy.freshness":
		return config.Settings.Policy.Freshness, true
	case "settings.policy.freshness.enabled":
		return dereferenceBool(config.Settings.Policy.Freshness.Enabled), true
	case "settings.policy.freshness.ttl":
		return dereferenceString(config.Settings.Policy.Freshness.TTL), true
	case "settings.tags":
		return append([]string(nil), config.Settings.Tags...), true
	case "settings.search_paths":
		return append([]string(nil), config.Settings.SearchPaths...), true
	case "settings.templates":
		return cloneTemplates(config.Settings.Templates), true
	case "plugins":
		return cloneMap(config.Plugins), true
	case "secret_references":
		return cloneSecretReferences(config.SecretReferences), true
	case "environment_files":
		return cloneEnvironmentFiles(config.EnvironmentFiles), true
	case "machine_bindings":
		return cloneMachineBindings(config.MachineBindings), true
	}
	if strings.HasPrefix(path, "settings.templates.") {
		return findTemplate(
			config.Settings.Templates,
			strings.TrimPrefix(path, "settings.templates."),
		)
	}
	if value, ok := explainPluginValue(config.Plugins, path); ok {
		return value, true
	}
	for name, reference := range config.SecretReferences {
		if path == "secret_references."+name {
			return reference, true
		}
	}
	for name, binding := range config.MachineBindings {
		if path == "machine_bindings."+name {
			return binding, true
		}
	}
	if strings.HasPrefix(path, "environment_files.") {
		return findEnvironmentFile(
			config.EnvironmentFiles,
			strings.TrimPrefix(path, "environment_files."),
		)
	}
	return nil, false
}

func explainPluginValue(plugins map[string]any, path string) (any, bool) {
	namespace := ""
	for candidate := range plugins {
		prefix := "plugins." + candidate
		if path != prefix && !strings.HasPrefix(path, prefix+".") {
			continue
		}
		if len(candidate) > len(namespace) {
			namespace = candidate
		}
	}
	if namespace == "" {
		return nil, false
	}
	value := plugins[namespace]
	remainder := strings.TrimPrefix(path, "plugins."+namespace)
	if remainder == "" {
		return cloneValue(value), true
	}
	remainder = strings.TrimPrefix(remainder, ".")
	for _, segment := range strings.Split(remainder, ".") {
		mapping, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = mapping[segment]
		if !ok {
			return nil, false
		}
	}
	return cloneValue(value), true
}

func strategyForPath(path string) (MergeStrategy, bool) {
	for _, specification := range FieldSpecs() {
		if specification.Path == path {
			return specification.Strategy, true
		}
	}
	if base, _, ok := keyedFieldPath(path); ok {
		for _, specification := range FieldSpecs() {
			if specification.Path == base {
				return specification.Strategy, true
			}
		}
	}
	if strings.HasPrefix(path, "plugins.") {
		return MergeDeepMap, true
	}
	return "", false
}

func contributionsForPath(
	provenance map[string][]Contribution,
	path string,
) []Contribution {
	result := make([]Contribution, 0)
	for contributionPath, contributions := range provenance {
		if path == contributionPath ||
			strings.HasPrefix(path, contributionPath+".") ||
			strings.HasPrefix(contributionPath, path+".") {
			result = append(result, contributions...)
		}
	}
	sort.SliceStable(result, func(left int, right int) bool {
		leftContribution := result[left]
		rightContribution := result[right]
		leftPrecedence := scopePrecedence(leftContribution.Source.Kind)
		rightPrecedence := scopePrecedence(rightContribution.Source.Kind)
		if leftPrecedence != rightPrecedence {
			return leftPrecedence < rightPrecedence
		}
		if leftContribution.Source.Path != rightContribution.Source.Path {
			return leftContribution.Source.Path <
				rightContribution.Source.Path
		}
		if leftContribution.Source.Name != rightContribution.Source.Name {
			return leftContribution.Source.Name <
				rightContribution.Source.Name
		}
		return leftContribution.Path < rightContribution.Path
	})
	return result
}

func matchingLock(locks map[string]Lock, path string) *Lock {
	var matched Lock
	found := false
	for lockPath, lock := range locks {
		if !relatedFieldPaths(path, lockPath) {
			continue
		}
		if !found ||
			lock.Authority > matched.Authority ||
			lock.Authority == matched.Authority &&
				len(lockPath) > len(matched.Path) {
			matched = lock
			found = true
		}
	}
	if !found {
		return nil
	}
	return &matched
}

func targetScope(scopes ScopePaths) ScopeKind {
	switch {
	case scopes.TicketRoot != "":
		return ScopeTicket
	case scopes.RepositoryRoot != "":
		return ScopeRepository
	case scopes.OwnerRoot != "":
		return ScopeOwner
	case scopes.HostRoot != "":
		return ScopeHost
	case scopes.OrganizationRoot != "":
		return ScopeOrganization
	default:
		return ScopeWorkspace
	}
}

func findingInvalidatesResolution(finding Finding) bool {
	return finding.ID == "configuration.secret_reference.missing"
}

func dereferenceString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func dereferenceBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func hasErrorFinding(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == "error" {
			return true
		}
	}
	return false
}

func readOnlyResult[T any](
	descriptor capability.Descriptor,
	data T,
	findings []Finding,
) capability.Result[T] {
	outcome := capability.OutcomeHealthy
	recoveryRequired := false
	if len(findings) > 0 {
		outcome = capability.OutcomeAttention
		recoveryRequired = hasErrorFinding(findings)
	}
	return capability.Result[T]{
		Capability:  descriptor.Capability,
		Version:     descriptor.Version,
		Outcome:     outcome,
		Data:        data,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery: capability.Recovery{
			Required: recoveryRequired,
			Guidance: []string{},
		},
	}
}

func emptyResolveResult() capability.Result[ResolveData] {
	return emptyResult[ResolveData](ResolveDescriptor)
}

func emptyExplainResult() capability.Result[ExplainData] {
	return emptyResult[ExplainData](ExplainDescriptor)
}

func emptyValidateResult() capability.Result[ValidateData] {
	return emptyResult[ValidateData](ValidateDescriptor)
}

func emptySourcesResult() capability.Result[SourcesData] {
	return emptyResult[SourcesData](SourcesDescriptor)
}

func emptyResult[T any](
	descriptor capability.Descriptor,
) capability.Result[T] {
	return capability.Result[T]{
		Capability:  descriptor.Capability,
		Version:     descriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func sourcePointer(source Source) *Source {
	return &source
}
