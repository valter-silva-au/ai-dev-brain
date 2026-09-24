package v3cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

func TestOrganizationCommandSurfaceReplacesLegacyOrgWithoutLoadingApp(
	t *testing.T,
) {
	service := &fakeOrganization{
		listResult: capability.Result[organization.ListData]{
			Capability:  organization.ListDescriptor.Capability,
			Version:     organization.ListDescriptor.Version,
			Outcome:     capability.OutcomeHealthy,
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	loadCount := 0
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: service,
		LoadLegacyApp: func() error {
			loadCount++
			return nil
		},
	})

	count := 0
	for _, command := range root.Commands() {
		if command.Name() == "org" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("org command count = %d, want 1", count)
	}
	orgCommand, _, err := root.Find([]string{"org"})
	if err != nil {
		t.Fatalf("find org command: %v", err)
	}
	for _, name := range []string{
		"init",
		"list",
		"show",
		"update",
		"adopt",
		"move",
		"archive",
		"validate",
	} {
		child, _, err := orgCommand.Find([]string{name})
		if err != nil || child == nil || child.Name() != name {
			t.Fatalf("organization command %q is unavailable: %v", name, err)
		}
	}

	execute(
		t,
		root,
		"org",
		"list",
		"--workspace",
		t.TempDir(),
		"--format",
		"json",
	)
	if loadCount != 0 {
		t.Fatalf("organization command loaded legacy app %d time(s)", loadCount)
	}
}

// TestOrganizationCreateIsAvailableAndLegacyBacked pins the entry point to the
// founder playbook. `adb org init` registers a .aidb trust scope; `adb
// initiative` reads a different registry (orgs/index.yaml), and composing the
// legacy tree with IncludeOrg:false unregistered the only command that wrote
// it — so `adb initiative create --org X` failed with `organization "X" not
// found` and no public command could fix that.
//
// `create` therefore has to be present, visible, and *not* v3-annotated: it
// needs the legacy App loaded, which the v3 annotation would skip.
func TestOrganizationCreateIsAvailableAndLegacyBacked(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	create, _, err := root.Find([]string{"org", "create"})
	if err != nil {
		t.Fatalf("find org create: %v", err)
	}
	if create == nil || create.Name() != "create" {
		t.Fatal("org create is unavailable")
	}
	if create.Hidden {
		t.Fatal("org create must be in public help")
	}
	if create.Annotations[v3Annotation] == "true" {
		t.Fatal("org create must not be v3-annotated: it needs the legacy App")
	}
	if create.Flags().Lookup("git-host") == nil {
		t.Fatal("org create must keep --git-host")
	}
}

// TestOrganizationCreateLoadsLegacyApp proves the annotation choice above has
// the effect it claims: running the command triggers the legacy loader, which
// is what makes StageManager reachable.
func TestOrganizationCreateLoadsLegacyApp(t *testing.T) {
	loadCount := 0
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		LoadLegacyApp: func() error {
			loadCount++
			return nil
		},
	})
	root.SetArgs([]string{"org", "create", "Acme"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	// The legacy App is nil in this package's tests, so the handler's own guard
	// returns an error. That is fine — the assertion is about the loader having
	// been called at all, which only happens for a non-v3 command.
	_ = root.ExecuteContext(context.Background())
	if loadCount != 1 {
		t.Fatalf("org create loaded legacy app %d time(s), want 1", loadCount)
	}
}

func TestOrganizationInitializePlansByDefaultAndRequiresExplicitApply(
	t *testing.T,
) {
	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	service := &fakeOrganization{
		initializeResult: capability.Result[organization.InitializeData]{
			Capability:  organization.InitializeDescriptor.Capability,
			Version:     organization.InitializeDescriptor.Version,
			Outcome:     capability.OutcomePlanned,
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	output := execute(
		t,
		NewRoot(RootOptions{
			Foundation:   &fakeFoundation{},
			Organization: service,
		}),
		"org",
		"init",
		"amazon",
		"--workspace",
		workspaceRoot,
		"--name",
		"Amazon",
		"--format",
		"json",
	)
	assertSingleJSONValue(t, output, service.initializeResult)
	if len(service.initializeRequests) != 1 {
		t.Fatalf(
			"organization initialize request count = %d, want 1",
			len(service.initializeRequests),
		)
	}
	request := service.initializeRequests[0]
	if request.WorkspaceRoot != workspaceRoot ||
		request.Slug != "amazon" ||
		request.Name != "Amazon" ||
		request.Apply {
		t.Fatalf("organization initialize request = %#v", request)
	}

	service.initializeRequests = nil
	service.initializeResult.Outcome = capability.OutcomeApplied
	execute(
		t,
		NewRoot(RootOptions{
			Foundation:   &fakeFoundation{},
			Organization: service,
		}),
		"org",
		"init",
		"amazon",
		"--workspace",
		workspaceRoot,
		"--name",
		"Amazon",
		"--apply",
		"--format",
		"json",
	)
	if len(service.initializeRequests) != 1 ||
		!service.initializeRequests[0].Apply {
		t.Fatalf(
			"explicit apply request = %#v",
			service.initializeRequests,
		)
	}
}

func TestOrganizationUpdatePreservesOmittedAndExplicitEmptyFields(
	t *testing.T,
) {
	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	service := &fakeOrganization{
		updateResult: capability.Result[organization.MutationData]{
			Capability:  organization.UpdateDescriptor.Capability,
			Version:     organization.UpdateDescriptor.Version,
			Outcome:     capability.OutcomePlanned,
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	execute(
		t,
		NewRoot(RootOptions{
			Foundation:   &fakeFoundation{},
			Organization: service,
		}),
		"org",
		"update",
		"amazon",
		"--workspace",
		workspaceRoot,
		"--owner",
		"",
		"--description",
		"Cloud",
		"--format",
		"json",
	)
	if len(service.updateRequests) != 1 {
		t.Fatalf(
			"organization update request count = %d, want 1",
			len(service.updateRequests),
		)
	}
	request := service.updateRequests[0]
	if request.Name != nil ||
		request.Parent != nil ||
		request.Owner == nil ||
		*request.Owner != "" ||
		request.Description == nil ||
		*request.Description != "Cloud" {
		t.Fatalf("organization update request = %#v", request)
	}
	if request.Apply {
		t.Fatal("organization update applied without --apply")
	}
}

type fakeOrganization struct {
	initializeResult   capability.Result[organization.InitializeData]
	initializeErr      error
	initializeRequests []organization.InitializeRequest
	listResult         capability.Result[organization.ListData]
	listErr            error
	listRequests       []organization.ListRequest
	showResult         capability.Result[organization.ShowData]
	showErr            error
	showRequests       []organization.ShowRequest
	updateResult       capability.Result[organization.MutationData]
	updateErr          error
	updateRequests     []organization.UpdateRequest
	adoptResult        capability.Result[organization.MutationData]
	adoptErr           error
	adoptRequests      []organization.AdoptRequest
	moveResult         capability.Result[organization.MutationData]
	moveErr            error
	moveRequests       []organization.MoveRequest
	archiveResult      capability.Result[organization.MutationData]
	archiveErr         error
	archiveRequests    []organization.ArchiveRequest
	validateResult     capability.Result[organization.ValidateData]
	validateErr        error
	validateRequests   []organization.ValidateRequest
}

func (service *fakeOrganization) Initialize(
	_ context.Context,
	request organization.InitializeRequest,
) (capability.Result[organization.InitializeData], error) {
	service.initializeRequests = append(service.initializeRequests, request)
	return service.initializeResult, service.initializeErr
}

func (service *fakeOrganization) List(
	_ context.Context,
	request organization.ListRequest,
) (capability.Result[organization.ListData], error) {
	service.listRequests = append(service.listRequests, request)
	return service.listResult, service.listErr
}

func (service *fakeOrganization) Show(
	_ context.Context,
	request organization.ShowRequest,
) (capability.Result[organization.ShowData], error) {
	service.showRequests = append(service.showRequests, request)
	return service.showResult, service.showErr
}

func (service *fakeOrganization) Update(
	_ context.Context,
	request organization.UpdateRequest,
) (capability.Result[organization.MutationData], error) {
	service.updateRequests = append(service.updateRequests, request)
	return service.updateResult, service.updateErr
}

func (service *fakeOrganization) Adopt(
	_ context.Context,
	request organization.AdoptRequest,
) (capability.Result[organization.MutationData], error) {
	service.adoptRequests = append(service.adoptRequests, request)
	return service.adoptResult, service.adoptErr
}

func (service *fakeOrganization) Move(
	_ context.Context,
	request organization.MoveRequest,
) (capability.Result[organization.MutationData], error) {
	service.moveRequests = append(service.moveRequests, request)
	return service.moveResult, service.moveErr
}

func (service *fakeOrganization) Archive(
	_ context.Context,
	request organization.ArchiveRequest,
) (capability.Result[organization.MutationData], error) {
	service.archiveRequests = append(service.archiveRequests, request)
	return service.archiveResult, service.archiveErr
}

func (service *fakeOrganization) Validate(
	_ context.Context,
	request organization.ValidateRequest,
) (capability.Result[organization.ValidateData], error) {
	service.validateRequests = append(service.validateRequests, request)
	return service.validateResult, service.validateErr
}
