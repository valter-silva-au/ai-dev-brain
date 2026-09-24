package v3mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

func TestRegisterRepositoryAddsLifecycleAndWorktreeTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "test")
	RegisterRepository(mcpServer, &fakeRepository{})
	for _, descriptor := range []capability.Descriptor{
		repository.AddDescriptor,
		repository.AdoptDescriptor,
		repository.ListDescriptor,
		repository.ShowDescriptor,
		repository.HealthDescriptor,
		repository.FetchDescriptor,
		repository.UpdateDescriptor,
		repository.MoveDescriptor,
		repository.ArchiveDescriptor,
		repository.WorktreeListDescriptor,
		repository.WorktreeRepairDescriptor,
		repository.WorktreePruneDescriptor,
	} {
		tool := requireTool(t, mcpServer, descriptor.Tool)
		if tool.Tool.InputSchema.Type != "object" ||
			tool.Tool.OutputSchema.Type != "object" {
			t.Fatalf("repository tool %q schema = %#v", descriptor.Tool, tool.Tool)
		}
	}
}

func TestRepositoryAddMCPPlansByDefault(t *testing.T) {
	service := &fakeRepository{
		addResult: capability.Result[repository.MutationData]{
			Capability:  repository.AddDescriptor.Capability,
			Version:     repository.AddDescriptor.Version,
			Outcome:     capability.OutcomePlanned,
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	mcpServer := server.NewMCPServer("test", "test")
	RegisterRepository(mcpServer, service)
	result := callTool(
		t,
		mcpServer,
		repository.AddDescriptor.Tool,
		map[string]any{
			"workspace_root": t.TempDir(),
			"organization":   "amazon",
			"remote":         "https://github.com/owner/repo.git",
			"mode":           "clone-new",
		},
	)
	if result.IsError {
		t.Fatalf("repository add tool error: %#v", result)
	}
	if len(service.addRequests) != 1 || service.addRequests[0].Apply {
		t.Fatalf("repository add requests = %#v", service.addRequests)
	}
}

// TestRepositoryListMCPOrganizationIsOptional keeps the MCP surface at parity
// with the CLI, where `adb repo list` dropped its required --org. An agent
// asking "what repositories are in this workspace?" is in exactly the position
// the relaxation exists for: it does not yet know the organization names.
func TestRepositoryListMCPOrganizationIsOptional(t *testing.T) {
	service := &fakeRepository{
		listResult: capability.Result[repository.ListData]{
			Capability:  repository.ListDescriptor.Capability,
			Version:     repository.ListDescriptor.Version,
			Outcome:     capability.OutcomeHealthy,
			Data:        repository.ListData{Repositories: []repository.Data{}},
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	mcpServer := server.NewMCPServer("test", "test")
	RegisterRepository(mcpServer, service)

	tool := requireTool(t, mcpServer, repository.ListDescriptor.Tool)
	for _, required := range tool.Tool.InputSchema.Required {
		if required == "organization" {
			t.Fatal("list tool must not mark organization required")
		}
	}

	result := callTool(
		t,
		mcpServer,
		repository.ListDescriptor.Tool,
		map[string]any{"workspace_root": t.TempDir()},
	)
	if result.IsError {
		t.Fatalf("repository list tool error: %#v", result)
	}
	if len(service.listRequests) != 1 {
		t.Fatalf("list requests = %#v", service.listRequests)
	}
	if service.listRequests[0].Organization != "" {
		t.Fatalf(
			"omitted organization became %q",
			service.listRequests[0].Organization,
		)
	}
}

// TestRepositoryListMCPStillAcceptsAnOrganization proves the relaxation is
// additive: naming an organization must still scope the read.
func TestRepositoryListMCPStillAcceptsAnOrganization(t *testing.T) {
	service := &fakeRepository{}
	mcpServer := server.NewMCPServer("test", "test")
	RegisterRepository(mcpServer, service)

	callTool(
		t,
		mcpServer,
		repository.ListDescriptor.Tool,
		map[string]any{
			"workspace_root": t.TempDir(),
			"organization":   "amazon",
		},
	)
	if len(service.listRequests) != 1 ||
		service.listRequests[0].Organization != "amazon" {
		t.Fatalf("list requests = %#v", service.listRequests)
	}
}

type fakeRepository struct {
	addResult    capability.Result[repository.MutationData]
	addRequests  []repository.AddRequest
	listResult   capability.Result[repository.ListData]
	listRequests []repository.ListRequest
}

func (service *fakeRepository) Add(
	_ context.Context,
	request repository.AddRequest,
) (capability.Result[repository.MutationData], error) {
	service.addRequests = append(service.addRequests, request)
	return service.addResult, nil
}

func (*fakeRepository) Adopt(
	context.Context,
	repository.AdoptRequest,
) (capability.Result[repository.MutationData], error) {
	return capability.Result[repository.MutationData]{}, nil
}

func (service *fakeRepository) List(
	_ context.Context,
	request repository.ListRequest,
) (capability.Result[repository.ListData], error) {
	service.listRequests = append(service.listRequests, request)
	return service.listResult, nil
}

func (*fakeRepository) Show(
	context.Context,
	repository.ShowRequest,
) (capability.Result[repository.ShowData], error) {
	return capability.Result[repository.ShowData]{}, nil
}

func (*fakeRepository) Health(
	context.Context,
	repository.HealthRequest,
) (capability.Result[repository.HealthData], error) {
	return capability.Result[repository.HealthData]{}, nil
}

func (*fakeRepository) Fetch(
	context.Context,
	repository.FetchRequest,
) (capability.Result[repository.MutationData], error) {
	return capability.Result[repository.MutationData]{}, nil
}

func (*fakeRepository) Update(
	context.Context,
	repository.UpdateRequest,
) (capability.Result[repository.MutationData], error) {
	return capability.Result[repository.MutationData]{}, nil
}

func (*fakeRepository) Move(
	context.Context,
	repository.MoveRequest,
) (capability.Result[repository.MutationData], error) {
	return capability.Result[repository.MutationData]{}, nil
}

func (*fakeRepository) Archive(
	context.Context,
	repository.ArchiveRequest,
) (capability.Result[repository.MutationData], error) {
	return capability.Result[repository.MutationData]{}, nil
}

func (*fakeRepository) WorktreeList(
	context.Context,
	repository.WorktreeListRequest,
) (capability.Result[repository.WorktreeListData], error) {
	return capability.Result[repository.WorktreeListData]{}, nil
}

func (*fakeRepository) WorktreeRepair(
	context.Context,
	repository.WorktreeRepairRequest,
) (capability.Result[repository.WorktreeMutationData], error) {
	return capability.Result[repository.WorktreeMutationData]{}, nil
}

func (*fakeRepository) WorktreePrune(
	context.Context,
	repository.WorktreePruneRequest,
) (capability.Result[repository.WorktreeMutationData], error) {
	return capability.Result[repository.WorktreeMutationData]{}, nil
}
