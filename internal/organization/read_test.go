package organization

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestListAndShowOrganizationsAreDeterministicAndReadOnly(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	for _, item := range []struct {
		slug string
		name string
	}{
		{"zeta", "Zeta"},
		{"amazon", "Amazon"},
	} {
		if _, err := service.Initialize(
			context.Background(),
			InitializeRequest{
				WorkspaceRoot: root,
				Slug:          item.slug,
				Name:          item.name,
				ActorType:     "human",
				Tool:          "adb-cli",
				Apply:         true,
			},
		); err != nil {
			t.Fatalf("initialize %s: %v", item.slug, err)
		}
	}

	before := snapshotOrganizationWorkspace(t, root)
	listed, err := service.List(
		context.Background(),
		ListRequest{WorkspaceRoot: root},
	)
	if err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	if len(listed.Data.Organizations) != 2 {
		t.Fatalf(
			"organization count = %d, want 2",
			len(listed.Data.Organizations),
		)
	}
	if listed.Data.Organizations[0].Slug != "amazon" ||
		listed.Data.Organizations[1].Slug != "zeta" {
		t.Fatalf(
			"organization order = %#v",
			listed.Data.Organizations,
		)
	}

	want := listed.Data.Organizations[0]
	selectors := []struct {
		name     string
		selector string
	}{
		{name: "immutable ID", selector: want.ID},
		{name: "slug", selector: want.Slug},
		{name: "canonical path", selector: want.Path},
	}
	for _, test := range selectors {
		t.Run(test.name, func(t *testing.T) {
			shown, err := service.Show(
				context.Background(),
				ShowRequest{
					WorkspaceRoot: root,
					Selector:      test.selector,
				},
			)
			if err != nil {
				t.Fatalf("show organization by %s: %v", test.name, err)
			}
			if !reflect.DeepEqual(shown.Data.Organization, want) {
				t.Fatalf(
					"show by %s\n got: %#v\nwant: %#v",
					test.name,
					shown.Data.Organization,
					want,
				)
			}
		})
	}
	after := snapshotOrganizationWorkspace(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"organization reads mutated workspace\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func TestListCanIncludeOrExcludeArchivedOrganizations(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("initialize organization: %v", err)
	}

	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read organization manifest: %v", err)
	}
	archivedAt := manifest.UpdatedAt.AddDate(0, 0, 1)
	manifest.Status = StatusArchived
	manifest.ArchivedAt = &archivedAt
	manifest.UpdatedAt = archivedAt
	manifest.LastMutation = Provenance{
		OperationID: "archive-operation",
		ActorType:   "human",
		Tool:        "adb-cli",
	}
	writeOrganizationManifestForTest(t, layout, manifest)
	projectOrganizationManifestForTest(t, root, layout, manifest)

	tests := []struct {
		name            string
		includeArchived bool
		wantCount       int
		wantStatus      string
	}{
		{
			name:      "active only",
			wantCount: 0,
		},
		{
			name:            "include archived",
			includeArchived: true,
			wantCount:       1,
			wantStatus:      StatusArchived,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := service.List(
				context.Background(),
				ListRequest{
					WorkspaceRoot:   root,
					IncludeArchived: test.includeArchived,
				},
			)
			if err != nil {
				t.Fatalf("list organizations: %v", err)
			}
			if len(result.Data.Organizations) != test.wantCount {
				t.Fatalf(
					"organization count = %d, want %d: %#v",
					len(result.Data.Organizations),
					test.wantCount,
					result.Data.Organizations,
				)
			}
			if test.wantCount > 0 &&
				result.Data.Organizations[0].Status != test.wantStatus {
				t.Fatalf(
					"organization status = %q, want %q",
					result.Data.Organizations[0].Status,
					test.wantStatus,
				)
			}
		})
	}
}

func TestOrganizationReadsRejectMissingSelector(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	tests := []struct {
		name string
		read func() error
	}{
		{
			name: "show",
			read: func() error {
				_, err := service.Show(
					context.Background(),
					ShowRequest{WorkspaceRoot: root},
				)
				return err
			},
		},
		{
			name: "validate",
			read: func() error {
				_, err := service.Validate(
					context.Background(),
					ValidateRequest{WorkspaceRoot: root},
				)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.read(); err == nil {
				t.Fatal("missing organization selector was accepted")
			}
		})
	}
}

func TestValidateOrganizationReportsDriftWithoutRepair(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("initialize organization: %v", err)
	}

	healthy, err := service.Validate(
		context.Background(),
		ValidateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
		},
	)
	if err != nil {
		t.Fatalf("validate healthy organization: %v", err)
	}
	if len(healthy.Data.Findings) != 0 {
		t.Fatalf("healthy findings = %#v", healthy.Data.Findings)
	}

	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	if err := os.WriteFile(
		layout.AgentsPath(),
		[]byte("# wrong\n"),
		0o644,
	); err != nil {
		t.Fatalf("corrupt AGENTS pointer: %v", err)
	}
	before := snapshotOrganizationWorkspace(t, root)
	result, err := service.Validate(
		context.Background(),
		ValidateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
		},
	)
	if err != nil {
		t.Fatalf("validate drifted organization: %v", err)
	}
	assertOrganizationFinding(
		t,
		result.Data.Findings,
		"organization.agents.drift",
		"warning",
	)
	after := snapshotOrganizationWorkspace(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"organization validation repaired drift\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func assertOrganizationFinding(
	t *testing.T,
	findings []Finding,
	id string,
	severity string,
) {
	t.Helper()

	for _, finding := range findings {
		if finding.ID != id {
			continue
		}
		if finding.Severity != severity ||
			finding.Summary == "" ||
			len(finding.Evidence) == 0 ||
			finding.Remediation == "" {
			t.Fatalf("finding %q is incomplete: %#v", id, finding)
		}
		return
	}
	t.Fatalf("finding %q not present in %#v", id, findings)
}

func snapshotOrganizationWorkspace(
	t *testing.T,
	root string,
) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	if err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				snapshot[relative+"/"] = "directory"
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(content)
			snapshot[relative] = hex.EncodeToString(digest[:])
			return nil
		},
	); err != nil {
		t.Fatalf("snapshot organization workspace: %v", err)
	}
	return snapshot
}

func writeOrganizationManifestForTest(
	t *testing.T,
	layout Layout,
	manifest Manifest,
) {
	t.Helper()

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode organization manifest: %v", err)
	}
	if err := os.WriteFile(
		layout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write organization manifest: %v", err)
	}
}

func projectOrganizationManifestForTest(
	t *testing.T,
	root string,
	layout Layout,
	manifest Manifest,
) {
	t.Helper()

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode organization manifest: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	state, err := controlplane.Open(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	status := controlplane.EntityStatusActive
	if manifest.Status == StatusArchived {
		status = controlplane.EntityStatusArchived
	}
	if err := state.ObserveOrganization(
		context.Background(),
		controlplane.OrganizationProjection{
			ID:           manifest.ID,
			Slug:         manifest.Slug,
			Path:         layout.Root(),
			DisplayName:  manifest.DisplayName,
			ParentID:     manifest.ParentID,
			ManifestHash: journal.Digest(encoded.Bytes()),
			Status:       status,
			Aliases:      append([]string(nil), manifest.Aliases...),
			ObservedAt:   manifest.UpdatedAt,
		},
	); err != nil {
		_ = state.Close()
		t.Fatalf("project organization manifest: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("close control plane: %v", err)
	}
}
