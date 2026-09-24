package v3mcp

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

type Repository interface {
	Add(
		context.Context,
		repository.AddRequest,
	) (capability.Result[repository.MutationData], error)
	Adopt(
		context.Context,
		repository.AdoptRequest,
	) (capability.Result[repository.MutationData], error)
	List(
		context.Context,
		repository.ListRequest,
	) (capability.Result[repository.ListData], error)
	Show(
		context.Context,
		repository.ShowRequest,
	) (capability.Result[repository.ShowData], error)
	Health(
		context.Context,
		repository.HealthRequest,
	) (capability.Result[repository.HealthData], error)
	Fetch(
		context.Context,
		repository.FetchRequest,
	) (capability.Result[repository.MutationData], error)
	Update(
		context.Context,
		repository.UpdateRequest,
	) (capability.Result[repository.MutationData], error)
	Move(
		context.Context,
		repository.MoveRequest,
	) (capability.Result[repository.MutationData], error)
	Archive(
		context.Context,
		repository.ArchiveRequest,
	) (capability.Result[repository.MutationData], error)
	WorktreeList(
		context.Context,
		repository.WorktreeListRequest,
	) (capability.Result[repository.WorktreeListData], error)
	WorktreeRepair(
		context.Context,
		repository.WorktreeRepairRequest,
	) (capability.Result[repository.WorktreeMutationData], error)
	WorktreePrune(
		context.Context,
		repository.WorktreePruneRequest,
	) (capability.Result[repository.WorktreeMutationData], error)
}

func RegisterRepository(mcpServer *server.MCPServer, service Repository) {
	mcpServer.AddTool(
		mcp.NewTool(
			repository.AddDescriptor.Tool,
			append(
				[]mcp.ToolOption{mcp.WithDescription(
					"Plan or add a managed repository. Mutation requires apply: true.",
				)},
				repositoryScopeOptions()...,
			)...,
		),
		handleRepositoryAdd(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.AdoptDescriptor.Tool,
			append(
				[]mcp.ToolOption{mcp.WithDescription(
					"Plan or adopt an existing Git clone. Mutation requires apply: true.",
				)},
				repositoryAdoptOptions()...,
			)...,
		),
		handleRepositoryAdopt(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.ListDescriptor.Tool,
			mcp.WithDescription(
				"List registered repositories. Omit organization to list every "+
					"organization's repositories.",
			),
			workspaceRootOption(),
			optionalOrganizationOption(),
			mcp.WithBoolean("include_archived"),
			mcp.WithOutputSchema[capability.Result[repository.ListData]](),
			repositoryAnnotations(true, false),
		),
		handleRepositoryList(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.ShowDescriptor.Tool,
			mcp.WithDescription("Show one registered repository."),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithOutputSchema[capability.Result[repository.ShowData]](),
			repositoryAnnotations(true, false),
		),
		handleRepositoryShow(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.HealthDescriptor.Tool,
			mcp.WithDescription("Classify repository health without mutation."),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithOutputSchema[capability.Result[repository.HealthData]](),
			repositoryAnnotations(true, false),
		),
		handleRepositoryHealth(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.FetchDescriptor.Tool,
			append(
				[]mcp.ToolOption{mcp.WithDescription(
					"Plan or fetch the canonical remote. Mutation requires apply: true.",
				)},
				repositoryMutationOptions()...,
			)...,
		),
		handleRepositoryFetch(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.UpdateDescriptor.Tool,
			append(
				[]mcp.ToolOption{mcp.WithDescription(
					"Plan or safely fast-forward a repository. Mutation requires apply: true.",
				)},
				repositoryMutationOptions()...,
			)...,
		),
		handleRepositoryUpdate(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.MoveDescriptor.Tool,
			mcp.WithDescription(
				"Plan or move repository identity and container. "+
					"Updating the Git remote must be explicitly requested.",
			),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithString("new_remote", mcp.Required()),
			mcp.WithBoolean("update_remote"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[repository.MutationData]](),
			repositoryAnnotations(false, false),
		),
		handleRepositoryMove(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.ArchiveDescriptor.Tool,
			mcp.WithDescription(
				"Plan an archive or restore state change. Mutation requires apply: true.",
			),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithBoolean("restore"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[repository.MutationData]](),
			repositoryAnnotations(false, false),
		),
		handleRepositoryArchive(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.WorktreeListDescriptor.Tool,
			mcp.WithDescription("List repository worktrees and ownership state."),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithOutputSchema[capability.Result[repository.WorktreeListData]](),
			repositoryAnnotations(true, false),
		),
		handleRepositoryWorktreeList(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.WorktreeRepairDescriptor.Tool,
			mcp.WithDescription(
				"Plan or recreate a missing registered worktree. "+
					"Mutation requires apply: true.",
			),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithString("ticket_key", mcp.Required()),
			mcp.WithString("name", mcp.Required()),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[repository.WorktreeMutationData]](),
			repositoryAnnotations(false, false),
		),
		handleRepositoryWorktreeRepair(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			repository.WorktreePruneDescriptor.Tool,
			mcp.WithDescription(
				"Plan or safely prune an inactive registered worktree by exact path. "+
					"Mutation requires apply: true.",
			),
			workspaceRootOption(),
			organizationOption(),
			repositorySelectorOption(),
			mcp.WithString("path", mcp.Required()),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[repository.WorktreeMutationData]](),
			repositoryAnnotations(false, true),
		),
		handleRepositoryWorktreePrune(service),
	)
}

func repositoryScopeOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		workspaceRootOption(),
		organizationOption(),
		mcp.WithString("remote", mcp.Required()),
		mcp.WithString("mode", mcp.Required()),
		mcp.WithString("host"),
		mcp.WithString("owner"),
		mcp.WithString("name"),
		mcp.WithString("display_name"),
		mcp.WithString("remote_name"),
		actorIDOption(),
		applyOption(),
		mcp.WithOutputSchema[capability.Result[repository.MutationData]](),
		repositoryAnnotations(false, false),
	}
}

func repositoryAdoptOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		workspaceRootOption(),
		organizationOption(),
		mcp.WithString("path", mcp.Required()),
		mcp.WithString("remote_name"),
		mcp.WithString("display_name"),
		actorIDOption(),
		applyOption(),
		mcp.WithOutputSchema[capability.Result[repository.MutationData]](),
		repositoryAnnotations(false, false),
	}
}

func repositoryMutationOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		workspaceRootOption(),
		organizationOption(),
		repositorySelectorOption(),
		actorIDOption(),
		applyOption(),
		mcp.WithOutputSchema[capability.Result[repository.MutationData]](),
		repositoryAnnotations(false, false),
	}
}

func handleRepositoryAdd(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, err := repositoryScopeArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		remote, err := request.RequireString("remote")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		mode, err := request.RequireString("mode")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Add(ctx, repository.AddRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Mode:          repository.AddMode(mode),
			Host:          request.GetString("host", ""),
			Owner:         request.GetString("owner", ""),
			Name:          request.GetString("name", ""),
			DisplayName:   request.GetString("display_name", ""),
			Remote:        remote,
			RemoteName:    request.GetString("remote_name", ""),
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("add repository", result, err)
	}
}

func handleRepositoryAdopt(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, err := repositoryScopeArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		source, err := requiredAbsolutePath(request, "path")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Adopt(ctx, repository.AdoptRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Path:          source,
			RemoteName:    request.GetString("remote_name", ""),
			DisplayName:   request.GetString("display_name", ""),
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("adopt repository", result, err)
	}
}

func handleRepositoryList(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, err := repositoryOptionalScopeArguments(
			service,
			request,
		)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.List(ctx, repository.ListRequest{
			WorkspaceRoot:   root,
			Organization:    organization,
			IncludeArchived: request.GetBool("include_archived", false),
		})
		return organizationJSONResult("list repositories", result, err)
	}
}

func handleRepositoryShow(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Show(ctx, repository.ShowRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
		})
		return organizationJSONResult("show repository", result, err)
	}
}

func handleRepositoryHealth(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Health(ctx, repository.HealthRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
		})
		return organizationJSONResult("classify repository health", result, err)
	}
}

func handleRepositoryFetch(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Fetch(ctx, repository.FetchRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("fetch repository", result, err)
	}
}

func handleRepositoryUpdate(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Update(ctx, repository.UpdateRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("update repository", result, err)
	}
}

func handleRepositoryMove(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		newRemote, err := request.RequireString("new_remote")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Move(ctx, repository.MoveRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
			NewRemote:     newRemote,
			UpdateRemote:  request.GetBool("update_remote", false),
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("move repository", result, err)
	}
}

func handleRepositoryArchive(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.Archive(ctx, repository.ArchiveRequest{
			WorkspaceRoot: root,
			Organization:  organization,
			Selector:      selector,
			Restore:       request.GetBool("restore", false),
			ActorType:     mcpActorType,
			ActorID:       request.GetString("actor_id", ""),
			Tool:          mcpTool,
			Apply:         request.GetBool("apply", false),
		})
		return organizationJSONResult("archive repository", result, err)
	}
}

func handleRepositoryWorktreeList(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.WorktreeList(
			ctx,
			repository.WorktreeListRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      selector,
			},
		)
		return organizationJSONResult("list repository worktrees", result, err)
	}
}

func handleRepositoryWorktreeRepair(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		ticketKey, err := request.RequireString("ticket_key")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		name, err := request.RequireString("name")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.WorktreeRepair(
			ctx,
			repository.WorktreeRepairRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      selector,
				TicketKey:     ticketKey,
				Name:          name,
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("repair repository worktree", result, err)
	}
}

func handleRepositoryWorktreePrune(service Repository) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		root, organization, selector, err := repositoryReadArguments(service, request)
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		target, err := requiredAbsolutePath(request, "path")
		if err != nil {
			return invalidRepositoryArguments(err)
		}
		result, err := service.WorktreePrune(
			ctx,
			repository.WorktreePruneRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      selector,
				Path:          target,
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("prune repository worktree", result, err)
	}
}

func organizationOption() mcp.ToolOption {
	return mcp.WithString(
		"organization",
		mcp.Required(),
		mcp.Description("Organization ID, slug, path, or alias."),
	)
}

// optionalOrganizationOption is for `list` alone. Every other repository tool
// addresses one repository by a selector unique only within an organization, so
// omitting it there would be an ambiguous lookup; listing a whole workspace is
// a coherent question, and an agent is the caller most likely not to know the
// organization names yet.
func optionalOrganizationOption() mcp.ToolOption {
	return mcp.WithString(
		"organization",
		mcp.Description(
			"Organization ID, slug, path, or alias. Omit to span every "+
				"organization in the workspace.",
		),
	)
}

func repositorySelectorOption() mcp.ToolOption {
	return mcp.WithString(
		"selector",
		mcp.Required(),
		mcp.Description("Repository ID, path, canonical identity, or alias."),
	)
}

func repositoryAnnotations(
	readOnly bool,
	destructive bool,
) mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(readOnly)(tool)
		mcp.WithDestructiveHintAnnotation(destructive)(tool)
		mcp.WithIdempotentHintAnnotation(true)(tool)
		mcp.WithOpenWorldHintAnnotation(false)(tool)
	}
}

func repositoryScopeArguments(
	service Repository,
	request mcp.CallToolRequest,
) (string, string, error) {
	if service == nil {
		return "", "", errors.New(
			"repository capability is unavailable: service is not configured",
		)
	}
	root, err := requiredAbsolutePath(request, "workspace_root")
	if err != nil {
		return "", "", err
	}
	organization, err := request.RequireString("organization")
	if err != nil {
		return "", "", err
	}
	return root, organization, nil
}

// repositoryOptionalScopeArguments is repositoryScopeArguments with the
// organization left optional, for `list`. An absent organization means "the
// whole workspace"; the service layer, not this adapter, decides what that
// spans.
func repositoryOptionalScopeArguments(
	service Repository,
	request mcp.CallToolRequest,
) (string, string, error) {
	if service == nil {
		return "", "", errors.New(
			"repository capability is unavailable: service is not configured",
		)
	}
	root, err := requiredAbsolutePath(request, "workspace_root")
	if err != nil {
		return "", "", err
	}
	return root, request.GetString("organization", ""), nil
}

func repositoryReadArguments(
	service Repository,
	request mcp.CallToolRequest,
) (string, string, string, error) {
	root, organization, err := repositoryScopeArguments(service, request)
	if err != nil {
		return "", "", "", err
	}
	selector, err := request.RequireString("selector")
	if err != nil {
		return "", "", "", err
	}
	return root, organization, selector, nil
}

func invalidRepositoryArguments(
	err error,
) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
}
