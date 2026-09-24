package organization

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestLayoutUsesStrictOrganizationBoundary(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	layout, err := NewLayout(workspaceRoot, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}

	wantRoot := filepath.Join(workspaceRoot, "organizations", "amazon")
	if layout.Root() != wantRoot {
		t.Fatalf("root = %q, want %q", layout.Root(), wantRoot)
	}
	got := map[string]string{
		"control":      layout.ControlDir(),
		"manifest":     layout.ManifestPath(),
		"config":       layout.ConfigPath(),
		"agents":       layout.AgentsPath(),
		"knowledge":    layout.KnowledgeDir(),
		"stakeholders": layout.StakeholdersDir(),
		"tickets":      layout.TicketsDir(),
		"repositories": layout.RepositoriesDir(),
	}
	want := map[string]string{
		"control":      filepath.Join(wantRoot, ".aidb"),
		"manifest":     filepath.Join(wantRoot, ".aidb", "manifest.yaml"),
		"config":       filepath.Join(wantRoot, ".aidb", "config.yaml"),
		"agents":       filepath.Join(wantRoot, "AGENTS.md"),
		"knowledge":    filepath.Join(wantRoot, "knowledge"),
		"stakeholders": filepath.Join(wantRoot, "stakeholders"),
		"tickets":      filepath.Join(wantRoot, "tickets"),
		"repositories": filepath.Join(wantRoot, "repos"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("layout\n got: %#v\nwant: %#v", got, want)
	}
}

func TestLayoutRejectsUnsafeOrUnstableSlugs(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	tests := []struct {
		name string
		slug string
	}{
		{name: "empty", slug: ""},
		{name: "uppercase", slug: "Amazon"},
		{name: "leading hyphen", slug: "-amazon"},
		{name: "trailing hyphen", slug: "amazon-"},
		{name: "underscore", slug: "amazon_web_services"},
		{name: "path separator", slug: "amazon/web-services"},
		{name: "parent traversal", slug: ".."},
		{name: "hidden", slug: ".hidden"},
		{name: "too long", slug: strings.Repeat("a", 64)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewLayout(workspaceRoot, test.slug); err == nil {
				t.Fatalf("slug %q was accepted", test.slug)
			}
		})
	}
}

func TestManifestRoundTripPreservesIdentityRolesAndProvenance(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	layout, err := NewLayout(workspaceRoot, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	createdAt := time.Date(
		2026,
		time.September,
		10,
		11,
		0,
		0,
		0,
		time.UTC,
	)
	manifest := NewManifest(
		"organization-1",
		"amazon",
		"Amazon",
		createdAt,
		Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			ActorID:     "local-user",
			Tool:        "adb-cli",
		},
	)
	manifest.ParentID = "organization-parent"
	manifest.Owner = "platform"
	manifest.Description = "Amazon software organization"
	manifest.Trust = "confidential"
	manifest.Profile = "default"
	manifest.Aliases = []string{"amazon-old"}

	if err := manifest.Validate(layout); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	decoded, err := DecodeManifest(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip\n got: %#v\nwant: %#v", decoded, manifest)
	}
	if decoded.SchemaVersion != ManifestSchema ||
		decoded.Kind != ManifestKind ||
		decoded.Status != StatusActive {
		t.Fatalf("manifest identity fields = %#v", decoded)
	}
}

func TestManifestRejectsEscapedRolesUnknownFieldsAndMultipleDocuments(
	t *testing.T,
) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	layout, err := NewLayout(workspaceRoot, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	manifest := NewManifest(
		"organization-1",
		"amazon",
		"Amazon",
		time.Date(2026, time.September, 10, 11, 0, 0, 0, time.UTC),
		Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	unknown := `schema_version: aidb.organization/v1
kind: Organization
id: organization-1
slug: amazon
display_name: Amazon
created_at: 2026-09-10T11:00:00Z
status: active
aliases: []
provenance:
  operation_id: operation-1
  actor_type: human
  tool: adb-cli
roles:
  config: .aidb/config.yaml
  agents: AGENTS.md
  knowledge: knowledge
  stakeholders: stakeholders
  tickets: tickets
  repositories: repos
unexpected: true
`

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	encodedManifest := append([]byte(nil), encoded.Bytes()...)

	tests := []struct {
		name  string
		check func() error
	}{
		{
			name: "escaped role",
			check: func() error {
				candidate := manifest
				candidate.Roles.Knowledge = "../outside"
				return candidate.Validate(layout)
			},
		},
		{
			name: "unknown field",
			check: func() error {
				_, err := DecodeManifest(strings.NewReader(unknown))
				return err
			},
		},
		{
			name: "multiple documents",
			check: func() error {
				content := append(
					append([]byte(nil), encodedManifest...),
					[]byte("---\n{}\n")...,
				)
				_, err := DecodeManifest(bytes.NewReader(content))
				return err
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.check(); err == nil {
				t.Fatal("invalid organization manifest was accepted")
			}
		})
	}
}

func TestAgentsPointerTargetsCanonicalWorkspaceInstructions(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	layout, err := NewLayout(workspaceRoot, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}

	content, err := AgentsPointer(layout)
	if err != nil {
		t.Fatalf("build AGENTS pointer: %v", err)
	}
	if !strings.Contains(content, "../../AGENTS.md") {
		t.Fatalf("AGENTS pointer does not target workspace instructions:\n%s", content)
	}
	if strings.Contains(content, workspaceRoot) {
		t.Fatalf("AGENTS pointer contains non-portable absolute path:\n%s", content)
	}
}

func TestLayoutInspectRoleUsesWorkspaceAsTrustedRoot(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "AWS")
	layout, err := NewLayout(workspaceRoot, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
		t.Fatalf("create organization root: %v", err)
	}

	inspection, err := layout.InspectRole("repos")
	if err != nil {
		t.Fatalf("inspect missing role: %v", err)
	}
	if inspection.State != workspace.RoleInspectionMissingTail ||
		inspection.Target != layout.RepositoriesDir() {
		t.Fatalf("missing role inspection = %#v", inspection)
	}

	if err := os.MkdirAll(layout.RepositoriesDir(), 0o755); err != nil {
		t.Fatalf("create repositories role: %v", err)
	}
	inspection, err = layout.InspectRole("repos")
	if err != nil {
		t.Fatalf("inspect contained role: %v", err)
	}
	if inspection.State != workspace.RoleInspectionContained {
		t.Fatalf("contained role inspection = %#v", inspection)
	}
}
