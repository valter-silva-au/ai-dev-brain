package v3mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

const (
	mcpActorType = "agent"
	mcpTool      = "adb-mcp"
)

type Organization interface {
	Initialize(
		context.Context,
		organization.InitializeRequest,
	) (capability.Result[organization.InitializeData], error)
	List(
		context.Context,
		organization.ListRequest,
	) (capability.Result[organization.ListData], error)
	Show(
		context.Context,
		organization.ShowRequest,
	) (capability.Result[organization.ShowData], error)
	Update(
		context.Context,
		organization.UpdateRequest,
	) (capability.Result[organization.MutationData], error)
	Adopt(
		context.Context,
		organization.AdoptRequest,
	) (capability.Result[organization.MutationData], error)
	Move(
		context.Context,
		organization.MoveRequest,
	) (capability.Result[organization.MutationData], error)
	Archive(
		context.Context,
		organization.ArchiveRequest,
	) (capability.Result[organization.MutationData], error)
	Validate(
		context.Context,
		organization.ValidateRequest,
	) (capability.Result[organization.ValidateData], error)
}

func RegisterOrganization(
	mcpServer *server.MCPServer,
	service Organization,
) {
	mcpServer.AddTool(
		mcp.NewTool(
			organization.InitializeDescriptor.Tool,
			mcp.WithDescription(
				"Plan or initialize an organization trust scope. "+
					"Mutation requires apply: true.",
			),
			workspaceRootOption(),
			mcp.WithString("slug", mcp.Required()),
			mcp.WithString("name", mcp.Required()),
			mcp.WithString("parent"),
			mcp.WithString("owner"),
			mcp.WithString("description"),
			mcp.WithString("trust"),
			mcp.WithString("profile"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[organization.InitializeData]](),
			organizationAnnotations(false),
		),
		handleOrganizationInitialize(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.ListDescriptor.Tool,
			mcp.WithDescription("List registered organizations."),
			workspaceRootOption(),
			mcp.WithBoolean(
				"include_archived",
				mcp.Description("Include archived organizations."),
			),
			mcp.WithOutputSchema[capability.Result[organization.ListData]](),
			organizationAnnotations(true),
		),
		handleOrganizationList(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.ShowDescriptor.Tool,
			mcp.WithDescription("Show one organization by ID, slug, path, or alias."),
			workspaceRootOption(),
			selectorOption(),
			mcp.WithOutputSchema[capability.Result[organization.ShowData]](),
			organizationAnnotations(true),
		),
		handleOrganizationShow(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.UpdateDescriptor.Tool,
			mcp.WithDescription(
				"Plan or update organization metadata. Omitted fields remain unchanged; "+
					"explicit empty strings clear optional fields. Mutation requires apply: true.",
			),
			workspaceRootOption(),
			selectorOption(),
			mcp.WithString("name"),
			mcp.WithString("parent"),
			mcp.WithString("owner"),
			mcp.WithString("description"),
			mcp.WithString("trust"),
			mcp.WithString("profile"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[organization.MutationData]](),
			organizationAnnotations(false),
		),
		handleOrganizationUpdate(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.AdoptDescriptor.Tool,
			mcp.WithDescription(
				"Plan or adopt an unmanaged directory as an organization. "+
					"Authored content is preserved and mutation requires apply: true.",
			),
			workspaceRootOption(),
			mcp.WithString("path", mcp.Required()),
			mcp.WithString("slug", mcp.Required()),
			mcp.WithString("name", mcp.Required()),
			mcp.WithString("parent"),
			mcp.WithString("owner"),
			mcp.WithString("description"),
			mcp.WithString("trust"),
			mcp.WithString("profile"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[organization.MutationData]](),
			organizationAnnotations(false),
		),
		handleOrganizationAdopt(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.MoveDescriptor.Tool,
			mcp.WithDescription(
				"Plan or move an organization to a new canonical slug while preserving "+
					"its immutable ID and prior selectors. Mutation requires apply: true.",
			),
			workspaceRootOption(),
			selectorOption(),
			mcp.WithString("new_slug", mcp.Required()),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[organization.MutationData]](),
			organizationAnnotations(false),
		),
		handleOrganizationMove(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.ArchiveDescriptor.Tool,
			mcp.WithDescription(
				"Plan an archive or restore state change without deleting content. "+
					"Mutation requires apply: true.",
			),
			workspaceRootOption(),
			selectorOption(),
			mcp.WithBoolean("restore"),
			actorIDOption(),
			applyOption(),
			mcp.WithOutputSchema[capability.Result[organization.MutationData]](),
			organizationAnnotations(false),
		),
		handleOrganizationArchive(service),
	)
	mcpServer.AddTool(
		mcp.NewTool(
			organization.ValidateDescriptor.Tool,
			mcp.WithDescription(
				"Validate an organization and report typed findings without repair.",
			),
			workspaceRootOption(),
			selectorOption(),
			mcp.WithOutputSchema[capability.Result[organization.ValidateData]](),
			organizationAnnotations(true),
		),
		handleOrganizationValidate(service),
	)
}

func handleOrganizationInitialize(
	service Organization,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, err := requiredAbsolutePath(request, "workspace_root")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		slug, err := request.RequireString("slug")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		name, err := request.RequireString("name")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Initialize(
			ctx,
			organization.InitializeRequest{
				WorkspaceRoot: root,
				Slug:          slug,
				Name:          name,
				Parent:        request.GetString("parent", ""),
				Owner:         request.GetString("owner", ""),
				Description:   request.GetString("description", ""),
				Trust:         request.GetString("trust", ""),
				Profile:       request.GetString("profile", ""),
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("initialize organization", result, err)
	}
}

func handleOrganizationList(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, err := requiredAbsolutePath(request, "workspace_root")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.List(
			ctx,
			organization.ListRequest{
				WorkspaceRoot:   root,
				IncludeArchived: request.GetBool("include_archived", false),
			},
		)
		return organizationJSONResult("list organizations", result, err)
	}
}

func handleOrganizationShow(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, selector, err := organizationReadArguments(request)
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Show(
			ctx,
			organization.ShowRequest{
				WorkspaceRoot: root,
				Selector:      selector,
			},
		)
		return organizationJSONResult("show organization", result, err)
	}
}

func handleOrganizationUpdate(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, selector, err := organizationReadArguments(request)
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Update(
			ctx,
			organization.UpdateRequest{
				WorkspaceRoot: root,
				Selector:      selector,
				Name:          optionalStringArgument(request, "name"),
				Parent:        optionalStringArgument(request, "parent"),
				Owner:         optionalStringArgument(request, "owner"),
				Description: optionalStringArgument(
					request,
					"description",
				),
				Trust:     optionalStringArgument(request, "trust"),
				Profile:   optionalStringArgument(request, "profile"),
				ActorType: mcpActorType,
				ActorID:   request.GetString("actor_id", ""),
				Tool:      mcpTool,
				Apply:     request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("update organization", result, err)
	}
}

func handleOrganizationAdopt(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, err := requiredAbsolutePath(request, "workspace_root")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		source, err := requiredAbsolutePath(request, "path")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		slug, err := request.RequireString("slug")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		name, err := request.RequireString("name")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Adopt(
			ctx,
			organization.AdoptRequest{
				WorkspaceRoot: root,
				Path:          source,
				Slug:          slug,
				Name:          name,
				Parent:        request.GetString("parent", ""),
				Owner:         request.GetString("owner", ""),
				Description:   request.GetString("description", ""),
				Trust:         request.GetString("trust", ""),
				Profile:       request.GetString("profile", ""),
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("adopt organization", result, err)
	}
}

func handleOrganizationMove(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, selector, err := organizationReadArguments(request)
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		newSlug, err := request.RequireString("new_slug")
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Move(
			ctx,
			organization.MoveRequest{
				WorkspaceRoot: root,
				Selector:      selector,
				NewSlug:       newSlug,
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("move organization", result, err)
	}
}

func handleOrganizationArchive(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, selector, err := organizationReadArguments(request)
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Archive(
			ctx,
			organization.ArchiveRequest{
				WorkspaceRoot: root,
				Selector:      selector,
				Restore:       request.GetBool("restore", false),
				ActorType:     mcpActorType,
				ActorID:       request.GetString("actor_id", ""),
				Tool:          mcpTool,
				Apply:         request.GetBool("apply", false),
			},
		)
		return organizationJSONResult("archive organization", result, err)
	}
}

func handleOrganizationValidate(service Organization) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return organizationUnavailable()
		}
		root, selector, err := organizationReadArguments(request)
		if err != nil {
			return invalidOrganizationArguments(err)
		}
		result, err := service.Validate(
			ctx,
			organization.ValidateRequest{
				WorkspaceRoot: root,
				Selector:      selector,
			},
		)
		return organizationJSONResult("validate organization", result, err)
	}
}

func organizationAnnotations(readOnly bool) mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(readOnly)(tool)
		mcp.WithDestructiveHintAnnotation(false)(tool)
		mcp.WithIdempotentHintAnnotation(true)(tool)
		mcp.WithOpenWorldHintAnnotation(false)(tool)
	}
}

func workspaceRootOption() mcp.ToolOption {
	return mcp.WithString(
		"workspace_root",
		mcp.Required(),
		mcp.Description("Absolute or relative v3 workspace path."),
	)
}

func selectorOption() mcp.ToolOption {
	return mcp.WithString(
		"selector",
		mcp.Required(),
		mcp.Description("Organization ID, slug, path, or alias."),
	)
}

func actorIDOption() mcp.ToolOption {
	return mcp.WithString(
		"actor_id",
		mcp.Description("Optional stable caller identity for provenance."),
	)
}

func applyOption() mcp.ToolOption {
	return mcp.WithBoolean(
		"apply",
		mcp.Description("Set true to apply the plan. Defaults to false."),
	)
}

func requiredAbsolutePath(
	request mcp.CallToolRequest,
	key string,
) (string, error) {
	value, err := request.RequireString(key)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", key, err)
	}
	return absolute, nil
}

func organizationReadArguments(
	request mcp.CallToolRequest,
) (string, string, error) {
	root, err := requiredAbsolutePath(request, "workspace_root")
	if err != nil {
		return "", "", err
	}
	selector, err := request.RequireString("selector")
	if err != nil {
		return "", "", err
	}
	return root, selector, nil
}

func optionalStringArgument(
	request mcp.CallToolRequest,
	key string,
) *string {
	arguments := request.GetArguments()
	raw, ok := arguments[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	return &value
}

func organizationUnavailable() (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(
		"organization capability is unavailable: service is not configured",
	), nil
}

func invalidOrganizationArguments(
	err error,
) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
}

func organizationJSONResult[T any](
	action string,
	result T,
	err error,
) (*mcp.CallToolResult, error) {
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return mcp.NewToolResultErrorFromErr(action+" failed", err), nil
	}
	toolResult, encodeErr := mcp.NewToolResultJSON(result)
	if encodeErr != nil {
		return nil, fmt.Errorf("encode %s result: %w", action, encodeErr)
	}
	return toolResult, nil
}
