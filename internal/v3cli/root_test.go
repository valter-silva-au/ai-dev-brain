package v3cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
)

func TestInitDryRunJSONWritesOneResultEnvelope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := initializeResult(root, capability.OutcomePlanned)
	service := &fakeFoundation{initializeResult: result}

	output := execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "init", "boundary", root, "--dry-run", "--format", "json")

	assertSingleJSONValue(t, output, result)
	if len(service.initializeRequests) != 1 {
		t.Fatalf(
			"initialize request count = %d, want 1",
			len(service.initializeRequests),
		)
	}
	request := service.initializeRequests[0]
	if request.Root != root || request.Name != "AWS" || request.Apply {
		t.Fatalf("initialize request = %#v", request)
	}
}

func TestInitApplyJSONMatchesServiceResult(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := initializeResult(root, capability.OutcomeApplied)
	service := &fakeFoundation{initializeResult: result}

	output := execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "init", "boundary", root, "--name", "Amazon", "--apply", "--format", "json")

	assertSingleJSONValue(t, output, result)
	request := service.initializeRequests[0]
	if request.Root != root || request.Name != "Amazon" || !request.Apply {
		t.Fatalf("initialize request = %#v", request)
	}
}

func TestInitHumanOutputIsDerivedFromServiceResult(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := initializeResult(root, capability.OutcomePlanned)
	service := &fakeFoundation{initializeResult: result}

	output := execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "init", "boundary", root, "--dry-run")

	for _, expected := range []string{
		"workspace.initialize/v1: planned",
		"Workspace: workspace-1",
		"Root: " + root,
		"planned create " + filepath.Join(root, ".aidb", "manifest.yaml"),
		"Next: Run adb init with --apply.",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("human output missing %q:\n%s", expected, output)
		}
	}
}

func TestDoctorJSONIsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	service := deterministicFoundation(t)
	if _, err := service.Initialize(context.Background(), foundation.InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	before := snapshotTree(t, root)
	output := execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "doctor", "--workspace", root, "--format", "json")

	var result capability.Result[foundation.DoctorData]
	decodeSingleJSONValue(t, output, &result)
	if result.Outcome != capability.OutcomeHealthy {
		t.Fatalf("doctor outcome = %q, want healthy", result.Outcome)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"doctor CLI mutated workspace\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func TestDoctorAttentionWritesCompleteJSONBeforeReturningError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := capability.Result[foundation.DoctorData]{
		Capability: "workspace.doctor",
		Version:    "v1",
		Outcome:    capability.OutcomeAttention,
		Data: foundation.DoctorData{
			WorkspaceID: "workspace-1",
			Root:        root,
			Findings: []foundation.Finding{{
				ID:          "workspace.role.escaped",
				Severity:    "error",
				Summary:     "A managed workspace role is unsafe.",
				Evidence:    []string{"role=organizations"},
				Remediation: "Replace the symlink with a real directory.",
			}},
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery: capability.Recovery{
			Required: true,
			Guidance: []string{},
		},
	}
	service := &fakeFoundation{doctorResult: result}
	command := NewRoot(RootOptions{Foundation: service})
	command.SetArgs([]string{
		"doctor",
		"--workspace",
		root,
		"--format",
		"json",
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("doctor attention returned nil error")
	}
	if err.Error() != "doctor requires attention" {
		t.Fatalf("doctor error = %q, want stable attention error", err)
	}
	assertSingleJSONValue(t, stdout.String(), result)
	for name, output := range map[string]string{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
	} {
		if strings.Contains(output, "Usage:") {
			t.Fatalf("%s contains Cobra usage:\n%s", name, output)
		}
	}
}

func TestFoundationCommandsDoNotLoadLegacyApp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	service := &fakeFoundation{
		initializeResult: initializeResult(root, capability.OutcomePlanned),
	}
	loadCount := 0
	loadLegacy := func() error {
		loadCount++
		return nil
	}

	execute(t, NewRoot(RootOptions{
		Foundation:    service,
		LoadLegacyApp: loadLegacy,
	}), "init", "boundary", root, "--dry-run", "--format", "json")
	if loadCount != 0 {
		t.Fatalf("init loaded legacy app %d time(s)", loadCount)
	}

	execute(t, NewRoot(RootOptions{
		Foundation:    service,
		LoadLegacyApp: loadLegacy,
	}), "version")
	if loadCount != 1 {
		t.Fatalf("legacy version loaded app %d time(s), want 1", loadCount)
	}
}

func TestUnifiedRootKeepsLegacyCommandsWithoutDuplicateInit(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation: &fakeFoundation{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	counts := make(map[string]int)
	for _, command := range root.Commands() {
		counts[command.Name()]++
	}
	if counts["init"] != 1 {
		t.Fatalf("init command count = %d, want 1", counts["init"])
	}
	if counts["doctor"] != 1 {
		t.Fatalf("doctor command count = %d, want 1", counts["doctor"])
	}
	for _, legacy := range []string{"task", "status", "work", "mcp", "version"} {
		if counts[legacy] != 1 {
			t.Fatalf("%s command count = %d, want 1", legacy, counts[legacy])
		}
	}

	// `status` and `work` are registered exactly once each — and HIDDEN. Both
	// were views over tasks, so TASK-00039 (Q7) moved them onto the `task` noun
	// as `task list --git` and `task worktree …`; they survive only as
	// deprecated aliases. Asserting the count alone would pass just as happily
	// if the composed root had kept advertising them.
	for _, retired := range []string{"status", "work"} {
		command, _, err := root.Find([]string{retired})
		if err != nil {
			t.Fatalf("adb %s is not reachable on the composed root: %v", retired, err)
		}
		if !command.Hidden {
			t.Errorf("adb %s is visible on the composed root, want hidden", retired)
		}
	}

	project, _, err := root.Find([]string{"init", "project"})
	if err != nil {
		t.Fatalf("find legacy init project command: %v", err)
	}
	if project == nil || project.Name() != "project" {
		t.Fatal("legacy init project command is unavailable")
	}
}

// TestInitChildVisibilityMatchesReplacementStatus pins the rule behind
// visibleInitChildren: an `adb init` child stays in public help exactly while
// nothing has replaced it. `workspace` scaffolds the task workspace and
// `project` provisions a document-program pack; `adb init <path>` creates the
// .aidb boundary and replaces neither, so hiding them left the only way to do
// those two things invisible.
func TestInitChildVisibilityMatchesReplacementStatus(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation: &fakeFoundation{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	for name, wantVisible := range map[string]bool{
		"workspace": true,
		"project":   true,
		"claude":    false,
		"update":    false,
	} {
		t.Run(name, func(t *testing.T) {
			child, _, err := root.Find([]string{"init", name})
			if err != nil {
				t.Fatalf("find init %s: %v", name, err)
			}
			if child == nil || child.Name() != name {
				t.Fatalf("init %s is unavailable", name)
			}
			if child.Hidden == wantVisible {
				t.Fatalf(
					"init %s hidden = %t, want hidden = %t",
					name,
					child.Hidden,
					!wantVisible,
				)
			}
		})
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

func initializeResult(
	root string,
	outcome capability.Outcome,
) capability.Result[foundation.InitializeData] {
	status := capability.EffectPlanned
	if outcome == capability.OutcomeApplied {
		status = capability.EffectApplied
	}
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
		Effects: []capability.Effect{{
			Action: "create",
			Target: filepath.Join(root, ".aidb", "manifest.yaml"),
			Status: status,
		}},
		Warnings: []capability.Notice{},
		NextActions: []capability.Action{{
			Code:    "apply_initialize",
			Message: "Run adb init with --apply.",
		}},
		Recovery: capability.Recovery{Guidance: []string{}},
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

func execute(t *testing.T, command interface {
	SetArgs([]string)
	SetOut(io.Writer)
	SetErr(io.Writer)
	ExecuteContext(context.Context) error
}, args ...string) string {
	t.Helper()

	var output bytes.Buffer
	command.SetArgs(args)
	command.SetOut(&output)
	command.SetErr(&output)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute %v: %v\n%s", args, err, output.String())
	}
	return output.String()
}

func assertSingleJSONValue[T any](
	t *testing.T,
	output string,
	want T,
) {
	t.Helper()

	var got T
	decodeSingleJSONValue(t, output, &got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON result\n got: %#v\nwant: %#v", got, want)
	}
}

func decodeSingleJSONValue(
	t *testing.T,
	output string,
	target any,
) {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode JSON result: %v\n%s", err, output)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("output contains more than one JSON value: %v\n%s", err, output)
	}
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
