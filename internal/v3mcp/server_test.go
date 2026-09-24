package v3mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
)

func TestRegisterAddsVersionedFoundationTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "test")
	Register(mcpServer, &fakeFoundation{})

	initialize := requireTool(t, mcpServer, "adb_workspace_initialize")
	doctor := requireTool(t, mcpServer, "adb_workspace_doctor")

	for name, tool := range map[string]mcp.Tool{
		"initialize": initialize.Tool,
		"doctor":     doctor.Tool,
	} {
		if tool.InputSchema.Type != "object" {
			t.Fatalf("%s input schema type = %q, want object", name, tool.InputSchema.Type)
		}
		if tool.OutputSchema.Type != "object" {
			t.Fatalf("%s output schema type = %q, want object", name, tool.OutputSchema.Type)
		}
		for _, property := range []string{"capability", "version", "outcome"} {
			if _, ok := tool.OutputSchema.Properties[property]; !ok {
				t.Fatalf("%s output schema missing %q", name, property)
			}
		}
	}

	assertAnnotation(t, initialize.Tool, false, false, true, false)
	assertAnnotation(t, doctor.Tool, true, false, true, false)
}

func TestInitializeReturnsSameStructuredEnvelopeAsService(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	expected := initializeResult(root, capability.OutcomePlanned)
	service := &fakeFoundation{initializeResult: expected}
	mcpServer := server.NewMCPServer("test", "test")
	Register(mcpServer, service)

	result := callTool(t, mcpServer, "adb_workspace_initialize", map[string]any{
		"root": root,
		"name": "AWS",
	})
	if result.IsError {
		t.Fatalf("initialize returned tool error: %#v", result)
	}
	assertJSONEquivalent(t, result.StructuredContent, expected)
	assertTextFallbackEquivalent(t, result, expected)

	if len(service.initializeRequests) != 1 {
		t.Fatalf(
			"initialize request count = %d, want 1",
			len(service.initializeRequests),
		)
	}
	if service.initializeRequests[0].Apply {
		t.Fatal("initialize mutated without apply: true")
	}
}

func TestInitializeRequiresExplicitApplyForMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	service := &fakeFoundation{
		initializeResult: initializeResult(root, capability.OutcomeApplied),
	}
	mcpServer := server.NewMCPServer("test", "test")
	Register(mcpServer, service)

	callTool(t, mcpServer, "adb_workspace_initialize", map[string]any{
		"root":  root,
		"name":  "AWS",
		"apply": true,
	})
	if len(service.initializeRequests) != 1 {
		t.Fatalf(
			"initialize request count = %d, want 1",
			len(service.initializeRequests),
		)
	}
	if !service.initializeRequests[0].Apply {
		t.Fatal("explicit apply was not forwarded")
	}
}

func TestDoctorToolIsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	service := deterministicFoundation(t)
	if _, err := service.Initialize(context.Background(), foundation.InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	mcpServer := server.NewMCPServer("test", "test")
	Register(mcpServer, service)
	before := snapshotTree(t, root)
	result := callTool(t, mcpServer, "adb_workspace_doctor", map[string]any{
		"root": root,
	})
	if result.IsError {
		t.Fatalf("doctor returned tool error: %#v", result)
	}

	var envelope capability.Result[foundation.DoctorData]
	roundTripJSON(t, result.StructuredContent, &envelope)
	if envelope.Outcome != capability.OutcomeHealthy {
		t.Fatalf("doctor outcome = %q, want healthy", envelope.Outcome)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"doctor MCP tool mutated workspace\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func TestToolResultsDoNotCaptureAmbientSecrets(t *testing.T) {
	const secret = "TASK39_MCP_SECRET_DO_NOT_LEAK"
	t.Setenv("AIDB_TEST_SECRET", secret)

	root := filepath.Join(t.TempDir(), "AWS")
	service := &fakeFoundation{
		initializeResult: initializeResult(root, capability.OutcomePlanned),
	}
	mcpServer := server.NewMCPServer("test", "test")
	Register(mcpServer, service)

	result := callTool(t, mcpServer, "adb_workspace_initialize", map[string]any{
		"root": root,
		"name": "AWS",
	})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("tool result leaked ambient environment secret")
	}
}

type fakeFoundation struct {
	initializeResult   capability.Result[foundation.InitializeData]
	initializeErr      error
	initializeRequests []foundation.InitializeRequest
	doctorResult       capability.Result[foundation.DoctorData]
	doctorErr          error
	doctorRequests     []foundation.DoctorRequest
}

func (service *fakeFoundation) Initialize(
	_ context.Context,
	request foundation.InitializeRequest,
) (capability.Result[foundation.InitializeData], error) {
	service.initializeRequests = append(service.initializeRequests, request)
	return service.initializeResult, service.initializeErr
}

func (service *fakeFoundation) Doctor(
	_ context.Context,
	request foundation.DoctorRequest,
) (capability.Result[foundation.DoctorData], error) {
	service.doctorRequests = append(service.doctorRequests, request)
	return service.doctorResult, service.doctorErr
}

func requireTool(
	t *testing.T,
	mcpServer *server.MCPServer,
	name string,
) *server.ServerTool {
	t.Helper()

	tool := mcpServer.GetTool(name)
	if tool == nil {
		t.Fatalf("tool %q is not registered", name)
	}
	return tool
}

func callTool(
	t *testing.T,
	mcpServer *server.MCPServer,
	name string,
	arguments map[string]any,
) *mcp.CallToolResult {
	t.Helper()

	tool := requireTool(t, mcpServer, name)
	request := mcp.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = arguments
	result, err := tool.Handler(context.Background(), request)
	if err != nil {
		t.Fatalf("call %s transport error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("call %s returned nil result", name)
	}
	return result
}

func assertAnnotation(
	t *testing.T,
	tool mcp.Tool,
	readOnly bool,
	destructive bool,
	idempotent bool,
	openWorld bool,
) {
	t.Helper()

	checks := []struct {
		name string
		got  *bool
		want bool
	}{
		{"readOnlyHint", tool.Annotations.ReadOnlyHint, readOnly},
		{"destructiveHint", tool.Annotations.DestructiveHint, destructive},
		{"idempotentHint", tool.Annotations.IdempotentHint, idempotent},
		{"openWorldHint", tool.Annotations.OpenWorldHint, openWorld},
	}
	for _, check := range checks {
		if check.got == nil || *check.got != check.want {
			t.Fatalf(
				"%s annotation = %v, want %t",
				check.name,
				check.got,
				check.want,
			)
		}
	}
}

func assertJSONEquivalent(t *testing.T, got any, want any) {
	t.Helper()

	var gotJSON any
	roundTripJSON(t, got, &gotJSON)
	var wantJSON any
	roundTripJSON(t, want, &wantJSON)
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("JSON values differ\n got: %#v\nwant: %#v", gotJSON, wantJSON)
	}
}

func assertTextFallbackEquivalent(
	t *testing.T,
	result *mcp.CallToolResult,
	want any,
) {
	t.Helper()

	if len(result.Content) != 1 {
		t.Fatalf("text fallback count = %d, want 1", len(result.Content))
	}
	text, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("text fallback type = %T", result.Content[0])
	}
	var decoded any
	if err := json.Unmarshal([]byte(text.Text), &decoded); err != nil {
		t.Fatalf("decode text fallback: %v", err)
	}
	assertJSONEquivalent(t, decoded, want)
}

func roundTripJSON(t *testing.T, value any, target any) {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
}

func initializeResult(
	root string,
	outcome capability.Outcome,
) capability.Result[foundation.InitializeData] {
	return capability.Result[foundation.InitializeData]{
		Capability: "workspace.initialize",
		Version:    "v1",
		Outcome:    outcome,
		Data: foundation.InitializeData{
			WorkspaceID: "workspace-1",
			OperationID: "operation-1",
			Root:        root,
			Manifest:    filepath.Join(root, ".aidb", "manifest.yaml"),
			Config:      filepath.Join(root, ".aidb", "config.yaml"),
			State:       filepath.Join(root, ".aidb", "state.sqlite"),
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func deterministicFoundation(t *testing.T) *foundation.Service {
	t.Helper()

	ids := []string{
		"workspace-1",
		"operation-1",
		"event-1",
		"event-2",
		"event-3",
		"event-4",
		"event-5",
		"event-6",
	}
	idIndex := 0
	tick := 0
	service, err := foundation.NewService(foundation.Options{
		Clock: func() time.Time {
			tick++
			return time.Date(
				2026,
				time.September,
				10,
				8,
				0,
				tick,
				0,
				time.UTC,
			)
		},
		IDGenerator: func() string {
			if idIndex >= len(ids) {
				t.Fatalf("unexpected id request at index %d", idIndex)
			}
			id := ids[idIndex]
			idIndex++
			return id
		},
	})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	return service
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			snapshot[relative] = fmt.Sprintf(
				"size=%d sha256=%x",
				len(content),
				sha256.Sum256(content),
			)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("snapshot workspace: %v", err)
	}
	return snapshot
}
