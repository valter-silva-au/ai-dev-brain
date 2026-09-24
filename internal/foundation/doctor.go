package foundation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type DoctorRequest struct {
	Root string `json:"root"`
}

type Finding struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Summary     string   `json:"summary"`
	Evidence    []string `json:"evidence"`
	Remediation string   `json:"remediation"`
}

type DoctorData struct {
	WorkspaceID string    `json:"workspace_id"`
	Root        string    `json:"root"`
	Findings    []Finding `json:"findings"`
}

type declaredWorkspaceRole struct {
	name     string
	declared string
}

const (
	workspaceRoleConfig = "config"
	workspaceRoleState  = "state"
	workspaceRoleEvents = "events"
)

func (service *Service) Doctor(
	ctx context.Context,
	request DoctorRequest,
) (capability.Result[DoctorData], error) {
	layout, err := workspace.NewLayout(request.Root)
	if err != nil {
		return emptyDoctorResult(), err
	}

	data := DoctorData{
		Root:     layout.Root(),
		Findings: []Finding{},
	}
	manifestContent, err := os.ReadFile(layout.ManifestPath())
	if errors.Is(err, os.ErrNotExist) {
		data.Findings = append(data.Findings, Finding{
			ID:       "workspace.boundary.missing",
			Severity: "error",
			Summary:  "The path is not an initialized v3 workspace.",
			Evidence: []string{layout.ManifestPath() + " does not exist."},
			Remediation: "Run adb init boundary for this explicit workspace path before " +
				"using workspace-scoped capabilities.",
		})
		return doctorResult(data), nil
	}
	if err != nil {
		return emptyDoctorResult(), fmt.Errorf(
			"read workspace manifest %q: %w",
			layout.ManifestPath(),
			err,
		)
	}

	manifest, err := workspace.DecodeManifest(
		bytes.NewReader(manifestContent),
	)
	// Manifest.Validate also validates role paths. Validate the non-role fields
	// independently so role path failures can use the dedicated doctor finding.
	if err == nil {
		structuralManifest := manifest
		structuralManifest.Roles = workspace.DefaultRoles()
		err = structuralManifest.Validate(layout)
	}
	if err != nil {
		data.Findings = append(data.Findings, Finding{
			ID:          "workspace.manifest.invalid",
			Severity:    "error",
			Summary:     "The workspace manifest is invalid.",
			Evidence:    []string{err.Error()},
			Remediation: "Restore a valid aidb.workspace/v1 manifest from source control or backup.",
		})
		// A decode/validate failure is doctor's OUTPUT, not doctor failing: the
		// error above is carried as an error-severity finding, which makes
		// doctorResult report needs_attention with recovery required. Returning
		// err instead would collapse the typed report into a bare error and lose
		// the remediation. TestDoctorInvalidManifestIsNotHealthy pins that this
		// path can never read as healthy.
		//nolint:nilerr // the error is reported as a finding, not swallowed
		return doctorResult(data), nil
	}
	data.WorkspaceID = manifest.ID

	safeRoles := inspectWorkspaceRoles(layout, manifest)
	for _, role := range declaredWorkspaceRoles(manifest) {
		if finding, ok := safeRoles.findings[role.name]; ok {
			data.Findings = append(data.Findings, finding)
		}
	}
	if configPath, ok := safeRoles.paths[workspaceRoleConfig]; ok {
		service.checkConfig(configPath, &data)
	}
	if statePath, ok := safeRoles.paths[workspaceRoleState]; ok {
		service.checkControlPlane(
			ctx,
			layout,
			statePath,
			manifest,
			manifestContent,
			&data,
		)
	}
	if eventsDir, ok := safeRoles.paths[workspaceRoleEvents]; ok {
		service.checkJournals(eventsDir, &data)
	}

	return doctorResult(data), nil
}

type inspectedWorkspaceRoles struct {
	paths    map[string]string
	findings map[string]Finding
}

func declaredWorkspaceRoles(manifest workspace.Manifest) []declaredWorkspaceRole {
	return []declaredWorkspaceRole{
		{name: workspaceRoleConfig, declared: manifest.Roles.Config},
		{name: workspaceRoleState, declared: manifest.Roles.State},
		{name: workspaceRoleEvents, declared: manifest.Roles.Events},
		{name: "cache", declared: manifest.Roles.Cache},
		{name: "organizations", declared: manifest.Roles.Organizations},
	}
}

func inspectWorkspaceRoles(
	layout workspace.Layout,
	manifest workspace.Manifest,
) inspectedWorkspaceRoles {
	inspected := inspectedWorkspaceRoles{
		paths:    make(map[string]string),
		findings: make(map[string]Finding),
	}
	for _, role := range declaredWorkspaceRoles(manifest) {
		inspection, err := layout.InspectRole(role.declared)
		if err == nil {
			inspected.paths[role.name] = inspection.Target
			continue
		}

		component := role.declared
		reason := workspace.RoleViolationUninspectable
		var violation *workspace.RoleViolation
		if errors.As(err, &violation) {
			component = violation.Component
			reason = violation.Reason
		}
		inspected.findings[role.name] = Finding{
			ID:       "workspace.role.escaped",
			Severity: "error",
			Summary:  "A managed workspace role is not physically contained.",
			Evidence: []string{
				"role=" + role.name,
				"declared=" + role.declared,
				"component=" + component,
				"reason=" + string(reason),
			},
			Remediation: "Preserve and review the role's existing content, then replace " +
				"the symbolic link with a real directory beneath the workspace.",
		}
	}
	return inspected
}

func (service *Service) checkConfig(
	configPath string,
	data *DoctorData,
) {
	info, err := os.Stat(configPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		data.Findings = append(data.Findings, Finding{
			ID:       "workspace.config.missing",
			Severity: "error",
			Summary:  "The portable workspace configuration is missing.",
			Evidence: []string{configPath + " does not exist."},
			Remediation: "Restore the declared portable configuration from source control or " +
				"review an initialization recovery plan.",
		})
	case err != nil:
		data.Findings = append(data.Findings, Finding{
			ID:          "workspace.config.unreadable",
			Severity:    "error",
			Summary:     "The portable workspace configuration cannot be inspected.",
			Evidence:    []string{err.Error()},
			Remediation: "Repair filesystem access without replacing the file.",
		})
	case !info.Mode().IsRegular():
		data.Findings = append(data.Findings, Finding{
			ID:          "workspace.config.invalid_type",
			Severity:    "error",
			Summary:     "The portable workspace configuration is not a regular file.",
			Evidence:    []string{configPath},
			Remediation: "Replace the path with a reviewed portable configuration file.",
		})
	}
}

func (service *Service) checkControlPlane(
	ctx context.Context,
	layout workspace.Layout,
	statePath string,
	manifest workspace.Manifest,
	manifestContent []byte,
	data *DoctorData,
) {
	stateStore, err := controlplane.OpenReadOnly(ctx, statePath)
	if err != nil {
		id := "control_plane.invalid"
		summary := "The rebuildable control-plane database is invalid."
		if errors.Is(err, os.ErrNotExist) {
			id = "control_plane.missing"
			summary = "The rebuildable control-plane database is missing."
		}
		data.Findings = append(data.Findings, Finding{
			ID:          id,
			Severity:    "error",
			Summary:     summary,
			Evidence:    []string{err.Error()},
			Remediation: "Run a reviewed control-plane rebuild; do not modify portable source artifacts.",
		})
		return
	}
	defer func() {
		_ = stateStore.Close()
	}()

	check, err := stateStore.Check(ctx)
	if err != nil {
		data.Findings = append(data.Findings, Finding{
			ID:          "control_plane.check_failed",
			Severity:    "error",
			Summary:     "The control-plane integrity check failed.",
			Evidence:    []string{err.Error()},
			Remediation: "Preserve the database and run a reviewed rebuild plan.",
		})
		return
	}
	if check.Integrity != "ok" {
		data.Findings = append(data.Findings, Finding{
			ID:          "control_plane.integrity",
			Severity:    "error",
			Summary:     "SQLite reported a control-plane integrity failure.",
			Evidence:    []string{check.Integrity},
			Remediation: "Preserve the database and rebuild derived state from portable sources and journals.",
		})
	}
	for _, operation := range check.IncompleteOperations {
		data.Findings = append(data.Findings, Finding{
			ID:       "operation.state.incomplete",
			Severity: "warning",
			Summary:  "The control plane records an incomplete operation.",
			Evidence: []string{
				operation.ID + " is " + operation.Status,
			},
			Remediation: "Inspect the matching journal before retrying or repairing.",
		})
	}

	projection, err := stateStore.Workspace(ctx)
	if err != nil {
		data.Findings = append(data.Findings, Finding{
			ID:          "control_plane.projection.missing",
			Severity:    "warning",
			Summary:     "The workspace projection is missing from derived state.",
			Evidence:    []string{err.Error()},
			Remediation: "Run a reviewed control-plane rebuild from the workspace manifest.",
		})
		return
	}
	manifestHash := journal.Digest(manifestContent)
	if projection.ID != manifest.ID ||
		projection.Root != layout.Root() ||
		projection.ManifestHash != manifestHash {
		data.Findings = append(data.Findings, Finding{
			ID:       "control_plane.projection.stale",
			Severity: "warning",
			Summary:  "The derived workspace projection does not match its source manifest.",
			Evidence: []string{
				fmt.Sprintf(
					"projection id=%q root=%q manifest_hash=%q",
					projection.ID,
					projection.Root,
					projection.ManifestHash,
				),
				fmt.Sprintf(
					"source id=%q root=%q manifest_hash=%q",
					manifest.ID,
					layout.Root(),
					manifestHash,
				),
			},
			Remediation: "Rebuild the workspace projection from the portable manifest.",
		})
	}
}

func (service *Service) checkJournals(
	eventsDir string,
	data *DoctorData,
) {
	entries, err := os.ReadDir(eventsDir)
	if errors.Is(err, os.ErrNotExist) {
		data.Findings = append(data.Findings, Finding{
			ID:          "operation.journal.missing",
			Severity:    "error",
			Summary:     "The durable operation journal directory is missing.",
			Evidence:    []string{eventsDir + " does not exist."},
			Remediation: "Restore the journal directory before applying workspace mutations.",
		})
		return
	}
	if err != nil {
		data.Findings = append(data.Findings, Finding{
			ID:          "operation.journal.unreadable",
			Severity:    "error",
			Summary:     "The durable operation journals cannot be inspected.",
			Evidence:    []string{err.Error()},
			Remediation: "Repair filesystem access without deleting journal evidence.",
		})
		return
	}

	journalStore, err := journal.NewStore(
		eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		data.Findings = append(data.Findings, Finding{
			ID:          "operation.journal.invalid",
			Severity:    "error",
			Summary:     "The operation journal store is invalid.",
			Evidence:    []string{err.Error()},
			Remediation: "Repair the journal root without deleting operation evidence.",
		})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		state, err := journalStore.Inspect(entry.Name())
		if err != nil {
			data.Findings = append(data.Findings, Finding{
				ID:       "operation.journal.invalid",
				Severity: "error",
				Summary:  "An operation journal cannot be validated.",
				Evidence: []string{
					entry.Name() + ": " + err.Error(),
				},
				Remediation: "Preserve the journal and resolve its structural or hash error.",
			})
			continue
		}
		switch state.Status {
		case journal.StatusCommitted:
		case journal.StatusFailed:
			data.Findings = append(data.Findings, Finding{
				ID:          "operation.journal.failed",
				Severity:    "warning",
				Summary:     "An operation journal records a failed operation.",
				Evidence:    []string{entry.Name()},
				Remediation: "Review the failure and its recovery guidance before retrying.",
			})
		case journal.StatusAttention:
			data.Findings = append(data.Findings, Finding{
				ID:       "operation.journal.attention",
				Severity: "error",
				Summary:  "An operation journal has ambiguous or invalid state.",
				Evidence: []string{
					entry.Name() + ": " + state.Reason,
				},
				Remediation: "Resolve the target hashes manually before any retry.",
			})
		// StatusPlanned and StatusApplying are the genuinely incomplete states —
		// a plan written but not applied, or an apply that never reached a
		// terminal event. They are named explicitly, rather than left to the
		// default, so a NEW journal.Status cannot inherit "incomplete" by
		// accident: exhaustive fails the build until it is classified here.
		case journal.StatusPlanned, journal.StatusApplying:
			fallthrough
		// The default is kept as well as the explicit cases: a status this build
		// does not recognise must still produce a finding rather than drop the
		// journal from the report.
		default:
			data.Findings = append(data.Findings, Finding{
				ID:       "operation.journal.incomplete",
				Severity: "warning",
				Summary:  "An operation journal is incomplete.",
				Evidence: []string{
					fmt.Sprintf("%s is %s", entry.Name(), state.Status),
				},
				Remediation: "Inspect the operation plan and resume only when its target state is unambiguous.",
			})
		}
	}
}

func doctorResult(
	data DoctorData,
) capability.Result[DoctorData] {
	outcome := capability.OutcomeHealthy
	recoveryRequired := false
	if len(data.Findings) > 0 {
		outcome = capability.OutcomeAttention
		for _, finding := range data.Findings {
			if finding.Severity == "error" {
				recoveryRequired = true
				break
			}
		}
	}

	return capability.Result[DoctorData]{
		Capability:  DoctorDescriptor.Capability,
		Version:     DoctorDescriptor.Version,
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

func emptyDoctorResult() capability.Result[DoctorData] {
	return capability.Result[DoctorData]{
		Capability:  DoctorDescriptor.Capability,
		Version:     DoctorDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}
