package organization

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type Data struct {
	ID           string     `json:"id"`
	WorkspaceID  string     `json:"workspace_id"`
	Slug         string     `json:"slug"`
	DisplayName  string     `json:"display_name"`
	ParentID     string     `json:"parent_id,omitempty"`
	Owner        string     `json:"owner,omitempty"`
	Description  string     `json:"description,omitempty"`
	Trust        string     `json:"trust,omitempty"`
	Profile      string     `json:"profile,omitempty"`
	Status       string     `json:"status"`
	Path         string     `json:"path"`
	Aliases      []string   `json:"aliases"`
	Roles        Roles      `json:"roles"`
	Provenance   Provenance `json:"provenance"`
	LastMutation Provenance `json:"last_mutation"`
}

type ListRequest struct {
	WorkspaceRoot   string `json:"workspace_root"`
	IncludeArchived bool   `json:"include_archived"`
}

type ListData struct {
	WorkspaceID   string `json:"workspace_id"`
	Organizations []Data `json:"organizations"`
}

type ShowRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Selector      string `json:"selector"`
}

type ShowData struct {
	Organization Data `json:"organization"`
}

type ValidateRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Selector      string `json:"selector"`
}

type Finding struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Summary     string   `json:"summary"`
	Evidence    []string `json:"evidence"`
	Remediation string   `json:"remediation"`
}

type ValidateData struct {
	Organization Data      `json:"organization"`
	Findings     []Finding `json:"findings"`
}

func (service *Service) List(
	ctx context.Context,
	request ListRequest,
) (capability.Result[ListData], error) {
	workspaceLayout, workspaceManifest, state, err := openReadOnlyWorkspace(
		ctx,
		request.WorkspaceRoot,
	)
	if err != nil {
		return emptyListResult(), err
	}
	defer func() {
		_ = state.Close()
	}()

	projections, err := state.Organizations(ctx)
	if err != nil {
		return emptyListResult(), err
	}
	organizations := make([]Data, 0, len(projections))
	for _, projection := range projections {
		if !request.IncludeArchived &&
			projection.Status == controlplane.EntityStatusArchived {
			continue
		}
		data, _, err := readOrganizationData(
			workspaceLayout,
			workspaceManifest.ID,
			workspaceManifest.Roles.Organizations,
			projection,
		)
		if err != nil {
			return emptyListResult(), err
		}
		organizations = append(organizations, data)
	}

	return capability.Result[ListData]{
		Capability: ListDescriptor.Capability,
		Version:    ListDescriptor.Version,
		Outcome:    capability.OutcomeHealthy,
		Data: ListData{
			WorkspaceID:   workspaceManifest.ID,
			Organizations: organizations,
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func (service *Service) Show(
	ctx context.Context,
	request ShowRequest,
) (capability.Result[ShowData], error) {
	if request.Selector == "" {
		return emptyShowResult(), errors.New(
			"organization selector is required",
		)
	}
	workspaceLayout, workspaceManifest, state, err := openReadOnlyWorkspace(
		ctx,
		request.WorkspaceRoot,
	)
	if err != nil {
		return emptyShowResult(), err
	}
	defer func() {
		_ = state.Close()
	}()

	projection, err := state.Organization(ctx, request.Selector)
	if err != nil {
		return emptyShowResult(), err
	}
	data, _, err := readOrganizationData(
		workspaceLayout,
		workspaceManifest.ID,
		workspaceManifest.Roles.Organizations,
		projection,
	)
	if err != nil {
		return emptyShowResult(), err
	}
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Outcome:     capability.OutcomeHealthy,
		Data:        ShowData{Organization: data},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func (service *Service) Validate(
	ctx context.Context,
	request ValidateRequest,
) (capability.Result[ValidateData], error) {
	if request.Selector == "" {
		return emptyValidateResult(), errors.New(
			"organization selector is required",
		)
	}
	workspaceLayout, workspaceManifest, state, err := openReadOnlyWorkspace(
		ctx,
		request.WorkspaceRoot,
	)
	if err != nil {
		return emptyValidateResult(), err
	}
	defer func() {
		_ = state.Close()
	}()

	projection, err := state.Organization(ctx, request.Selector)
	if err != nil {
		return emptyValidateResult(), err
	}
	data, manifest, err := readOrganizationData(
		workspaceLayout,
		workspaceManifest.ID,
		workspaceManifest.Roles.Organizations,
		projection,
	)
	if err != nil {
		return emptyValidateResult(), err
	}
	layout, err := NewLayoutForRole(
		workspaceLayout.Root(),
		workspaceManifest.Roles.Organizations,
		manifest.Slug,
	)
	if err != nil {
		return emptyValidateResult(), err
	}

	findings := make([]Finding, 0)
	manifestContent, err := readOrganizationTargetWithin(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err != nil {
		findings = append(findings, Finding{
			ID:          "organization.manifest.unreadable",
			Severity:    "error",
			Summary:     "The organization manifest cannot be read.",
			Evidence:    []string{err.Error()},
			Remediation: "Restore the portable organization manifest before mutation.",
		})
	} else {
		manifestHash := journal.Digest(manifestContent)
		if projection.ID != manifest.ID ||
			projection.Slug != manifest.Slug ||
			projection.Path != layout.Root() ||
			projection.DisplayName != manifest.DisplayName ||
			projection.ParentID != manifest.ParentID ||
			projection.ManifestHash != manifestHash ||
			string(projection.Status) != manifest.Status ||
			!reflect.DeepEqual(projection.Aliases, manifest.Aliases) {
			findings = append(findings, Finding{
				ID:       "organization.projection.stale",
				Severity: "warning",
				Summary:  "The rebuildable organization projection does not match its manifest.",
				Evidence: []string{
					fmt.Sprintf(
						"projection id=%q slug=%q path=%q hash=%q",
						projection.ID,
						projection.Slug,
						projection.Path,
						projection.ManifestHash,
					),
					fmt.Sprintf(
						"manifest id=%q slug=%q path=%q hash=%q",
						manifest.ID,
						manifest.Slug,
						layout.Root(),
						manifestHash,
					),
				},
				Remediation: "Run a reviewed control-plane rebuild from the organization manifest.",
			})
		}
	}

	checkRegularFile(
		workspaceLayout.Root(),
		layout.ConfigPath(),
		"organization.config",
		"portable organization configuration",
		&findings,
	)
	expectedAgents, pointerErr := AgentsPointer(layout)
	_, agentsErr := workspace.InspectPath(
		workspaceLayout.Root(),
		layout.AgentsPath(),
	)
	var agentsContent []byte
	if agentsErr == nil {
		agentsContent, agentsErr = readOrganizationTargetWithin(
			workspaceLayout.Root(),
			layout.AgentsPath(),
		)
	}
	switch {
	case agentsErr != nil:
		findings = append(findings, Finding{
			ID:          "organization.agents.missing",
			Severity:    "error",
			Summary:     "The organization AGENTS.md pointer is missing or unreadable.",
			Evidence:    []string{agentsErr.Error()},
			Remediation: "Restore the generated pointer to workspace AGENTS.md.",
		})
	case pointerErr != nil:
		return emptyValidateResult(), pointerErr
	case !bytes.Equal(agentsContent, []byte(expectedAgents)):
		findings = append(findings, Finding{
			ID:       "organization.agents.drift",
			Severity: "warning",
			Summary:  "The organization AGENTS.md pointer has drifted.",
			Evidence: []string{
				layout.AgentsPath() + " differs from the registered generated content.",
			},
			Remediation: "Review and restore the portable pointer; do not overwrite authored instructions automatically.",
		})
	}

	for name, path := range map[string]string{
		"knowledge":    layout.KnowledgeDir(),
		"stakeholders": layout.StakeholdersDir(),
		"tickets":      layout.TicketsDir(),
		"repositories": layout.RepositoriesDir(),
	} {
		inspection, err := workspace.InspectPath(
			workspaceLayout.Root(),
			path,
		)
		if err != nil {
			findings = append(findings, Finding{
				ID:          "organization.role.unreadable",
				Severity:    "error",
				Summary:     "An organization semantic role cannot be inspected.",
				Evidence:    []string{name + ": " + err.Error()},
				Remediation: "Repair filesystem access without replacing authored content.",
			})
			continue
		}
		if inspection.State == workspace.RoleInspectionMissingTail {
			continue
		}
		info, err := statOrganizationTargetWithin(
			workspaceLayout.Root(),
			path,
		)
		if err != nil {
			findings = append(findings, Finding{
				ID:          "organization.role.unreadable",
				Severity:    "error",
				Summary:     "An organization semantic role cannot be inspected.",
				Evidence:    []string{name + ": " + err.Error()},
				Remediation: "Repair filesystem access without replacing authored content.",
			})
			continue
		}
		if !info.IsDir() {
			findings = append(findings, Finding{
				ID:          "organization.role.invalid_type",
				Severity:    "error",
				Summary:     "An organization directory role is not a directory.",
				Evidence:    []string{name + ": " + path},
				Remediation: "Move the conflicting path aside and restore the registered directory role.",
			})
		}
	}

	outcome := capability.OutcomeHealthy
	recoveryRequired := false
	if len(findings) > 0 {
		outcome = capability.OutcomeAttention
		for _, finding := range findings {
			if finding.Severity == "error" {
				recoveryRequired = true
				break
			}
		}
	}
	return capability.Result[ValidateData]{
		Capability: ValidateDescriptor.Capability,
		Version:    ValidateDescriptor.Version,
		Outcome:    outcome,
		Data: ValidateData{
			Organization: data,
			Findings:     findings,
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery: capability.Recovery{
			Required: recoveryRequired,
			Guidance: []string{},
		},
	}, nil
}

func openReadOnlyWorkspace(
	ctx context.Context,
	root string,
) (workspace.Layout, workspace.Manifest, *controlplane.Store, error) {
	layout, err := workspace.NewLayout(root)
	if err != nil {
		return workspace.Layout{}, workspace.Manifest{}, nil, err
	}
	manifest, err := workspace.ReadManifest(layout.ManifestPath())
	if err != nil {
		return workspace.Layout{}, workspace.Manifest{}, nil, err
	}
	if err := manifest.Validate(layout); err != nil {
		return workspace.Layout{}, workspace.Manifest{}, nil, err
	}
	if _, err := layout.InspectRole(manifest.Roles.Organizations); err != nil {
		return workspace.Layout{}, workspace.Manifest{}, nil, fmt.Errorf(
			"inspect workspace organizations role: %w",
			err,
		)
	}
	state, err := controlplane.OpenReadOnly(ctx, layout.StatePath())
	if err != nil {
		return workspace.Layout{}, workspace.Manifest{}, nil, err
	}
	return layout, manifest, state, nil
}

func readOrganizationData(
	workspaceLayout workspace.Layout,
	workspaceID string,
	organizationsRole string,
	projection controlplane.OrganizationProjection,
) (Data, Manifest, error) {
	layout, err := NewLayoutForRole(
		workspaceLayout.Root(),
		organizationsRole,
		projection.Slug,
	)
	if err != nil {
		return Data{}, Manifest{}, err
	}
	if layout.Root() != projection.Path {
		return Data{}, Manifest{}, fmt.Errorf(
			"organization projection path %q does not match canonical path %q",
			projection.Path,
			layout.Root(),
		)
	}
	if _, err := workspace.InspectPath(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	); err != nil {
		return Data{}, Manifest{}, fmt.Errorf(
			"inspect organization manifest path %q: %w",
			layout.ManifestPath(),
			err,
		)
	}
	content, err := readOrganizationTargetWithin(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err != nil {
		return Data{}, Manifest{}, err
	}
	manifest, err := DecodeManifest(bytes.NewReader(content))
	if err != nil {
		return Data{}, Manifest{}, fmt.Errorf(
			"read organization manifest %q: %w",
			layout.ManifestPath(),
			err,
		)
	}
	if err := manifest.Validate(layout); err != nil {
		return Data{}, Manifest{}, err
	}
	return Data{
		ID:           manifest.ID,
		WorkspaceID:  workspaceID,
		Slug:         manifest.Slug,
		DisplayName:  manifest.DisplayName,
		ParentID:     manifest.ParentID,
		Owner:        manifest.Owner,
		Description:  manifest.Description,
		Trust:        manifest.Trust,
		Profile:      manifest.Profile,
		Status:       manifest.Status,
		Path:         layout.Root(),
		Aliases:      append([]string(nil), manifest.Aliases...),
		Roles:        manifest.Roles,
		Provenance:   manifest.Provenance,
		LastMutation: manifest.LastMutation,
	}, manifest, nil
}

func checkRegularFile(
	root string,
	path string,
	idPrefix string,
	summaryName string,
	findings *[]Finding,
) {
	inspection, err := workspace.InspectPath(root, path)
	switch {
	case err != nil:
		*findings = append(*findings, Finding{
			ID:          idPrefix + ".unreadable",
			Severity:    "error",
			Summary:     "The " + summaryName + " cannot be inspected.",
			Evidence:    []string{err.Error()},
			Remediation: "Repair filesystem access without replacing the file.",
		})
		return
	case inspection.State == workspace.RoleInspectionMissingTail:
		*findings = append(*findings, Finding{
			ID:          idPrefix + ".missing",
			Severity:    "error",
			Summary:     "The " + summaryName + " is missing.",
			Evidence:    []string{path + " does not exist."},
			Remediation: "Restore the portable file from source control or a reviewed recovery plan.",
		})
		return
	}
	info, err := statOrganizationTargetWithin(root, path)
	switch {
	case err != nil:
		*findings = append(*findings, Finding{
			ID:          idPrefix + ".unreadable",
			Severity:    "error",
			Summary:     "The " + summaryName + " cannot be inspected.",
			Evidence:    []string{err.Error()},
			Remediation: "Repair filesystem access without replacing the file.",
		})
	case !info.Mode().IsRegular():
		*findings = append(*findings, Finding{
			ID:          idPrefix + ".invalid_type",
			Severity:    "error",
			Summary:     "The " + summaryName + " is not a regular file.",
			Evidence:    []string{path},
			Remediation: "Move the conflicting path aside and restore the portable file.",
		})
	}
}

func emptyListResult() capability.Result[ListData] {
	return capability.Result[ListData]{
		Capability:  ListDescriptor.Capability,
		Version:     ListDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyShowResult() capability.Result[ShowData] {
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyValidateResult() capability.Result[ValidateData] {
	return capability.Result[ValidateData]{
		Capability:  ValidateDescriptor.Capability,
		Version:     ValidateDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}
