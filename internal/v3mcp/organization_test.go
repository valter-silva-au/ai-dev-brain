package v3mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

func TestRegisterOrganizationAddsVersionedToolsAndAnnotations(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "test")
	RegisterOrganization(mcpServer, &fakeOrganization{})

	expected := map[string]struct {
		readOnly    bool
		destructive bool
	}{
		organization.InitializeDescriptor.Tool: {false, false},
		organization.ListDescriptor.Tool:       {true, false},
		organization.ShowDescriptor.Tool:       {true, false},
		organization.UpdateDescriptor.Tool:     {false, false},
		organization.AdoptDescriptor.Tool:      {false, false},
		organization.MoveDescriptor.Tool:       {false, false},
		organization.ArchiveDescriptor.Tool:    {false, false},
		organization.ValidateDescriptor.Tool:   {true, false},
	}
	for name, annotations := range expected {
		tool := requireTool(t, mcpServer, name)
		if tool.Tool.InputSchema.Type != "object" {
			t.Fatalf("%s input schema type = %q", name, tool.Tool.InputSchema.Type)
		}
		if tool.Tool.OutputSchema.Type != "object" {
			t.Fatalf("%s output schema type = %q", name, tool.Tool.OutputSchema.Type)
		}
		assertAnnotation(
			t,
			tool.Tool,
			annotations.readOnly,
			annotations.destructive,
			true,
			false,
		)
	}
}

func TestOrganizationInitializeAndUpdateRequireExplicitApply(t *testing.T) {
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
	mcpServer := server.NewMCPServer("test", "test")
	RegisterOrganization(mcpServer, service)

	result := callTool(
		t,
		mcpServer,
		organization.InitializeDescriptor.Tool,
		map[string]any{
			"workspace_root": t.TempDir(),
			"slug":           "amazon",
			"name":           "Amazon",
		},
	)
	if result.IsError {
		t.Fatalf("organization initialize tool error: %#v", result)
	}
	assertJSONEquivalent(t, result.StructuredContent, service.initializeResult)
	if len(service.initializeRequests) != 1 ||
		service.initializeRequests[0].Apply {
		t.Fatalf(
			"organization initialize requests = %#v",
			service.initializeRequests,
		)
	}

	callTool(
		t,
		mcpServer,
		organization.UpdateDescriptor.Tool,
		map[string]any{
			"workspace_root": t.TempDir(),
			"selector":       "amazon",
			"owner":          "",
			"description":    "Cloud",
		},
	)
	if len(service.updateRequests) != 1 {
		t.Fatalf(
			"organization update request count = %d, want 1",
			len(service.updateRequests),
		)
	}
	request := service.updateRequests[0]
	if request.Owner == nil ||
		*request.Owner != "" ||
		request.Description == nil ||
		*request.Description != "Cloud" ||
		request.Name != nil ||
		request.Apply {
		t.Fatalf("organization update request = %#v", request)
	}
}

type fakeOrganization struct {
	initializeResult   capability.Result[organization.InitializeData]
	initializeRequests []organization.InitializeRequest
	listResult         capability.Result[organization.ListData]
	listRequests       []organization.ListRequest
	showResult         capability.Result[organization.ShowData]
	showRequests       []organization.ShowRequest
	updateResult       capability.Result[organization.MutationData]
	updateRequests     []organization.UpdateRequest
	adoptResult        capability.Result[organization.MutationData]
	adoptRequests      []organization.AdoptRequest
	moveResult         capability.Result[organization.MutationData]
	moveRequests       []organization.MoveRequest
	archiveResult      capability.Result[organization.MutationData]
	archiveRequests    []organization.ArchiveRequest
	validateResult     capability.Result[organization.ValidateData]
	validateRequests   []organization.ValidateRequest
}

func (service *fakeOrganization) Initialize(
	_ context.Context,
	request organization.InitializeRequest,
) (capability.Result[organization.InitializeData], error) {
	service.initializeRequests = append(service.initializeRequests, request)
	return service.initializeResult, nil
}

func (service *fakeOrganization) List(
	_ context.Context,
	request organization.ListRequest,
) (capability.Result[organization.ListData], error) {
	service.listRequests = append(service.listRequests, request)
	return service.listResult, nil
}

func (service *fakeOrganization) Show(
	_ context.Context,
	request organization.ShowRequest,
) (capability.Result[organization.ShowData], error) {
	service.showRequests = append(service.showRequests, request)
	return service.showResult, nil
}

func (service *fakeOrganization) Update(
	_ context.Context,
	request organization.UpdateRequest,
) (capability.Result[organization.MutationData], error) {
	service.updateRequests = append(service.updateRequests, request)
	return service.updateResult, nil
}

func (service *fakeOrganization) Adopt(
	_ context.Context,
	request organization.AdoptRequest,
) (capability.Result[organization.MutationData], error) {
	service.adoptRequests = append(service.adoptRequests, request)
	return service.adoptResult, nil
}

func (service *fakeOrganization) Move(
	_ context.Context,
	request organization.MoveRequest,
) (capability.Result[organization.MutationData], error) {
	service.moveRequests = append(service.moveRequests, request)
	return service.moveResult, nil
}

func (service *fakeOrganization) Archive(
	_ context.Context,
	request organization.ArchiveRequest,
) (capability.Result[organization.MutationData], error) {
	service.archiveRequests = append(service.archiveRequests, request)
	return service.archiveResult, nil
}

func (service *fakeOrganization) Validate(
	_ context.Context,
	request organization.ValidateRequest,
) (capability.Result[organization.ValidateData], error) {
	service.validateRequests = append(service.validateRequests, request)
	return service.validateResult, nil
}

var _ Organization = (*fakeOrganization)(nil)
var _ = mcp.CallToolRequest{}
