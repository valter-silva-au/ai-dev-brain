package ticket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

var errUnsafeRootedPath = errors.New("unsafe rooted path")

var (
	ValidateDescriptor = capability.Descriptor{
		Capability: "ticket.validate",
		Version:    "v1",
		Command:    "validate",
		Tool:       "adb_ticket_validate",
		Summary:    "Validate one portable ticket without mutation.",
		Mutating:   false,
	}
	PolicyCheckDescriptor = capability.Descriptor{
		Capability: "policy.check",
		Version:    "v1",
		Command:    "check",
		Tool:       "adb_policy_check",
		Summary:    "Check ticket conformance across one recognized scope.",
		Mutating:   false,
	}
	PolicyReconcileDescriptor = capability.Descriptor{
		Capability: "policy.reconcile",
		Version:    "v1",
		Command:    "reconcile",
		Tool:       "adb_policy_reconcile",
		Summary:    "Plan or apply safe ticket conformance repairs.",
		Mutating:   true,
	}
)

type ValidateRequest struct {
	Scope         ScopeLayout
	Selector      string
	ActiveProfile profile.Profile
	Mode          profile.ConformanceMode
}

type PolicyCheckRequest struct {
	Scope           ScopeLayout
	ActiveProfile   profile.Profile
	Mode            profile.ConformanceMode
	IncludeArchived bool
}

type PolicyReconcileRequest struct {
	Scope           ScopeLayout
	ActiveProfile   profile.Profile
	Mode            profile.ConformanceMode
	IncludeArchived bool
	ActorType       string
	ActorID         string
	Tool            string
	OperationID     string
	PlanHash        string
	Apply           bool
}

type PolicyTicket struct {
	Path   string                    `json:"path" yaml:"path"`
	Ticket *TicketData               `json:"ticket,omitempty" yaml:"ticket,omitempty"`
	Report profile.ConformanceReport `json:"report" yaml:"report"`
	Plan   profile.ReconcilePlan     `json:"plan" yaml:"plan"`
}

type ValidateData struct {
	Ticket PolicyTicket `json:"ticket" yaml:"ticket"`
}

type PolicyCheckData struct {
	Tickets []PolicyTicket `json:"tickets" yaml:"tickets"`
}

type PolicyReconcileData struct {
	OperationID string         `json:"operation_id" yaml:"operation_id"`
	PlanHash    string         `json:"plan_hash" yaml:"plan_hash"`
	Tickets     []PolicyTicket `json:"tickets" yaml:"tickets"`
}

type policyCandidate struct {
	path    string
	located *LocatedManifest
	issues  []profile.ConformanceIssue
}

type policyEvaluation struct {
	candidate policyCandidate
	request   profile.ConformanceRequest
	ticket    PolicyTicket
}

type reconcileStep struct {
	journal    journal.Step
	ticketRoot string
	ticketID   string
	relative   string
	content    []byte
}

func (service *Service) Validate(
	ctx context.Context,
	request ValidateRequest,
) (capability.Result[ValidateData], error) {
	checked, err := service.PolicyCheck(ctx, PolicyCheckRequest{
		Scope:         request.Scope,
		ActiveProfile: request.ActiveProfile,
		Mode:          request.Mode,
	})
	if err != nil {
		return emptyValidateConformanceResult(), err
	}
	for _, ticket := range checked.Data.Tickets {
		if policyTicketMatches(ticket, request.Selector) {
			return capability.Result[ValidateData]{
				Capability:  ValidateDescriptor.Capability,
				Version:     ValidateDescriptor.Version,
				Outcome:     conformanceOutcome([]PolicyTicket{ticket}),
				Data:        ValidateData{Ticket: ticket},
				Effects:     []capability.Effect{},
				Warnings:    []capability.Notice{},
				NextActions: []capability.Action{},
				Recovery:    capability.Recovery{Guidance: []string{}},
			}, nil
		}
	}
	return emptyValidateConformanceResult(), fmt.Errorf(
		"read ticket %q: no matching ticket",
		request.Selector,
	)
}

func (service *Service) PolicyCheck(
	_ context.Context,
	request PolicyCheckRequest,
) (capability.Result[PolicyCheckData], error) {
	evaluations, err := service.evaluatePolicy(
		request.Scope,
		request.ActiveProfile,
		request.Mode,
		request.IncludeArchived,
	)
	if err != nil {
		return emptyPolicyCheckResult(), err
	}
	tickets := policyTickets(evaluations)
	return capability.Result[PolicyCheckData]{
		Capability:  PolicyCheckDescriptor.Capability,
		Version:     PolicyCheckDescriptor.Version,
		Outcome:     conformanceOutcome(tickets),
		Data:        PolicyCheckData{Tickets: tickets},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func (service *Service) PolicyReconcile(
	_ context.Context,
	request PolicyReconcileRequest,
) (capability.Result[PolicyReconcileData], error) {
	if request.Mode != profile.ConformanceRepairSafe {
		return emptyPolicyReconcileResult(), errors.New(
			"policy reconciliation requires repair-safe mode",
		)
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return emptyPolicyReconcileResult(), err
	}
	if request.Apply &&
		(request.OperationID == "" || request.PlanHash == "") {
		return emptyPolicyReconcileResult(), errors.New(
			"applying policy reconciliation requires the preview operation id and plan hash",
		)
	}
	operationID := request.OperationID
	if operationID == "" {
		operationID = service.newID()
	}
	if err := validatePolicyOperationID(operationID); err != nil {
		return emptyPolicyReconcileResult(), err
	}
	resources, err := loadWorkspaceResources(request.Scope.WorkspaceRoot())
	if err != nil {
		return emptyPolicyReconcileResult(), err
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return emptyPolicyReconcileResult(), err
	}
	if request.Apply {
		if existing, readErr := store.ReadPlan(operationID); readErr == nil {
			if existing.Kind != PolicyReconcileDescriptor.Capability ||
				existing.IdempotencyKey != policyIdempotencyKey(request.Scope) ||
				reconcilePlanHash(existing.Steps) != request.PlanHash {
				return emptyPolicyReconcileResult(), journal.ErrPlanConflict
			}
			state, inspectErr := store.InspectRecorded(operationID)
			if inspectErr != nil {
				return emptyPolicyReconcileResult(), inspectErr
			}
			if state.Status == journal.StatusCommitted {
				return policyReconcileResult(
					capability.OutcomeUnchanged,
					operationID,
					request.PlanHash,
					[]PolicyTicket{},
					effectsFromJournal(existing.Steps, capability.EffectSkipped),
				), nil
			}
			return service.resumePolicyReconcile(
				request,
				operationID,
				store,
				existing,
			)
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return emptyPolicyReconcileResult(), readErr
		}
	}

	evaluations, err := service.evaluatePolicy(
		request.Scope,
		request.ActiveProfile,
		request.Mode,
		request.IncludeArchived,
	)
	if err != nil {
		return emptyPolicyReconcileResult(), err
	}
	steps, err := buildReconcileSteps(evaluations)
	if err != nil {
		return emptyPolicyReconcileResult(), err
	}
	effects := effectsFromReconcileSteps(steps, capability.EffectPlanned)
	tickets := policyTickets(evaluations)
	planHash := reconcilePlanHash(journalSteps(steps))
	if !request.Apply {
		outcome := capability.OutcomePlanned
		if len(steps) == 0 {
			outcome = conformanceOutcome(tickets)
		}
		return policyReconcileResult(
			outcome,
			operationID,
			planHash,
			tickets,
			effects,
		), nil
	}
	if request.PlanHash != planHash {
		return policyReconcileResult(
				capability.OutcomeConflict,
				operationID,
				planHash,
				tickets,
				effects,
			), errors.New(
				"policy reconciliation targets changed after preview",
			)
	}
	if len(steps) == 0 {
		return policyReconcileResult(
			capability.OutcomeUnchanged,
			operationID,
			planHash,
			tickets,
			[]capability.Effect{},
		), nil
	}

	plan := journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: policyIdempotencyKey(request.Scope),
		Kind:           PolicyReconcileDescriptor.Capability,
		Steps:          journalSteps(steps),
	}
	if _, err := store.Begin(plan); err != nil {
		return emptyPolicyReconcileResult(), err
	}
	return service.applyPolicyReconcile(
		request.Scope,
		operationID,
		planHash,
		store,
		plan.Steps,
		func() ([]PolicyTicket, []reconcileStep, error) {
			return tickets, steps, nil
		},
	)
}

func (service *Service) evaluatePolicy(
	scope ScopeLayout,
	activeProfile profile.Profile,
	mode profile.ConformanceMode,
	includeArchived bool,
) ([]policyEvaluation, error) {
	if _, err := profile.ReportIssues(mode, nil); err != nil {
		return nil, err
	}
	if err := profile.ValidateProfile(activeProfile); err != nil {
		return nil, fmt.Errorf("validate active policy profile: %w", err)
	}
	candidates, err := scanScopeForPolicy(scope)
	if err != nil {
		return nil, err
	}
	result := make([]policyEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.located == nil {
			report, reportErr := profile.ReportIssues(mode, candidate.issues)
			if reportErr != nil {
				return nil, reportErr
			}
			result = append(result, policyEvaluation{
				candidate: candidate,
				ticket: PolicyTicket{
					Path:   candidate.path,
					Report: report,
					Plan: profile.ReconcilePlan{
						Preview: true,
						Blocked: true,
						Effects: []profile.ReconcileEffect{},
					},
				},
			})
			continue
		}
		if !includeArchived &&
			candidate.located.Manifest.ArchiveState ==
				ArchiveStateArchived {
			continue
		}
		evaluated, evaluateErr := service.evaluateTicketPolicy(
			candidate,
			activeProfile,
			mode,
		)
		if evaluateErr != nil {
			return nil, evaluateErr
		}
		result = append(result, evaluated)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].ticket.Path < result[right].ticket.Path
	})
	return result, nil
}

func (service *Service) evaluateTicketPolicy(
	candidate policyCandidate,
	activeProfile profile.Profile,
	mode profile.ConformanceMode,
) (policyEvaluation, error) {
	located := *candidate.located
	ticketRoot, closeTicketRoot, err := openLocatedTicketRoot(located)
	if err != nil {
		issues := append(candidate.issues, profile.ConformanceIssue{
			Code:    "ticket.root.changed",
			Class:   profile.ConformanceContainedPathError,
			Path:    located.Layout.Root(),
			Summary: "Ticket root changed after scope discovery.",
		})
		report, reportErr := profile.ReportIssues(mode, issues)
		if reportErr != nil {
			return policyEvaluation{}, reportErr
		}
		data := ticketData(located.Layout, located.Manifest)
		return policyEvaluation{
			candidate: candidate,
			ticket: PolicyTicket{
				Path:   located.Layout.Root(),
				Ticket: &data,
				Report: report,
				Plan: profile.ReconcilePlan{
					Preview: true,
					Blocked: true,
					Effects: []profile.ReconcileEffect{},
				},
			},
		}, nil
	}
	defer closeTicketRoot()
	currentProfile, err := service.currentTicketProfile(
		located.Manifest.Profile,
		activeProfile,
	)
	if err != nil {
		issues := append(candidate.issues, profile.ConformanceIssue{
			Code:    "ticket.profile.unresolved",
			Class:   profile.ConformanceConfigurationError,
			Summary: err.Error(),
		})
		report, reportErr := profile.ReportIssues(mode, issues)
		if reportErr != nil {
			return policyEvaluation{}, reportErr
		}
		data := ticketData(located.Layout, located.Manifest)
		return policyEvaluation{
			candidate: candidate,
			ticket: PolicyTicket{
				Path:   located.Layout.Root(),
				Ticket: &data,
				Report: report,
				Plan: profile.ReconcilePlan{
					Preview: true,
					Blocked: true,
					Effects: []profile.ReconcileEffect{},
				},
			},
		}, nil
	}
	renderData := ticketRenderData(located.Manifest)
	desired, err := profile.Render(profile.RenderRequest{
		Profile:  activeProfile,
		Resolver: service.templateResolver,
		Data:     renderData,
	})
	if err != nil {
		return blockedPolicyEvaluation(
			candidate,
			located,
			mode,
			profile.ConformanceIssue{
				Code:    "ticket.active_profile.render_failed",
				Class:   profile.ConformanceConfigurationError,
				Path:    located.Layout.Root(),
				Summary: "Active ticket profile cannot be rendered.",
			},
		)
	}
	currentRender, renderIssues, err := service.readPolicyRenderManifest(
		located,
		ticketRoot,
		currentProfile,
		renderData,
	)
	if err != nil {
		blockingIssues := append(
			append([]profile.ConformanceIssue{}, renderIssues...),
			profile.ConformanceIssue{
				Code:    "ticket.current_profile.render_failed",
				Class:   profile.ConformanceConfigurationError,
				Path:    located.Layout.RenderManifestPath(),
				Summary: "Current ticket profile provenance cannot be reconstructed.",
			},
		)
		return blockedPolicyEvaluation(
			candidate,
			located,
			mode,
			blockingIssues...,
		)
	}
	tombstones, tombstoneIssues := readPolicyTombstones(
		located.Layout,
		ticketRoot,
	)
	// observePolicyArtifacts reports per-artifact trouble as a blocking
	// ConformanceIssue in observationIssues (folded into request.Issues below),
	// so there is no whole-pass failure to branch on here.
	observations, observationIssues := observePolicyArtifacts(
		located,
		activeProfile,
		ticketRoot,
	)
	request := profile.ConformanceRequest{
		Scope:          "ticket:" + located.Manifest.ID,
		Mode:           mode,
		CurrentProfile: currentProfile,
		ActiveProfile:  activeProfile,
		RenderManifest: currentRender,
		DesiredRender:  desired,
		Tombstones:     tombstones,
		Observations:   observations,
		Issues: append(
			append(
				append(
					append([]profile.ConformanceIssue{}, candidate.issues...),
					renderIssues...,
				),
				tombstoneIssues...,
			),
			observationIssues...,
		),
	}
	report, err := profile.CheckConformance(request)
	if err != nil {
		return policyEvaluation{}, fmt.Errorf(
			"check ticket conformance %q: %w",
			located.Layout.Root(),
			err,
		)
	}
	plan, err := profile.PlanReconciliation(request, report)
	if err != nil {
		return policyEvaluation{}, err
	}
	data := ticketData(located.Layout, located.Manifest)
	return policyEvaluation{
		candidate: candidate,
		request:   request,
		ticket: PolicyTicket{
			Path:   located.Layout.Root(),
			Ticket: &data,
			Report: report,
			Plan:   plan,
		},
	}, nil
}

func blockedPolicyEvaluation(
	candidate policyCandidate,
	located LocatedManifest,
	mode profile.ConformanceMode,
	blockingIssues ...profile.ConformanceIssue,
) (policyEvaluation, error) {
	issues := append(
		append([]profile.ConformanceIssue{}, candidate.issues...),
		blockingIssues...,
	)
	report, err := profile.ReportIssues(mode, issues)
	if err != nil {
		return policyEvaluation{}, err
	}
	data := ticketData(located.Layout, located.Manifest)
	return policyEvaluation{
		candidate: candidate,
		ticket: PolicyTicket{
			Path:   located.Layout.Root(),
			Ticket: &data,
			Report: report,
			Plan: profile.ReconcilePlan{
				Preview: true,
				Blocked: true,
				Effects: []profile.ReconcileEffect{},
			},
		},
	}, nil
}

func openLocatedTicketRoot(
	located LocatedManifest,
) (*os.Root, func(), error) {
	scopeRoot, err := openScopeTicketsRoot(located.Layout.Scope())
	if err != nil {
		return nil, nil, err
	}
	fail := func(cause error) (*os.Root, func(), error) {
		_ = scopeRoot.Close()
		return nil, nil, cause
	}
	relative := located.Layout.RelativePath()
	info, err := scopeRoot.Lstat(relative)
	if err != nil {
		return fail(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fail(errors.New("ticket root is not a safe directory"))
	}
	ticketRoot, err := scopeRoot.OpenRoot(relative)
	if err != nil {
		return fail(err)
	}
	openedInfo, err := ticketRoot.Stat(".")
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = ticketRoot.Close()
		if err != nil {
			return fail(err)
		}
		return fail(errors.New(
			"ticket root changed while it was being opened",
		))
	}
	statusBytes, err := readRootedRegularFile(
		ticketRoot,
		"status.yaml",
	)
	if err != nil {
		_ = ticketRoot.Close()
		return fail(err)
	}
	manifest, err := DecodeManifest(bytes.NewReader(statusBytes))
	if err != nil || manifest.ID != located.Manifest.ID {
		_ = ticketRoot.Close()
		if err != nil {
			return fail(err)
		}
		return fail(errors.New(
			"ticket identity changed after scope discovery",
		))
	}
	return ticketRoot, func() {
		_ = ticketRoot.Close()
		_ = scopeRoot.Close()
	}, nil
}

func scanScopeForPolicy(scope ScopeLayout) ([]policyCandidate, error) {
	if err := scope.ValidateFilesystemContainment(); err != nil {
		return nil, err
	}
	root, err := openScopeTicketsRoot(scope)
	if errors.Is(err, os.ErrNotExist) {
		return []policyCandidate{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(-1)
	closeErr := directory.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	sort.Slice(entries, func(left int, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	result := make([]policyCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == ".aidb" {
			continue
		}
		path := filepath.Join(scope.TicketsRoot(), entry.Name())
		info, lstatErr := root.Lstat(entry.Name())
		if lstatErr != nil {
			result = append(result, policyIssueCandidate(
				path,
				"ticket.path.unreadable",
				profile.ConformanceContainedPathError,
				"Ticket scope entry cannot be safely inspected.",
			))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result = append(result, policyIssueCandidate(
				path,
				"ticket.path.symlink",
				profile.ConformanceContainedPathError,
				"Ticket scope entries must not be symlinks.",
			))
			continue
		}
		if !info.IsDir() {
			continue
		}
		candidate := readPolicyCandidate(scope, root, entry.Name(), path)
		result = append(result, candidate)
	}
	return annotatePolicyCatalogIssues(result), nil
}

func annotatePolicyCatalogIssues(
	candidates []policyCandidate,
) []policyCandidate {
	owners := make(map[string][]int)
	for index, candidate := range candidates {
		if candidate.located == nil {
			continue
		}
		selectors := identitySelectors(
			candidate.located.Layout,
			candidate.located.Manifest,
		)
		selectors[foldSelector(candidate.located.Manifest.ID)] =
			candidate.located.Manifest.ID
		for selector := range selectors {
			owners[selector] = append(owners[selector], index)
		}
	}
	selectors := make([]string, 0, len(owners))
	for selector := range owners {
		selectors = append(selectors, selector)
	}
	sort.Strings(selectors)
	for _, selector := range selectors {
		indexes := owners[selector]
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			candidates[index].issues = append(
				candidates[index].issues,
				profile.ConformanceIssue{
					Code:  "ticket.catalog.collision",
					Class: profile.ConformanceInvalidMetadata,
					Path:  candidates[index].path,
					Summary: fmt.Sprintf(
						"Ticket selector %q collides within its scope.",
						selector,
					),
				},
			)
		}
	}
	return candidates
}

func readPolicyCandidate(
	scope ScopeLayout,
	root *os.Root,
	entryName string,
	path string,
) policyCandidate {
	ticketRoot, err := root.OpenRoot(entryName)
	if err != nil {
		return policyIssueCandidate(
			path,
			"ticket.path.changed",
			profile.ConformanceContainedPathError,
			"Ticket directory changed while it was being inspected.",
		)
	}
	defer ticketRoot.Close()
	statusInfo, err := ticketRoot.Lstat("status.yaml")
	if errors.Is(err, os.ErrNotExist) {
		return policyIssueCandidate(
			path,
			"ticket.status.missing",
			profile.ConformanceInvalidMetadata,
			"Ticket directory has no authoritative status.yaml.",
		)
	}
	if err != nil ||
		statusInfo.Mode()&os.ModeSymlink != 0 ||
		!statusInfo.Mode().IsRegular() {
		return policyIssueCandidate(
			path,
			"ticket.status.invalid_path",
			profile.ConformanceContainedPathError,
			"Ticket status.yaml is not a safe regular file.",
		)
	}
	file, err := ticketRoot.Open("status.yaml")
	if err != nil {
		return policyIssueCandidate(
			path,
			"ticket.status.unreadable",
			profile.ConformanceInvalidMetadata,
			"Ticket status.yaml cannot be read.",
		)
	}
	manifest, decodeErr := DecodeManifest(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		return policyIssueCandidate(
			path,
			"ticket.status.invalid",
			profile.ConformanceInvalidMetadata,
			"Ticket status.yaml is invalid.",
		)
	}
	if manifest.OrganizationID != scope.OrganizationID() ||
		manifest.RepositoryID != scope.RepositoryID() {
		return policyIssueCandidate(
			path,
			"ticket.scope.mismatch",
			profile.ConformanceContainedPathError,
			"Ticket metadata crosses its owning trust scope.",
		)
	}
	layout, err := scope.Ticket(manifest.LocalKey, manifest.PathSlug)
	if err != nil || layout.RelativePath() != entryName {
		return policyIssueCandidate(
			path,
			"ticket.layout.invalid",
			profile.ConformanceInvalidMetadata,
			"Ticket directory does not match its portable identity.",
		)
	}
	if err := validateStoredManifest(manifest, layout); err != nil {
		return policyIssueCandidate(
			path,
			"ticket.status.invalid",
			profile.ConformanceInvalidMetadata,
			"Ticket status metadata is invalid.",
		)
	}
	located := LocatedManifest{Layout: layout, Manifest: manifest}
	return policyCandidate{
		path:    path,
		located: &located,
		issues:  []profile.ConformanceIssue{},
	}
}

func policyIssueCandidate(
	path string,
	code string,
	class profile.ConformanceClass,
	summary string,
) policyCandidate {
	return policyCandidate{
		path: path,
		issues: []profile.ConformanceIssue{{
			Code:    code,
			Class:   class,
			Path:    path,
			Summary: summary,
		}},
	}
}

func (service *Service) currentTicketProfile(
	reference profile.ProfileReference,
	active profile.Profile,
) (profile.Profile, error) {
	if reference.ID == active.ID && reference.Version == active.Version {
		return active, nil
	}
	if service.profileResolver == nil {
		return profile.Profile{}, errors.New(
			"ticket profile resolver is unavailable",
		)
	}
	value, err := service.profileResolver.ResolveProfile(reference)
	if err != nil {
		return profile.Profile{}, err
	}
	if value.ID != reference.ID || value.Version != reference.Version {
		return profile.Profile{}, errors.New(
			"ticket profile resolver returned mismatched identity",
		)
	}
	if err := profile.ValidateProfile(value); err != nil {
		return profile.Profile{}, err
	}
	return value, nil
}

func ticketRenderData(manifest Manifest) map[string]string {
	return map[string]string{
		"Title":     manifest.Title,
		"TicketID":  manifest.LocalKey,
		"CreatedAt": manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (service *Service) readPolicyRenderManifest(
	located LocatedManifest,
	root *os.Root,
	currentProfile profile.Profile,
	renderData map[string]string,
) (
	profile.RenderManifest,
	[]profile.ConformanceIssue,
	error,
) {
	content, err := readRootedRegularFile(
		root,
		filepath.Join(".aidb", "rendered.yaml"),
	)
	var decodeErr error
	if err == nil {
		var value profile.RenderManifest
		value, decodeErr = profile.DecodeRenderManifest(
			bytes.NewReader(content),
		)
		if decodeErr == nil {
			return value, []profile.ConformanceIssue{}, nil
		}
	}
	class := profile.ConformanceInvalidMetadata
	if errors.Is(err, errUnsafeRootedPath) ||
		profile.IsContainedPathError(decodeErr) {
		class = profile.ConformanceContainedPathError
	}
	issues := []profile.ConformanceIssue{{
		Code:    "ticket.render_manifest.invalid",
		Class:   class,
		Path:    located.Layout.RenderManifestPath(),
		Summary: "Ticket render provenance is missing or invalid.",
	}}
	fallback, renderErr := profile.Render(profile.RenderRequest{
		Profile:  currentProfile,
		Resolver: service.templateResolver,
		Data:     renderData,
	})
	if renderErr != nil {
		return profile.RenderManifest{}, issues, renderErr
	}
	return fallback.Manifest, issues, nil
}

func readPolicyTombstones(
	layout Layout,
	root *os.Root,
) (profile.TombstoneManifest, []profile.ConformanceIssue) {
	content, err := readRootedRegularFile(
		root,
		filepath.Join(".aidb", "tombstones.yaml"),
	)
	var decodeErr error
	if err == nil {
		var value profile.TombstoneManifest
		value, decodeErr = profile.DecodeTombstones(
			bytes.NewReader(content),
		)
		if decodeErr == nil {
			return value, []profile.ConformanceIssue{}
		}
	}
	class := profile.ConformanceInvalidMetadata
	if errors.Is(err, errUnsafeRootedPath) ||
		profile.IsContainedPathError(decodeErr) {
		class = profile.ConformanceContainedPathError
	}
	return profile.TombstoneManifest{
			SchemaVersion: profile.TombstoneManifestSchemaVersion,
			Tombstones:    []profile.Tombstone{},
		}, []profile.ConformanceIssue{{
			Code:    "ticket.tombstones.invalid",
			Class:   class,
			Path:    layout.TombstonesPath(),
			Summary: "Ticket tombstones are missing or invalid.",
		}}
}

// readRootedRegularFile reads a contained regular file relative to root.
//
// It deliberately drops readRegularAt's os.FileInfo: every caller here wants the
// bytes, and the only users of the FileInfo (publication identity checks) call
// readRegularAt directly with the parent root they already hold.
func readRootedRegularFile(
	root *os.Root,
	relativePath string,
) ([]byte, error) {
	parent, base, closeParent, err := openSafeRootParent(
		root,
		relativePath,
	)
	if err != nil {
		return nil, err
	}
	defer closeParent()
	content, _, err := readRegularAt(parent, base)
	return content, err
}

func readRegularAt(
	parent *os.Root,
	base string,
) ([]byte, os.FileInfo, error) {
	info, err := parent.Lstat(base)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf(
			"%w: rooted file target is not a regular file",
			errUnsafeRootedPath,
		)
	}
	file, err := parent.Open(base)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, fmt.Errorf(
			"%w: rooted file changed while it was being opened",
			errUnsafeRootedPath,
		)
	}
	content, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, nil, readErr
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}
	return content, info, nil
}

func openSafeRootParent(
	root *os.Root,
	relativePath string,
) (*os.Root, string, func(), error) {
	parent, base, owned, err := openSafeRootParentHandle(
		root,
		relativePath,
	)
	if err != nil {
		return nil, "", nil, err
	}
	closeParent := func() {}
	if owned {
		closeParent = func() {
			_ = parent.Close()
		}
	}
	return parent, base, closeParent, nil
}

func openSafeRootParentHandle(
	root *os.Root,
	relativePath string,
) (*os.Root, string, bool, error) {
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." ||
		filepath.IsAbs(cleaned) ||
		filepath.VolumeName(cleaned) != "" ||
		cleaned == ".." ||
		strings.HasPrefix(
			cleaned,
			".."+string(filepath.Separator),
		) {
		return nil, "", false, fmt.Errorf(
			"%w: rooted path is invalid: %q",
			errUnsafeRootedPath,
			relativePath,
		)
	}
	base := filepath.Base(cleaned)
	parentPath := filepath.Dir(cleaned)
	if parentPath == "." {
		return root, base, false, nil
	}
	components := strings.Split(parentPath, string(filepath.Separator))
	current := root
	owned := false
	closeCurrent := func() {
		if owned {
			_ = current.Close()
		}
	}
	for _, component := range components {
		info, err := current.Lstat(component)
		if err != nil {
			closeCurrent()
			return nil, "", false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			closeCurrent()
			return nil, "", false, fmt.Errorf(
				"%w: rooted parent contains a non-directory or symlink",
				errUnsafeRootedPath,
			)
		}
		next, err := current.OpenRoot(component)
		if err != nil {
			closeCurrent()
			return nil, "", false, err
		}
		openedInfo, err := next.Stat(".")
		if err != nil || !os.SameFile(info, openedInfo) {
			_ = next.Close()
			closeCurrent()
			if err != nil {
				return nil, "", false, err
			}
			return nil, "", false, fmt.Errorf(
				"%w: rooted parent changed while it was being opened",
				errUnsafeRootedPath,
			)
		}
		closeCurrent()
		current = next
		owned = true
	}
	return current, base, owned, nil
}

func inspectRootedDirectoryPath(
	root *os.Root,
	relativePath string,
) (bool, error) {
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." ||
		filepath.IsAbs(cleaned) ||
		filepath.VolumeName(cleaned) != "" ||
		cleaned == ".." ||
		strings.HasPrefix(
			cleaned,
			".."+string(filepath.Separator),
		) {
		return false, fmt.Errorf(
			"rooted directory path is invalid: %q",
			relativePath,
		)
	}
	components := strings.Split(cleaned, string(filepath.Separator))
	opened := make([]*os.Root, 0, len(components))
	defer func() {
		for index := len(opened) - 1; index >= 0; index-- {
			_ = opened[index].Close()
		}
	}()
	current := root
	for _, component := range components {
		info, err := current.Lstat(component)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, errors.New(
				"rooted directory path contains a non-directory or symlink",
			)
		}
		next, err := current.OpenRoot(component)
		if err != nil {
			return false, err
		}
		openedInfo, err := next.Stat(".")
		if err != nil || !os.SameFile(info, openedInfo) {
			_ = next.Close()
			if err != nil {
				return false, err
			}
			return false, errors.New(
				"rooted directory changed while it was being opened",
			)
		}
		opened = append(opened, next)
		current = next
	}
	return true, nil
}

// observePolicyArtifacts inspects every artifact the active profile declares and
// returns what it found plus the issues that inspection itself raised.
//
// It returns no error: an artifact that cannot be inspected or read is reported as
// a ConformanceIssue (a contained-path or invalid-metadata finding, which always
// blocks) rather than as a failure of the observation pass, so one unsafe artifact
// never costs the caller the observations for all the others.
func observePolicyArtifacts(
	located LocatedManifest,
	activeProfile profile.Profile,
	root *os.Root,
) (
	[]profile.ArtifactObservation,
	[]profile.ConformanceIssue,
) {
	checkpoints := make(
		map[string]AppendOnlyCheckpoint,
		len(located.Manifest.AppendOnlyCheckpoints),
	)
	for _, checkpoint := range located.Manifest.AppendOnlyCheckpoints {
		checkpoints[checkpoint.Role] = checkpoint
	}
	observations := make(
		[]profile.ArtifactObservation,
		0,
		len(activeProfile.Artifacts),
	)
	issues := make([]profile.ConformanceIssue, 0)
	for _, artifact := range activeProfile.Artifacts {
		observation := profile.ArtifactObservation{
			Role: artifact.Role,
			Path: artifact.Path,
			Kind: artifact.Kind,
		}
		nativePath := filepath.FromSlash(artifact.Path)
		info, lstatErr := lstatRooted(root, nativePath)
		if errors.Is(lstatErr, os.ErrNotExist) {
			observations = append(observations, observation)
			continue
		}
		if lstatErr != nil || info.Mode()&os.ModeSymlink != 0 {
			issues = append(issues, profile.ConformanceIssue{
				Code:    "ticket.artifact.unsafe_path",
				Class:   profile.ConformanceContainedPathError,
				Role:    artifact.Role,
				Path:    artifact.Path,
				Summary: "Ticket artifact cannot be safely inspected.",
			})
			observations = append(observations, observation)
			continue
		}
		if (artifact.Kind == profile.ArtifactDirectory && !info.IsDir()) ||
			(artifact.Kind == profile.ArtifactFile &&
				!info.Mode().IsRegular()) {
			issues = append(issues, profile.ConformanceIssue{
				Code:    "ticket.artifact.kind_mismatch",
				Class:   profile.ConformanceInvalidMetadata,
				Role:    artifact.Role,
				Path:    artifact.Path,
				Summary: "Ticket artifact kind differs from the active profile.",
			})
			observations = append(observations, observation)
			continue
		}
		if artifact.Kind == profile.ArtifactDirectory {
			observation.Exists = true
			observations = append(observations, observation)
			continue
		}
		content, readErr := readRootedRegularFile(root, nativePath)
		if readErr != nil {
			issues = append(issues, profile.ConformanceIssue{
				Code:    "ticket.artifact.unreadable",
				Class:   profile.ConformanceContainedPathError,
				Role:    artifact.Role,
				Path:    artifact.Path,
				Summary: "Ticket artifact cannot be safely read.",
			})
			observations = append(observations, observation)
			continue
		}
		observation.Exists = true
		observation.Hash = profile.HashContent(content)
		observation.Size = int64(len(content))
		if artifact.AppendOnly {
			checkpoint, exists := checkpoints[artifact.Role]
			if exists {
				observation.BaselineLength = checkpoint.Length
				observation.BaselineHash = checkpoint.Hash
				prefixLength := checkpoint.Length
				if prefixLength > int64(len(content)) {
					prefixLength = int64(len(content))
				}
				if prefixLength >= 0 {
					observation.PrefixHash = profile.HashContent(
						content[:prefixLength],
					)
				}
			}
		}
		observations = append(observations, observation)
	}
	return observations, issues
}

func lstatRooted(
	root *os.Root,
	relativePath string,
) (os.FileInfo, error) {
	parent, base, closeParent, err := openSafeRootParent(
		root,
		relativePath,
	)
	if err != nil {
		return nil, err
	}
	defer closeParent()
	return parent.Lstat(base)
}

func buildReconcileSteps(
	evaluations []policyEvaluation,
) ([]reconcileStep, error) {
	result := make([]reconcileStep, 0)
	for _, evaluation := range evaluations {
		if evaluation.candidate.located == nil ||
			evaluation.ticket.Report.Blocked {
			continue
		}
		located := *evaluation.candidate.located
		safeFileRoles := make(map[string]struct{})
		for _, effect := range evaluation.ticket.Plan.Effects {
			if !effect.Safe {
				continue
			}
			relative := filepath.FromSlash(effect.Path)
			target := filepath.Join(
				located.Layout.Root(),
				relative,
			)
			if err := requireContainedPath(located.Layout.Root(), target); err != nil {
				return nil, err
			}
			switch effect.Action {
			case profile.ReconcileCreateDirectory:
				result = append(result, reconcileStep{
					journal: journal.Step{
						Action:   "create-directory",
						Target:   target,
						TicketID: located.Manifest.ID,
					},
					ticketRoot: located.Layout.Root(),
					ticketID:   located.Manifest.ID,
					relative:   relative,
				})
			case profile.ReconcileCreateFile:
				result = append(result, reconcileStep{
					journal: journal.Step{
						Action:    "create-file",
						Target:    target,
						TicketID:  located.Manifest.ID,
						AfterHash: journal.Digest(effect.Content),
					},
					ticketRoot: located.Layout.Root(),
					ticketID:   located.Manifest.ID,
					relative:   relative,
					content:    append([]byte(nil), effect.Content...),
				})
				safeFileRoles[effect.Role] = struct{}{}
			case profile.ReconcileReplaceGenerated:
				result = append(result, reconcileStep{
					journal: journal.Step{
						Action:     "replace-generated",
						Target:     target,
						TicketID:   located.Manifest.ID,
						BeforeHash: effect.BeforeHash,
						AfterHash:  effect.AfterHash,
					},
					ticketRoot: located.Layout.Root(),
					ticketID:   located.Manifest.ID,
					relative:   relative,
					content:    append([]byte(nil), effect.Content...),
				})
				safeFileRoles[effect.Role] = struct{}{}
			case profile.ReconcileReview:
				// Unreachable, and it must stay that way: a review effect is
				// never Safe, so the guard above has already skipped it. Named
				// explicitly so that marking a review effect safe trips the
				// exhaustive linter here rather than quietly minting a
				// filesystem step for something a human was asked to decide.
				continue
			}
		}
		if len(safeFileRoles) == 0 {
			continue
		}
		next := evaluation.request.RenderManifest
		next.Artifacts = append(
			[]profile.RenderedRecord(nil),
			evaluation.request.RenderManifest.Artifacts...,
		)
		desired := make(
			map[string]profile.RenderedRecord,
			len(evaluation.request.DesiredRender.Manifest.Artifacts),
		)
		for _, record := range evaluation.request.DesiredRender.Manifest.Artifacts {
			desired[record.Role] = record
		}
		for index, record := range next.Artifacts {
			if _, safe := safeFileRoles[record.Role]; !safe {
				continue
			}
			if replacement, exists := desired[record.Role]; exists {
				next.Artifacts[index] = replacement
				delete(safeFileRoles, record.Role)
			}
		}
		for role := range safeFileRoles {
			if record, exists := desired[role]; exists {
				next.Artifacts = append(next.Artifacts, record)
			}
		}
		sort.Slice(next.Artifacts, func(left int, right int) bool {
			return next.Artifacts[left].Role < next.Artifacts[right].Role
		})
		nextBytes, err := encodeRenderManifest(next)
		if err != nil {
			return nil, err
		}
		ticketRoot, closeTicketRoot, err := openLocatedTicketRoot(located)
		if err != nil {
			return nil, err
		}
		beforeBytes, err := readRootedRegularFile(
			ticketRoot,
			filepath.Join(".aidb", "rendered.yaml"),
		)
		closeTicketRoot()
		if err != nil {
			return nil, err
		}
		if bytes.Equal(beforeBytes, nextBytes) {
			continue
		}
		result = append(result, reconcileStep{
			journal: journal.Step{
				Action:     "update-render-manifest",
				Target:     located.Layout.RenderManifestPath(),
				TicketID:   located.Manifest.ID,
				BeforeHash: journal.Digest(beforeBytes),
				AfterHash:  journal.Digest(nextBytes),
			},
			ticketRoot: located.Layout.Root(),
			ticketID:   located.Manifest.ID,
			relative:   filepath.Join(".aidb", "rendered.yaml"),
			content:    nextBytes,
		})
	}
	sort.SliceStable(result, func(left int, right int) bool {
		leftOrder := reconcileActionOrder(result[left].journal.Action)
		rightOrder := reconcileActionOrder(result[right].journal.Action)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return result[left].journal.Target < result[right].journal.Target
	})
	for index := range result {
		result[index].journal.Ordinal = index + 1
	}
	return result, nil
}

func reconcileActionOrder(action string) int {
	switch action {
	case "create-directory":
		return 0
	case "create-file", "replace-generated":
		return 1
	case "update-render-manifest":
		return 2
	default:
		return 3
	}
}

func (service *Service) resumePolicyReconcile(
	request PolicyReconcileRequest,
	operationID string,
	store *journal.Store,
	plan journal.Plan,
) (capability.Result[PolicyReconcileData], error) {
	return service.applyPolicyReconcile(
		request.Scope,
		operationID,
		request.PlanHash,
		store,
		plan.Steps,
		func() ([]PolicyTicket, []reconcileStep, error) {
			if err := service.recoverPolicyPublicationGuards(
				request.Scope,
				operationID,
				plan.Steps,
			); err != nil {
				return nil, nil, err
			}
			evaluations, err := service.evaluatePolicy(
				request.Scope,
				request.ActiveProfile,
				request.Mode,
				request.IncludeArchived,
			)
			if err != nil {
				return nil, nil, err
			}
			steps, err := reconstructReconcileSteps(plan, evaluations)
			if err != nil {
				return nil, nil, err
			}
			return policyTickets(evaluations), steps, nil
		},
	)
}

func (service *Service) recoverPolicyPublicationGuards(
	scope ScopeLayout,
	operationID string,
	steps []journal.Step,
) error {
	scopeRoot, err := openScopeTicketsRoot(scope)
	if err != nil {
		return err
	}
	defer scopeRoot.Close()
	for _, step := range steps {
		if step.Action != "replace-generated" &&
			step.Action != "update-render-manifest" {
			continue
		}
		ticketRoot, targetName, ticketTarget, err :=
			validatePolicyJournalTarget(
				scope,
				scopeRoot,
				step.Target,
				step.TicketID,
			)
		if err != nil {
			return err
		}
		guardBase := reconcilePublicationGuardName(
			operationID,
			step.Ordinal,
		)
		ticketGuard := rootedSiblingPath(ticketTarget, guardBase)
		guard, guardInfo, err := readRegularAt(
			ticketRoot,
			ticketGuard,
		)
		if errors.Is(err, os.ErrNotExist) {
			_ = ticketRoot.Close()
			continue
		}
		if err != nil {
			_ = ticketRoot.Close()
			return err
		}
		guardHash := journal.Digest(guard)
		tokenBase := "." + guardBase + ".recover-" + uuid.NewString()
		ticketToken := rootedSiblingPath(ticketTarget, tokenBase)
		if err := ticketRoot.Link(ticketGuard, ticketToken); err != nil {
			_ = ticketRoot.Close()
			return err
		}
		if err := verifyRootedPublishedFile(
			ticketRoot,
			ticketToken,
			guardInfo,
			guardHash,
		); err != nil {
			_ = ticketRoot.Close()
			return err
		}
		targetToken := rootedSiblingPath(targetName, tokenBase)
		targetGuard := rootedSiblingPath(targetName, guardBase)
		if service.beforeRecoveryGuardSettle != nil {
			if err := service.beforeRecoveryGuardSettle(
				step.Target,
			); err != nil {
				err = removeRecoveryToken(ticketRoot, ticketToken, err)
				_ = ticketRoot.Close()
				return err
			}
		}
		targetParts := strings.Split(
			filepath.Clean(targetName),
			string(filepath.Separator),
		)
		if len(targetParts) < 2 {
			err := removeRecoveryToken(ticketRoot, ticketToken, errors.New(
				"reconciliation recovery target lacks a ticket path",
			))
			_ = ticketRoot.Close()
			return err
		}
		if err := validateScopedTicketIdentity(
			scopeRoot,
			ticketRoot,
			targetParts[0],
			step.TicketID,
		); err != nil {
			err = removeRecoveryToken(ticketRoot, ticketToken, err)
			_ = ticketRoot.Close()
			return err
		}
		_, settleErr := settleRootedPublicationSource(
			scopeRoot,
			targetName,
			targetToken,
			targetGuard,
			guardInfo,
			guard,
			guardHash,
			step,
		)
		settleErr = removeRecoveryToken(ticketRoot, ticketToken, settleErr)
		_ = ticketRoot.Close()
		if settleErr != nil {
			return settleErr
		}
	}
	return nil
}

// removeRecoveryToken deletes the hard link recovery used to carry a publication
// guard between two rooted views of the same ticket, and reports a failed
// deletion instead of dropping it.
//
// The residue is inert — recovery only ever looks for the deterministic guard
// name, never for a ".recover-<uuid>" link — but it is a durable hard link left
// inside a ticket directory, and the recovery pass is exactly the code an
// operator reads after a crash. Silence there would hide the one trace that
// something was left behind. cause may be nil (the settle succeeded), in which
// case a failed deletion becomes the reported error rather than being lost.
func removeRecoveryToken(
	ticketRoot *os.Root,
	token string,
	cause error,
) error {
	if err := ticketRoot.Remove(token); err != nil {
		removeErr := fmt.Errorf(
			"remove reconciliation recovery token %q: %w",
			token,
			err,
		)
		if cause == nil {
			return removeErr
		}
		return errors.Join(cause, removeErr)
	}
	return cause
}

func validatePolicyJournalTarget(
	scope ScopeLayout,
	scopeRoot *os.Root,
	target string,
	ticketID string,
) (*os.Root, string, string, error) {
	if ticketID == "" {
		return nil, "", "", errors.New(
			"reconciliation recovery step lacks immutable ticket identity",
		)
	}
	relative, err := filepath.Rel(scope.TicketsRoot(), target)
	if err != nil ||
		relative == "." ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(
			relative,
			".."+string(filepath.Separator),
		) {
		return nil, "", "", fmt.Errorf(
			"reconciliation recovery target escapes its scope: %q",
			target,
		)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" {
		return nil, "", "", fmt.Errorf(
			"reconciliation recovery target lacks a ticket path: %q",
			target,
		)
	}

	info, err := scopeRoot.Lstat(parts[0])
	if err != nil {
		return nil, "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, "", "", errors.New(
			"reconciliation recovery ticket root is unsafe",
		)
	}
	ticketRoot, err := scopeRoot.OpenRoot(parts[0])
	if err != nil {
		return nil, "", "", err
	}
	fail := func(cause error) (*os.Root, string, string, error) {
		_ = ticketRoot.Close()
		return nil, "", "", cause
	}
	openedInfo, err := ticketRoot.Stat(".")
	if err != nil || !os.SameFile(info, openedInfo) {
		if err != nil {
			return fail(err)
		}
		return fail(errors.New(
			"reconciliation recovery ticket root changed while opening",
		))
	}
	statusBytes, err := readRootedRegularFile(ticketRoot, "status.yaml")
	if err != nil {
		return fail(err)
	}
	manifest, err := DecodeManifest(bytes.NewReader(statusBytes))
	if err != nil {
		return fail(err)
	}
	if manifest.ID != ticketID {
		return fail(errors.New(
			"reconciliation recovery ticket identity changed",
		))
	}

	ticketRelative := filepath.Join(parts[1:]...)
	_, _, closeTargetParent, err := openSafeRootParent(
		ticketRoot,
		ticketRelative,
	)
	if err != nil {
		return fail(err)
	}
	closeTargetParent()
	return ticketRoot, relative, ticketRelative, nil
}

func reconstructReconcileSteps(
	plan journal.Plan,
	evaluations []policyEvaluation,
) ([]reconcileStep, error) {
	contentByTarget := make(map[string][]byte)
	for _, evaluation := range evaluations {
		if evaluation.candidate.located == nil {
			continue
		}
		layout := evaluation.candidate.located.Layout
		for _, file := range evaluation.request.DesiredRender.Files {
			target := filepath.Join(
				layout.Root(),
				filepath.FromSlash(file.Path),
			)
			contentByTarget[target] = append([]byte(nil), file.Content...)
		}
	}
	result := make([]reconcileStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		prepared := reconcileStep{journal: step}
		ticketRoot, ticketID, relative, locationErr :=
			reconcileStepLocation(step, evaluations)
		if locationErr != nil {
			return nil, locationErr
		}
		prepared.ticketRoot = ticketRoot
		prepared.ticketID = ticketID
		prepared.relative = relative
		switch step.Action {
		case "create-directory":
		case "create-file", "replace-generated":
			content, exists := contentByTarget[step.Target]
			if !exists &&
				step.Action == "create-file" &&
				step.AfterHash == journal.Digest(nil) {
				content = []byte{}
				exists = true
			}
			if !exists || journal.Digest(content) != step.AfterHash {
				return nil, errors.New(
					"cannot reconstruct exact reconciliation file content",
				)
			}
			prepared.content = content
		case "update-render-manifest":
			content, err := reconstructRenderManifest(
				step,
				plan.Steps,
				evaluations,
			)
			if err != nil {
				return nil, err
			}
			prepared.content = content
		default:
			return nil, fmt.Errorf(
				"unsupported reconciliation journal action %q",
				step.Action,
			)
		}
		result = append(result, prepared)
	}
	return result, nil
}

func reconcileStepLocation(
	step journal.Step,
	evaluations []policyEvaluation,
) (string, string, string, error) {
	if step.TicketID == "" {
		return "", "", "", errors.New(
			"reconciliation step lacks immutable ticket identity",
		)
	}
	for _, evaluation := range evaluations {
		if evaluation.candidate.located == nil {
			continue
		}
		root := evaluation.candidate.located.Layout.Root()
		relative, err := filepath.Rel(root, step.Target)
		if err != nil ||
			relative == ".." ||
			filepath.IsAbs(relative) ||
			strings.HasPrefix(
				relative,
				".."+string(filepath.Separator),
			) {
			continue
		}
		if evaluation.candidate.located.Manifest.ID != step.TicketID {
			return "", "", "", errors.New(
				"reconciliation ticket identity changed after planning",
			)
		}
		return root, evaluation.candidate.located.Manifest.ID, relative, nil
	}
	return "", "", "", fmt.Errorf(
		"reconciliation target %q is outside evaluated tickets",
		step.Target,
	)
}

func reconstructRenderManifest(
	renderStep journal.Step,
	steps []journal.Step,
	evaluations []policyEvaluation,
) ([]byte, error) {
	for _, evaluation := range evaluations {
		if evaluation.candidate.located == nil ||
			filepath.Clean(
				evaluation.candidate.located.Layout.RenderManifestPath(),
			) != filepath.Clean(renderStep.Target) {
			continue
		}
		ticketRoot, closeTicketRoot, err := openLocatedTicketRoot(
			*evaluation.candidate.located,
		)
		if err != nil {
			return nil, err
		}
		content, readErr := readRootedRegularFile(
			ticketRoot,
			filepath.Join(".aidb", "rendered.yaml"),
		)
		closeTicketRoot()
		if readErr != nil {
			return nil, readErr
		}
		current, decodeErr := profile.DecodeRenderManifest(
			bytes.NewReader(content),
		)
		if decodeErr != nil {
			return nil, decodeErr
		}
		desired := make(map[string]profile.RenderedRecord)
		for _, record := range evaluation.request.DesiredRender.Manifest.Artifacts {
			desired[record.Role] = record
		}
		repairedRoles := make(map[string]struct{})
		for _, step := range steps {
			if step.Action != "create-file" &&
				step.Action != "replace-generated" {
				continue
			}
			for _, artifact := range evaluation.request.ActiveProfile.Artifacts {
				target := filepath.Join(
					evaluation.candidate.located.Layout.Root(),
					filepath.FromSlash(artifact.Path),
				)
				if filepath.Clean(target) == filepath.Clean(step.Target) {
					repairedRoles[artifact.Role] = struct{}{}
				}
			}
		}
		for index, record := range current.Artifacts {
			if _, repaired := repairedRoles[record.Role]; !repaired {
				continue
			}
			current.Artifacts[index] = desired[record.Role]
			delete(repairedRoles, record.Role)
		}
		for role := range repairedRoles {
			current.Artifacts = append(current.Artifacts, desired[role])
		}
		sort.Slice(current.Artifacts, func(left int, right int) bool {
			return current.Artifacts[left].Role <
				current.Artifacts[right].Role
		})
		encoded, err := encodeRenderManifest(current)
		if err != nil {
			return nil, err
		}
		if journal.Digest(encoded) != renderStep.AfterHash {
			return nil, errors.New(
				"reconstructed render manifest does not match journal plan",
			)
		}
		return encoded, nil
	}
	return nil, errors.New("render manifest ticket is unavailable")
}

func (service *Service) applyPolicyReconcile(
	scope ScopeLayout,
	operationID string,
	planHash string,
	store *journal.Store,
	plannedSteps []journal.Step,
	prepare func() ([]PolicyTicket, []reconcileStep, error),
) (capability.Result[PolicyReconcileData], error) {
	var tickets []PolicyTicket
	var steps []reconcileStep
	alreadyCommitted := false
	applyErr := withWorkspaceMutationLock(scope, func() error {
		state, err := store.InspectRecorded(operationID)
		if err != nil {
			return err
		}
		if state.Status == journal.StatusCommitted {
			alreadyCommitted = true
			return nil
		}
		if state.Status == journal.StatusAttention {
			return state.Error()
		}
		preparedTickets, preparedSteps, err := prepare()
		if err != nil {
			return appendPolicyReconcileFailure(
				store,
				operationID,
				err,
			)
		}
		if !reflect.DeepEqual(
			journalSteps(preparedSteps),
			plannedSteps,
		) {
			return appendPolicyReconcileFailure(
				store,
				operationID,
				journal.ErrPlanConflict,
			)
		}
		tickets = preparedTickets
		steps = preparedSteps
		roots, err := openReconcileRoots(scope, steps)
		if err != nil {
			return appendPolicyReconcileFailure(
				store,
				operationID,
				err,
			)
		}
		defer roots.Close()
		if err := preflightReconcileSteps(roots, steps); err != nil {
			return appendPolicyReconcileFailure(
				store,
				operationID,
				err,
			)
		}
		if _, err := store.Append(operationID, journal.EventInput{
			Phase: journal.PhaseApplying,
		}); err != nil {
			return err
		}
		for _, step := range steps {
			if err := service.applyReconcileStep(
				roots,
				store,
				operationID,
				step,
			); err != nil {
				return appendPolicyReconcileFailure(
					store,
					operationID,
					err,
				)
			}
		}
		if _, err := store.Append(operationID, journal.EventInput{
			Phase: journal.PhaseCommitted,
		}); err != nil {
			return err
		}
		return nil
	})
	if alreadyCommitted {
		return policyReconcileResult(
			capability.OutcomeUnchanged,
			operationID,
			planHash,
			[]PolicyTicket{},
			effectsFromJournal(plannedSteps, capability.EffectSkipped),
		), nil
	}
	effects := effectsFromReconcileSteps(steps, capability.EffectPlanned)
	if applyErr != nil {
		result := policyReconcileResult(
			capability.OutcomeFailed,
			operationID,
			planHash,
			tickets,
			failedEffects(effects),
		)
		result.Recovery = capability.Recovery{
			Required: true,
			Guidance: []string{
				"Inspect the durable reconciliation journal and retry the same operation id.",
			},
		}
		return result, applyErr
	}
	return policyReconcileResult(
		capability.OutcomeApplied,
		operationID,
		planHash,
		tickets,
		effectsFromReconcileSteps(steps, capability.EffectApplied),
	), nil
}

func appendPolicyReconcileFailure(
	store *journal.Store,
	operationID string,
	cause error,
) error {
	_, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseFailed,
		Error: &journal.ErrorInfo{
			Code:    PolicyReconcileDescriptor.Capability + ".failed",
			Message: cause.Error(),
		},
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

type openedReconcileRoots struct {
	scope     *os.Root
	scopePath string
	tickets   map[string]*os.Root
	parents   map[string]*os.Root
}

func openReconcileRoots(
	scope ScopeLayout,
	steps []reconcileStep,
) (*openedReconcileRoots, error) {
	scopeRoot, err := openScopeTicketsRoot(scope)
	if err != nil {
		return nil, err
	}
	result := &openedReconcileRoots{
		scope:     scopeRoot,
		scopePath: scope.TicketsRoot(),
		tickets:   make(map[string]*os.Root),
		parents:   make(map[string]*os.Root),
	}
	fail := func(cause error) (*openedReconcileRoots, error) {
		result.Close()
		return nil, cause
	}
	for _, step := range steps {
		if step.ticketRoot == "" ||
			step.ticketID == "" ||
			step.relative == "" {
			return fail(errors.New(
				"reconciliation step lacks rooted ticket identity",
			))
		}
		if _, exists := result.tickets[step.ticketRoot]; exists {
			continue
		}
		relative, err := filepath.Rel(
			scope.TicketsRoot(),
			step.ticketRoot,
		)
		if err != nil ||
			relative == "." ||
			relative == ".." ||
			filepath.IsAbs(relative) ||
			filepath.Dir(relative) != "." ||
			strings.HasPrefix(
				relative,
				".."+string(filepath.Separator),
			) {
			return fail(fmt.Errorf(
				"reconciliation ticket root escapes its scope: %q",
				step.ticketRoot,
			))
		}
		info, err := scopeRoot.Lstat(relative)
		if err != nil {
			return fail(err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fail(fmt.Errorf(
				"reconciliation ticket root %q is unsafe",
				step.ticketRoot,
			))
		}
		ticketRoot, err := scopeRoot.OpenRoot(relative)
		if err != nil {
			return fail(err)
		}
		openedInfo, err := ticketRoot.Stat(".")
		if err != nil || !os.SameFile(info, openedInfo) {
			_ = ticketRoot.Close()
			if err != nil {
				return fail(err)
			}
			return fail(errors.New(
				"reconciliation ticket root changed while opening",
			))
		}
		statusBytes, err := readRootedRegularFile(
			ticketRoot,
			"status.yaml",
		)
		if err != nil {
			_ = ticketRoot.Close()
			return fail(err)
		}
		manifest, err := DecodeManifest(bytes.NewReader(statusBytes))
		if err != nil || manifest.ID != step.ticketID {
			_ = ticketRoot.Close()
			if err != nil {
				return fail(err)
			}
			return fail(errors.New(
				"reconciliation ticket identity changed after preview",
			))
		}
		result.tickets[step.ticketRoot] = ticketRoot
	}
	return result, nil
}

func (roots *openedReconcileRoots) Close() {
	if roots == nil {
		return
	}
	for _, root := range roots.parents {
		_ = root.Close()
	}
	for _, root := range roots.tickets {
		_ = root.Close()
	}
	if roots.scope != nil {
		_ = roots.scope.Close()
	}
}

func (roots *openedReconcileRoots) parentForStep(
	step reconcileStep,
) (*os.Root, string, error) {
	ticketRoot, exists := roots.tickets[step.ticketRoot]
	if !exists {
		return nil, "", errors.New(
			"reconciliation step ticket root is not open",
		)
	}
	cleaned := filepath.Clean(step.relative)
	parentPath := filepath.Dir(cleaned)
	base := filepath.Base(cleaned)
	if parentPath == "." {
		return ticketRoot, base, nil
	}
	key := step.ticketRoot + "\x00" + parentPath
	if parent, ok := roots.parents[key]; ok {
		return parent, base, nil
	}
	parent, openedBase, owned, err := openSafeRootParentHandle(
		ticketRoot,
		cleaned,
	)
	if err != nil {
		return nil, "", err
	}
	if openedBase != base {
		if owned {
			_ = parent.Close()
		}
		return nil, "", errors.New(
			"reconciliation rooted parent resolved an unexpected base",
		)
	}
	if !owned {
		return ticketRoot, base, nil
	}
	roots.parents[key] = parent
	return parent, base, nil
}

func (roots *openedReconcileRoots) scopedTargetForStep(
	step reconcileStep,
) (string, error) {
	if roots == nil || roots.scope == nil || roots.scopePath == "" {
		return "", errors.New(
			"reconciliation scope root is not open",
		)
	}
	relative, err := filepath.Rel(roots.scopePath, step.journal.Target)
	if err != nil ||
		relative == "." ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(
			relative,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf(
			"reconciliation target escapes its open scope: %q",
			step.journal.Target,
		)
	}
	if filepath.Clean(
		filepath.Join(roots.scopePath, relative),
	) != filepath.Clean(step.journal.Target) {
		return "", errors.New(
			"reconciliation scoped target differs from journal target",
		)
	}
	return relative, nil
}

func (roots *openedReconcileRoots) validateParentForStep(
	step reconcileStep,
	held *os.Root,
) error {
	ticketRoot, exists := roots.tickets[step.ticketRoot]
	if !exists {
		return errors.New("reconciliation step ticket root is not open")
	}
	current, _, closeCurrent, err := openSafeRootParent(
		ticketRoot,
		step.relative,
	)
	if err != nil {
		return err
	}
	defer closeCurrent()
	currentInfo, err := current.Stat(".")
	if err != nil {
		return err
	}
	heldInfo, err := held.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(currentInfo, heldInfo) {
		return errors.New(
			"reconciliation rooted parent changed after preflight",
		)
	}
	return nil
}

func (roots *openedReconcileRoots) validateScopedBindingForStep(
	step reconcileStep,
	heldParent *os.Root,
) error {
	scopedTarget, err := roots.scopedTargetForStep(step)
	if err != nil {
		return err
	}
	parts := strings.Split(
		filepath.Clean(scopedTarget),
		string(filepath.Separator),
	)
	if len(parts) < 2 || parts[0] == "" {
		return errors.New(
			"reconciliation scoped target lacks a ticket path",
		)
	}
	heldTicket, exists := roots.tickets[step.ticketRoot]
	if !exists {
		return errors.New("reconciliation step ticket root is not open")
	}
	if err := validateScopedTicketIdentity(
		roots.scope,
		heldTicket,
		parts[0],
		step.ticketID,
	); err != nil {
		return err
	}
	currentParent, _, closeCurrent, err := openSafeRootParent(
		roots.scope,
		scopedTarget,
	)
	if err != nil {
		return err
	}
	defer closeCurrent()
	currentInfo, err := currentParent.Stat(".")
	if err != nil {
		return err
	}
	heldInfo, err := heldParent.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(currentInfo, heldInfo) {
		return errors.New(
			"reconciliation logical parent changed after validation hook",
		)
	}
	return nil
}

func validateScopedTicketIdentity(
	scopeRoot *os.Root,
	heldTicket *os.Root,
	ticketName string,
	ticketID string,
) error {
	if ticketName == "" || ticketID == "" {
		return errors.New(
			"reconciliation ticket binding is incomplete",
		)
	}
	logicalInfo, err := scopeRoot.Lstat(ticketName)
	if err != nil {
		return err
	}
	if logicalInfo.Mode()&os.ModeSymlink != 0 || !logicalInfo.IsDir() {
		return errors.New(
			"reconciliation logical ticket root is unsafe",
		)
	}
	heldInfo, err := heldTicket.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(logicalInfo, heldInfo) {
		return errors.New(
			"reconciliation logical ticket root changed after validation",
		)
	}
	current, err := scopeRoot.OpenRoot(ticketName)
	if err != nil {
		return err
	}
	defer current.Close()
	currentInfo, err := current.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(logicalInfo, currentInfo) {
		return errors.New(
			"reconciliation logical ticket root changed while reopening",
		)
	}
	statusBytes, err := readRootedRegularFile(current, "status.yaml")
	if err != nil {
		return err
	}
	manifest, err := DecodeManifest(bytes.NewReader(statusBytes))
	if err != nil {
		return err
	}
	if manifest.ID != ticketID {
		return errors.New(
			"reconciliation logical ticket identity changed after validation",
		)
	}
	return nil
}

func preflightReconcileSteps(
	roots *openedReconcileRoots,
	steps []reconcileStep,
) error {
	plannedDirectories := make(map[string][]string)
	for _, step := range steps {
		if step.journal.Action == "create-directory" {
			plannedDirectories[step.ticketRoot] = append(
				plannedDirectories[step.ticketRoot],
				filepath.Clean(step.relative),
			)
		}
	}
	for _, step := range steps {
		root, exists := roots.tickets[step.ticketRoot]
		if !exists {
			return errors.New(
				"reconciliation step ticket root is not open",
			)
		}
		if filepath.Clean(
			filepath.Join(step.ticketRoot, step.relative),
		) != filepath.Clean(step.journal.Target) {
			return errors.New(
				"reconciliation journal target differs from rooted target",
			)
		}
		switch step.journal.Action {
		case "create-directory":
			if _, err := inspectRootedDirectoryPath(
				root,
				step.relative,
			); err != nil {
				return err
			}
		case "create-file":
			parent, base, parentErr := roots.parentForStep(step)
			if parentErr != nil &&
				!errors.Is(parentErr, os.ErrNotExist) {
				return parentErr
			}
			var content []byte
			var err error
			if parentErr == nil {
				content, _, err = readRegularAt(parent, base)
			} else {
				err = parentErr
			}
			if err == nil {
				if journal.Digest(content) != step.journal.AfterHash {
					return fmt.Errorf(
						"reconciliation create target %q changed after planning",
						step.journal.Target,
					)
				}
			} else if errors.Is(err, os.ErrNotExist) {
				parent := filepath.Dir(step.relative)
				if parent != "." {
					exists, parentErr := inspectRootedDirectoryPath(
						root,
						parent,
					)
					if parentErr != nil {
						return parentErr
					}
					if !exists &&
						!plannedDirectoryCreatesParent(
							plannedDirectories[step.ticketRoot],
							parent,
						) {
						return fmt.Errorf(
							"reconciliation file parent %q is missing",
							parent,
						)
					}
				}
			} else {
				return err
			}
		case "replace-generated", "update-render-manifest":
			parent, base, err := roots.parentForStep(step)
			if err != nil {
				return err
			}
			content, _, err := readRegularAt(parent, base)
			if err != nil {
				return err
			}
			hash := journal.Digest(content)
			if hash != step.journal.BeforeHash &&
				hash != step.journal.AfterHash {
				return fmt.Errorf(
					"reconciliation target %q changed after planning",
					step.journal.Target,
				)
			}
		default:
			return fmt.Errorf(
				"unsupported reconciliation action %q",
				step.journal.Action,
			)
		}
	}
	return nil
}

func plannedDirectoryCreatesParent(
	planned []string,
	parent string,
) bool {
	parent = filepath.Clean(parent)
	for _, directory := range planned {
		directory = filepath.Clean(directory)
		if directory == parent ||
			strings.HasPrefix(
				directory,
				parent+string(filepath.Separator),
			) {
			return true
		}
	}
	return false
}

func (service *Service) applyReconcileStep(
	roots *openedReconcileRoots,
	store *journal.Store,
	operationID string,
	step reconcileStep,
) error {
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step.journal.Ordinal,
	}); err != nil {
		return err
	}
	ticketRoot, exists := roots.tickets[step.ticketRoot]
	if !exists {
		return errors.New("reconciliation step ticket root is not open")
	}
	scopedTarget, err := roots.scopedTargetForStep(step)
	if err != nil {
		return err
	}
	switch step.journal.Action {
	case "create-directory":
		if err := service.publishRootedDirectory(
			roots.scope,
			ticketRoot,
			scopedTarget,
			step.journal.Target,
			step.ticketID,
		); err != nil {
			return err
		}
	case "create-file":
		parent, base, err := roots.parentForStep(step)
		if err != nil {
			return err
		}
		if err := roots.validateParentForStep(step, parent); err != nil {
			return err
		}
		if err := service.publishBoundRootedFile(
			roots.scope,
			scopedTarget,
			parent,
			base,
			func() error {
				return roots.validateScopedBindingForStep(
					step,
					parent,
				)
			},
			operationID,
			step,
			false,
		); err != nil {
			return err
		}
	case "replace-generated", "update-render-manifest":
		parent, base, err := roots.parentForStep(step)
		if err != nil {
			return err
		}
		if err := roots.validateParentForStep(step, parent); err != nil {
			return err
		}
		if err := service.publishBoundRootedFile(
			roots.scope,
			scopedTarget,
			parent,
			base,
			func() error {
				return roots.validateScopedBindingForStep(
					step,
					parent,
				)
			},
			operationID,
			step,
			true,
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"unsupported reconciliation action %q",
			step.journal.Action,
		)
	}
	if service.afterFilesystemStage != nil {
		if err := service.afterFilesystemStage(step.journal.Action); err != nil {
			return err
		}
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step.journal.Ordinal,
	}); err != nil {
		return err
	}
	return nil
}

// publishRootedFile publishes a step's content through a single root, staging and
// publishing inside the same directory.
//
// Production reconciliation never takes this shape — it stages under the ticket
// root and publishes through the scope root, so it calls publishBoundRootedFile
// directly. This wrapper exists for tests that exercise the publication machinery
// against one root, which is why unparam sees its arguments as constant.
//
//nolint:unparam // test-only seam; the parameters mirror publishBoundRootedFile.
func (service *Service) publishRootedFile(
	parent *os.Root,
	base string,
	operationID string,
	step reconcileStep,
	requireBefore bool,
) error {
	return service.publishBoundRootedFile(
		parent,
		base,
		parent,
		base,
		nil,
		operationID,
		step,
		requireBefore,
	)
}

func (service *Service) publishBoundRootedFile(
	publicationRoot *os.Root,
	targetName string,
	stagingParent *os.Root,
	stagingBase string,
	validateBinding func() error,
	operationID string,
	step reconcileStep,
	requireBefore bool,
) (publishErr error) {
	firstGuardName := rootedSiblingPath(
		targetName,
		reconcilePublicationGuardName(
			operationID,
			step.journal.Ordinal,
		),
	)
	if requireBefore {
		settled, err := settleRootedPublicationGuard(
			publicationRoot,
			targetName,
			firstGuardName,
			step.journal,
		)
		if err != nil {
			return err
		}
		if settled {
			return nil
		}
	}
	current, _, readErr := readRegularAt(publicationRoot, targetName)
	switch {
	case readErr == nil &&
		journal.Digest(current) == step.journal.AfterHash:
		return nil
	case readErr == nil &&
		requireBefore &&
		journal.Digest(current) == step.journal.BeforeHash:
	case readErr == nil:
		return fmt.Errorf(
			"reconciliation target %q changed after planning",
			step.journal.Target,
		)
	case errors.Is(readErr, os.ErrNotExist) && !requireBefore:
	default:
		// readErr is necessarily non-nil here: the readErr == nil arms above are
		// exhaustive over a successful read. A missing target that the step
		// required (requireBefore) arrives here too, and reports its own
		// ErrNotExist — the same answer the preflight in this file gives for that
		// case, so the two agree on what a vanished target looks like.
		return readErr
	}

	temporaryBase := "." + stagingBase + ".tmp-" + uuid.NewString()
	temporaryName := rootedSiblingPath(targetName, temporaryBase)
	temporary, err := stagingParent.OpenFile(
		temporaryBase,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		return err
	}
	temporaryExists := true
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !temporaryExists {
			return
		}
		removeErr := stagingParent.Remove(temporaryBase)
		if removeErr == nil {
			return
		}
		// Report the leaked staging file alongside a failure, but never turn a
		// success into one. This function returns nil with the staging file
		// still present on the crash-recovery path (the target already carries
		// the after-hash), and failing that would make a target that is already
		// correct unresumable — a far worse outcome than a stray dot-file that
		// nothing reads.
		if publishErr != nil {
			publishErr = errors.Join(publishErr, fmt.Errorf(
				"remove reconciliation staging file %q: %w",
				temporaryBase,
				removeErr,
			))
		}
	}()
	if _, err := temporary.Write(step.content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	temporaryInfo, err := temporary.Stat()
	if err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true

	current, _, readErr = readRegularAt(publicationRoot, targetName)
	switch {
	case readErr == nil &&
		journal.Digest(current) == step.journal.AfterHash:
		return nil
	case readErr == nil &&
		requireBefore &&
		journal.Digest(current) == step.journal.BeforeHash:
	case errors.Is(readErr, os.ErrNotExist) && !requireBefore:
	case readErr != nil:
		return readErr
	default:
		return fmt.Errorf(
			"reconciliation target %q changed before publication",
			step.journal.Target,
		)
	}
	if service.beforeRootedPublish != nil {
		if err := service.beforeRootedPublish(step.journal.Target); err != nil {
			return err
		}
	}
	if validateBinding != nil {
		if err := validateBinding(); err != nil {
			return err
		}
	}
	if err := verifyRootedPublishedFile(
		publicationRoot,
		temporaryName,
		temporaryInfo,
		step.journal.AfterHash,
	); err != nil {
		return err
	}
	if requireBefore {
		if err := service.replaceRootedFileNoClobber(
			publicationRoot,
			targetName,
			temporaryName,
			temporaryInfo,
			operationID,
			step,
		); err != nil {
			return err
		}
	} else {
		if err := publishRootedContentExclusive(
			publicationRoot,
			targetName,
			step.content,
			step.journal.AfterHash,
		); err != nil {
			return err
		}
	}
	if err := stagingParent.Remove(temporaryBase); err != nil {
		return err
	}
	temporaryExists = false
	if err := syncRootedParent(publicationRoot, targetName); err != nil {
		return err
	}
	return nil
}

func publishRootedContentExclusive(
	root *os.Root,
	targetName string,
	content []byte,
	expectedHash string,
) error {
	return publishRootedContentExclusiveWithWrite(
		root,
		targetName,
		content,
		expectedHash,
		func(target *os.File, content []byte) (int, error) {
			return target.Write(content)
		},
	)
}

func publishRootedContentExclusiveWithWrite(
	root *os.Root,
	targetName string,
	content []byte,
	expectedHash string,
	write func(*os.File, []byte) (int, error),
) (resultErr error) {
	if write == nil {
		return errors.New("reconciliation rooted writer is required")
	}
	target, err := root.OpenFile(
		targetName,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		return err
	}
	createdInfo, err := target.Stat()
	if err != nil {
		_ = target.Close()
		return err
	}
	closed := false
	committed := false
	defer func() {
		if !closed {
			if closeErr := target.Close(); closeErr != nil {
				resultErr = errors.Join(resultErr, closeErr)
			}
			closed = true
		}
		if !committed {
			if cleanupErr := removeRootedFileIfSame(
				root,
				targetName,
				createdInfo,
			); cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()
	written, err := write(target, content)
	if err != nil {
		return err
	}
	if written != len(content) {
		return io.ErrShortWrite
	}
	if err := target.Sync(); err != nil {
		return err
	}
	targetInfo, err := target.Stat()
	if err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := verifyRootedPublishedFile(
		root,
		targetName,
		targetInfo,
		expectedHash,
	); err != nil {
		return err
	}
	if err := syncRootedParent(root, targetName); err != nil {
		return err
	}
	committed = true
	return nil
}

func removeRootedFileIfSame(
	root *os.Root,
	targetName string,
	expected os.FileInfo,
) error {
	current, err := root.Lstat(targetName)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 ||
		!current.Mode().IsRegular() ||
		expected == nil ||
		!os.SameFile(expected, current) {
		return nil
	}
	file, err := root.Open(targetName)
	if err != nil {
		return err
	}
	opened, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !os.SameFile(expected, opened) {
		return nil
	}
	current, err = root.Lstat(targetName)
	if err != nil {
		return err
	}
	if !os.SameFile(expected, current) {
		return nil
	}
	if err := root.Remove(targetName); err != nil {
		return err
	}
	return syncRootedParent(root, targetName)
}

func verifyRootedPublishedFile(
	root *os.Root,
	targetName string,
	expectedSource os.FileInfo,
	expectedHash string,
) error {
	content, targetInfo, err := readRegularAt(root, targetName)
	if err != nil {
		return err
	}
	if expectedSource == nil ||
		!os.SameFile(expectedSource, targetInfo) ||
		journal.Digest(content) != expectedHash {
		return fmt.Errorf(
			"reconciliation publication source changed for %q",
			filepath.Join(root.Name(), targetName),
		)
	}
	return nil
}

func rootedSiblingPath(targetName string, siblingBase string) string {
	parent := filepath.Dir(targetName)
	if parent == "." {
		return siblingBase
	}
	return filepath.Join(parent, siblingBase)
}

func syncRootedParent(root *os.Root, targetName string) error {
	parentName := filepath.Dir(targetName)
	if parentName == "." {
		return atomicfile.SyncDirectory(root.Name())
	}
	parent, err := root.OpenRoot(parentName)
	if err != nil {
		return err
	}
	defer parent.Close()
	return atomicfile.SyncDirectory(parent.Name())
}

func (service *Service) publishRootedDirectory(
	scopeRoot *os.Root,
	ticketRoot *os.Root,
	scopedTarget string,
	target string,
	ticketID string,
) error {
	cleaned := filepath.Clean(scopedTarget)
	if cleaned == "." || filepath.IsAbs(cleaned) {
		return fmt.Errorf(
			"reconciliation directory target is invalid: %q",
			target,
		)
	}
	parts := strings.Split(cleaned, string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" {
		return fmt.Errorf(
			"reconciliation directory target lacks a ticket path: %q",
			target,
		)
	}
	if err := validateScopedTicketIdentity(
		scopeRoot,
		ticketRoot,
		parts[0],
		ticketID,
	); err != nil {
		return err
	}

	current := ticketRoot
	currentOwned := false
	defer func() {
		if currentOwned {
			_ = current.Close()
		}
	}()
	logicalParent := parts[0]
	for _, component := range parts[1:] {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf(
				"reconciliation directory component is invalid: %q",
				component,
			)
		}
		logicalChild := filepath.Join(logicalParent, component)
		childInfo, childErr := current.Lstat(component)
		switch {
		case childErr == nil:
			if childInfo.Mode()&os.ModeSymlink != 0 ||
				!childInfo.IsDir() {
				return fmt.Errorf(
					"reconciliation directory target is unsafe: %q",
					target,
				)
			}
			logicalInfo, err := scopeRoot.Lstat(logicalChild)
			if err != nil || !os.SameFile(childInfo, logicalInfo) {
				if err != nil {
					return err
				}
				return errors.New(
					"reconciliation directory parent changed after preflight",
				)
			}
		case errors.Is(childErr, os.ErrNotExist):
			temporaryBase := "." + component + ".tmp-" + uuid.NewString()
			if err := current.Mkdir(temporaryBase, 0o755); err != nil {
				return err
			}
			// Bound to this iteration's root: current is reassigned as the walk
			// descends, and the staging directory belongs to the level that
			// created it.
			stagingRoot := current
			temporaryExists := true
			// failStaging removes the staging directory this iteration created
			// and reports a failed removal alongside cause. The directory is
			// empty and named ".<component>.tmp-<uuid>", so nothing reads it —
			// but it is durable state inside a ticket tree that only this code
			// path knows about, so its removal failing is the operator's only
			// notice that the tree was left dirty.
			failStaging := func(cause error) error {
				if !temporaryExists {
					return cause
				}
				if err := stagingRoot.Remove(temporaryBase); err != nil {
					return errors.Join(cause, fmt.Errorf(
						"remove reconciliation staging directory %q: %w",
						temporaryBase,
						err,
					))
				}
				return cause
			}
			temporaryInfo, err := current.Lstat(temporaryBase)
			if err != nil {
				return failStaging(err)
			}
			if service.beforeRootedPublish != nil {
				if err := service.beforeRootedPublish(target); err != nil {
					return failStaging(err)
				}
			}
			if err := validateScopedTicketIdentity(
				scopeRoot,
				ticketRoot,
				parts[0],
				ticketID,
			); err != nil {
				return failStaging(err)
			}
			logicalTemporary := filepath.Join(
				logicalParent,
				temporaryBase,
			)
			logicalTemporaryInfo, err := scopeRoot.Lstat(
				logicalTemporary,
			)
			if err != nil ||
				!os.SameFile(temporaryInfo, logicalTemporaryInfo) {
				if err != nil {
					return failStaging(err)
				}
				return failStaging(errors.New(
					"reconciliation directory staging source changed",
				))
			}
			if err := scopeRoot.Rename(
				logicalTemporary,
				logicalChild,
			); err != nil {
				return failStaging(err)
			}
			temporaryExists = false
			if err := syncRootedParent(scopeRoot, logicalChild); err != nil {
				return err
			}
			childInfo, err = current.Lstat(component)
			if err != nil {
				return err
			}
			logicalInfo, err := scopeRoot.Lstat(logicalChild)
			if err != nil || !os.SameFile(childInfo, logicalInfo) {
				if err != nil {
					return err
				}
				return errors.New(
					"reconciliation directory publication changed target",
				)
			}
		default:
			return childErr
		}

		next, err := current.OpenRoot(component)
		if err != nil {
			return err
		}
		openedInfo, err := next.Stat(".")
		if err != nil || !os.SameFile(childInfo, openedInfo) {
			_ = next.Close()
			if err != nil {
				return err
			}
			return errors.New(
				"reconciliation directory changed while opening",
			)
		}
		if currentOwned {
			_ = current.Close()
		}
		current = next
		currentOwned = true
		logicalParent = logicalChild
	}
	return nil
}

func (service *Service) replaceRootedFileNoClobber(
	root *os.Root,
	targetName string,
	temporaryName string,
	temporaryInfo os.FileInfo,
	operationID string,
	step reconcileStep,
) error {
	firstGuardName := rootedSiblingPath(
		targetName,
		reconcilePublicationGuardName(
			operationID,
			step.journal.Ordinal,
		),
	)
	if _, err := root.Lstat(firstGuardName); err == nil {
		settled, settleErr := settleRootedPublicationGuard(
			root,
			targetName,
			firstGuardName,
			step.journal,
		)
		if settleErr != nil {
			return settleErr
		}
		if settled {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	guardName, err := reserveRootedPublicationGuard(
		root,
		temporaryName,
		firstGuardName,
		temporaryInfo,
		step.journal.AfterHash,
	)
	if err != nil {
		return err
	}
	if service.beforeGuardMove != nil {
		if err := service.beforeGuardMove(
			filepath.Join(root.Name(), guardName),
		); err != nil {
			return err
		}
	}

	if err := root.Rename(targetName, guardName); err != nil {
		return err
	}
	if err := syncRootedParent(root, targetName); err != nil {
		return err
	}
	if service.afterRootedDisplace != nil {
		if err := service.afterRootedDisplace(step.journal.Target); err != nil {
			return err
		}
	}

	displaced, _, err := readRegularAt(root, guardName)
	if err != nil {
		return err
	}
	switch displacedHash := journal.Digest(displaced); displacedHash {
	case step.journal.AfterHash:
		settled, settleErr := settleRootedPublicationGuard(
			root,
			targetName,
			guardName,
			step.journal,
		)
		if settleErr != nil {
			return settleErr
		}
		if !settled {
			return errors.New(
				"reconciliation desired target could not be restored",
			)
		}
		return nil
	case step.journal.BeforeHash:
	default:
		restoreErr := publishRootedContentExclusive(
			root,
			targetName,
			displaced,
			displacedHash,
		)
		changedErr := fmt.Errorf(
			"reconciliation target %q changed immediately before publication",
			step.journal.Target,
		)
		if restoreErr != nil {
			return errors.Join(changedErr, restoreErr)
		}
		return changedErr
	}

	if err := publishRootedContentExclusive(
		root,
		targetName,
		step.content,
		step.journal.AfterHash,
	); err != nil {
		_, settleErr := settleRootedPublicationGuard(
			root,
			targetName,
			guardName,
			step.journal,
		)
		if settleErr != nil {
			return errors.Join(err, settleErr)
		}
		return err
	}
	if service.beforeGuardRelease != nil {
		if err := service.beforeGuardRelease(step.journal.Target); err != nil {
			return err
		}
	}
	return nil
}

func settleRootedPublicationGuard(
	root *os.Root,
	targetName string,
	guardName string,
	step journal.Step,
) (bool, error) {
	guard, guardInfo, err := readRegularAt(root, guardName)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return settleRootedPublicationSource(
		root,
		targetName,
		guardName,
		guardName,
		guardInfo,
		guard,
		journal.Digest(guard),
		step,
	)
}

func settleRootedPublicationSource(
	root *os.Root,
	targetName string,
	sourceName string,
	guardName string,
	sourceInfo os.FileInfo,
	sourceContent []byte,
	sourceHash string,
	step journal.Step,
) (bool, error) {
	if err := verifyRootedPublishedFile(
		root,
		sourceName,
		sourceInfo,
		sourceHash,
	); err != nil {
		return false, err
	}
	if sourceHash != step.BeforeHash && sourceHash != step.AfterHash {
		return false, fmt.Errorf(
			"reconciliation recovery guard %q contains unplanned content",
			filepath.Join(root.Name(), guardName),
		)
	}
	current, currentInfo, currentErr := readRegularAt(root, targetName)
	if errors.Is(currentErr, os.ErrNotExist) {
		if err := publishRootedContentExclusive(
			root,
			targetName,
			sourceContent,
			sourceHash,
		); err != nil {
			return false, err
		}
		if err := syncRootedParent(root, targetName); err != nil {
			return false, err
		}
		switch sourceHash {
		case step.AfterHash:
			return true, nil
		case step.BeforeHash:
			return false, nil
		default:
			return false, fmt.Errorf(
				"reconciliation restored unexpected content for %q",
				step.Target,
			)
		}
	}
	if currentErr != nil {
		return false, currentErr
	}
	currentHash := journal.Digest(current)

	if os.SameFile(sourceInfo, currentInfo) {
		switch currentHash {
		case step.AfterHash:
			return true, nil
		case step.BeforeHash:
			return false, nil
		default:
			return false, fmt.Errorf(
				"reconciliation target %q contains unexpected content",
				step.Target,
			)
		}
	}
	switch {
	case currentHash == step.AfterHash &&
		(sourceHash == step.BeforeHash || sourceHash == step.AfterHash):
		return true, nil
	case currentHash == step.BeforeHash &&
		sourceHash == step.AfterHash:
		return false, nil
	case currentHash == step.BeforeHash &&
		sourceHash == step.BeforeHash:
		return false, nil
	case sourceHash == step.BeforeHash:
		return false, fmt.Errorf(
			"reconciliation target %q was concurrently replaced; recovery guard %q was retained",
			step.Target,
			filepath.Join(root.Name(), guardName),
		)
	default:
		return false, fmt.Errorf(
			"reconciliation target %q and recovery guard %q both require review",
			step.Target,
			filepath.Join(root.Name(), guardName),
		)
	}
}

func reserveRootedPublicationGuard(
	root *os.Root,
	temporaryName string,
	first string,
	expectedSource os.FileInfo,
	expectedHash string,
) (string, error) {
	for attempt := 0; ; attempt++ {
		candidate := first
		if attempt > 0 {
			candidate += "." + strconv.Itoa(attempt)
		}
		err := root.Link(temporaryName, candidate)
		if err == nil {
			if verifyErr := verifyRootedPublishedFile(
				root,
				candidate,
				expectedSource,
				expectedHash,
			); verifyErr != nil {
				return "", verifyErr
			}
			if syncErr := syncRootedParent(root, candidate); syncErr != nil {
				return "", syncErr
			}
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}

func reconcilePublicationGuardName(
	operationID string,
	ordinal int,
) string {
	return ".aidb-reconcile-" + operationID + "-" +
		strconv.Itoa(ordinal) + ".previous"
}

func policyTicketMatches(ticket PolicyTicket, selector string) bool {
	if selector == "" {
		return false
	}
	if filepath.Clean(ticket.Path) == filepath.Clean(selector) {
		return true
	}
	if ticket.Ticket == nil {
		return filepath.Base(ticket.Path) == selector
	}
	manifest := ticket.Ticket.Manifest
	if selector == manifest.ID ||
		selector == manifest.LocalKey ||
		selector == manifest.VisibleKey ||
		selector == filepath.Base(ticket.Path) {
		return true
	}
	for _, alias := range manifest.Aliases {
		if selector == alias {
			return true
		}
	}
	return false
}

func policyTickets(evaluations []policyEvaluation) []PolicyTicket {
	result := make([]PolicyTicket, 0, len(evaluations))
	for _, evaluation := range evaluations {
		result = append(result, evaluation.ticket)
	}
	return result
}

func conformanceOutcome(tickets []PolicyTicket) capability.Outcome {
	for _, ticket := range tickets {
		if ticket.Report.Blocked || ticket.Report.Attention {
			return capability.OutcomeAttention
		}
	}
	return capability.OutcomeHealthy
}

func policyIdempotencyKey(scope ScopeLayout) string {
	return PolicyReconcileDescriptor.Capability + ":" +
		string(scope.Kind()) + ":" +
		scope.OrganizationID() + ":" +
		scope.RepositoryID()
}

func validatePolicyOperationID(operationID string) error {
	parsed, err := uuid.Parse(operationID)
	if err != nil || parsed.String() != operationID {
		return errors.New(
			"policy reconciliation operation id must be a canonical UUID",
		)
	}
	return nil
}

func journalSteps(steps []reconcileStep) []journal.Step {
	result := make([]journal.Step, 0, len(steps))
	for _, step := range steps {
		result = append(result, step.journal)
	}
	return result
}

func effectsFromReconcileSteps(
	steps []reconcileStep,
	status capability.EffectStatus,
) []capability.Effect {
	return effectsFromJournal(journalSteps(steps), status)
}

func effectsFromJournal(
	steps []journal.Step,
	status capability.EffectStatus,
) []capability.Effect {
	result := make([]capability.Effect, 0, len(steps))
	for _, step := range steps {
		result = append(result, capability.Effect{
			Action: step.Action,
			Target: step.Target,
			Status: status,
		})
	}
	return result
}

func policyReconcileResult(
	outcome capability.Outcome,
	operationID string,
	planHash string,
	tickets []PolicyTicket,
	effects []capability.Effect,
) capability.Result[PolicyReconcileData] {
	result := capability.Result[PolicyReconcileData]{
		Capability: PolicyReconcileDescriptor.Capability,
		Version:    PolicyReconcileDescriptor.Version,
		Outcome:    outcome,
		Data: PolicyReconcileData{
			OperationID: operationID,
			PlanHash:    planHash,
			Tickets:     tickets,
		},
		Effects:     effects,
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
	if outcome == capability.OutcomePlanned {
		result.NextActions = []capability.Action{{
			Code:    "apply_policy_reconcile",
			Message: "Run policy reconcile with apply enabled and the same operation id.",
		}}
	}
	return result
}

func reconcilePlanHash(steps []journal.Step) string {
	content, err := json.Marshal(steps)
	if err != nil {
		panic(fmt.Sprintf("marshal reconciliation plan steps: %v", err))
	}
	return journal.Digest(content)
}

func emptyValidateConformanceResult() capability.Result[ValidateData] {
	return capability.Result[ValidateData]{
		Capability:  ValidateDescriptor.Capability,
		Version:     ValidateDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyPolicyCheckResult() capability.Result[PolicyCheckData] {
	return capability.Result[PolicyCheckData]{
		Capability:  PolicyCheckDescriptor.Capability,
		Version:     PolicyCheckDescriptor.Version,
		Data:        PolicyCheckData{Tickets: []PolicyTicket{}},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyPolicyReconcileResult() capability.Result[PolicyReconcileData] {
	return capability.Result[PolicyReconcileData]{
		Capability: PolicyReconcileDescriptor.Capability,
		Version:    PolicyReconcileDescriptor.Version,
		Data: PolicyReconcileData{
			Tickets: []PolicyTicket{},
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}
