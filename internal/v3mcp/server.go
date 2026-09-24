package v3mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
)

type Foundation interface {
	Initialize(
		context.Context,
		foundation.InitializeRequest,
	) (capability.Result[foundation.InitializeData], error)
	Doctor(
		context.Context,
		foundation.DoctorRequest,
	) (capability.Result[foundation.DoctorData], error)
}

func Register(mcpServer *server.MCPServer, service Foundation) {
	mcpServer.AddTool(
		mcp.NewTool(
			foundation.InitializeDescriptor.Tool,
			mcp.WithDescription(
				"Plan or initialize an AI Dev Brain v3 workspace at an explicit path. "+
					"Omit apply or set it to false for a non-mutating plan; mutation "+
					"requires apply: true.",
			),
			mcp.WithString(
				"root",
				mcp.Required(),
				mcp.Description("Absolute or relative workspace path."),
			),
			mcp.WithString(
				"name",
				mcp.Description(
					"Workspace display name. Defaults to the path basename.",
				),
			),
			mcp.WithBoolean(
				"apply",
				mcp.Description(
					"Set true to apply the plan. Defaults to false.",
				),
			),
			mcp.WithOutputSchema[capability.Result[foundation.InitializeData]](),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
		),
		handleInitialize(service),
	)

	mcpServer.AddTool(
		mcp.NewTool(
			foundation.DoctorDescriptor.Tool,
			mcp.WithDescription(
				"Inspect an AI Dev Brain v3 workspace and return typed findings. "+
					"This tool never initializes, migrates, repairs, or rewrites the workspace.",
			),
			mcp.WithString(
				"root",
				mcp.Required(),
				mcp.Description("Absolute or relative workspace path."),
			),
			mcp.WithOutputSchema[capability.Result[foundation.DoctorData]](),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
		),
		handleDoctor(service),
	)
}

func handleInitialize(service Foundation) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return mcp.NewToolResultError(
				"workspace initialization is unavailable: foundation service is not configured",
			), nil
		}
		root, err := request.RequireString("root")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return mcp.NewToolResultErrorFromErr(
				"invalid workspace path",
				err,
			), nil
		}
		name := request.GetString("name", "")
		if name == "" {
			name = filepath.Base(filepath.Clean(absolute))
		}

		result, err := service.Initialize(
			ctx,
			foundation.InitializeRequest{
				Root:  absolute,
				Name:  name,
				Apply: request.GetBool("apply", false),
			},
		)
		if err != nil {
			return mcp.NewToolResultErrorFromErr(
				"workspace initialization failed",
				err,
			), nil
		}
		toolResult, err := mcp.NewToolResultJSON(result)
		if err != nil {
			return nil, fmt.Errorf("encode workspace initialization result: %w", err)
		}
		return toolResult, nil
	}
}

func handleDoctor(service Foundation) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		if service == nil {
			return mcp.NewToolResultError(
				"workspace doctor is unavailable: foundation service is not configured",
			), nil
		}
		root, err := request.RequireString("root")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return mcp.NewToolResultErrorFromErr(
				"invalid workspace path",
				err,
			), nil
		}

		result, err := service.Doctor(
			ctx,
			foundation.DoctorRequest{Root: absolute},
		)
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return mcp.NewToolResultErrorFromErr(
				"workspace doctor failed",
				err,
			), nil
		}
		toolResult, err := mcp.NewToolResultJSON(result)
		if err != nil {
			return nil, fmt.Errorf("encode workspace doctor result: %w", err)
		}
		return toolResult, nil
	}
}
