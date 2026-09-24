package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLayoutUsesAidbPaths(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	tests := map[string]string{
		"root":          root,
		"control":       filepath.Join(root, ".aidb"),
		"manifest":      filepath.Join(root, ".aidb", "manifest.yaml"),
		"config":        filepath.Join(root, ".aidb", "config.yaml"),
		"state":         filepath.Join(root, ".aidb", "state.sqlite"),
		"events":        filepath.Join(root, ".aidb", "events"),
		"cache":         filepath.Join(root, ".aidb", "cache"),
		"organizations": filepath.Join(root, "organizations"),
	}

	got := map[string]string{
		"root":          layout.Root(),
		"control":       layout.ControlDir(),
		"manifest":      layout.ManifestPath(),
		"config":        layout.ConfigPath(),
		"state":         layout.StatePath(),
		"events":        layout.EventsDir(),
		"cache":         layout.CacheDir(),
		"organizations": layout.OrganizationsDir(),
	}
	for name, want := range tests {
		if got[name] != want {
			t.Errorf("%s path = %q, want %q", name, got[name], want)
		}
	}

	if filepath.Base(layout.ControlDir()) == ".adb" {
		t.Fatal("v3 layout used the v2 .adb directory")
	}
}

func TestNewLayoutRejectsRelativeRoot(t *testing.T) {
	t.Parallel()

	if _, err := NewLayout("relative/workspace"); err == nil {
		t.Fatal("expected relative root to be rejected")
	}
}

func TestResolveRoleRejectsEscapes(t *testing.T) {
	t.Parallel()

	layout, err := NewLayout(filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	for _, role := range []string{"../outside", "/absolute", ""} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			if _, err := layout.ResolveRole(role); err == nil {
				t.Fatalf("expected role %q to be rejected", role)
			}
		})
	}

	got, err := layout.ResolveRole("organizations")
	if err != nil {
		t.Fatalf("resolve contained role: %v", err)
	}
	if want := filepath.Join(layout.Root(), "organizations"); got != want {
		t.Fatalf("resolved role = %q, want %q", got, want)
	}
}

func TestManifestRoundTripAndStrictDecode(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	manifest := NewManifest("workspace-1", "AWS", createdAt)

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}

	decoded, err := DecodeManifest(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if decoded.SchemaVersion != ManifestSchema {
		t.Errorf("schema = %q, want %q", decoded.SchemaVersion, ManifestSchema)
	}
	if decoded.Kind != ManifestKind {
		t.Errorf("kind = %q, want %q", decoded.Kind, ManifestKind)
	}
	if decoded.ID != manifest.ID || decoded.Name != manifest.Name {
		t.Errorf("identity = %#v, want %#v", decoded, manifest)
	}
	if !decoded.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %s, want %s", decoded.CreatedAt, createdAt)
	}

	invalid := append(encoded.Bytes(), []byte("unknown_field: true\n")...)
	if _, err := DecodeManifest(bytes.NewReader(invalid)); err == nil {
		t.Fatal("expected unknown manifest field to be rejected")
	}
}

func TestManifestValidationRejectsEscapedRole(t *testing.T) {
	t.Parallel()

	layout, err := NewLayout(filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	manifest := NewManifest(
		"workspace-1",
		"AWS",
		time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	)
	manifest.Roles.Events = "../events"

	if err := manifest.Validate(layout); err == nil {
		t.Fatal("expected escaped role to be rejected")
	}
}

func TestDiscoverFindsNearestValidWorkspace(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create control dir: %v", err)
	}
	manifest := NewManifest(
		"workspace-1",
		"AWS",
		time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	)
	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(layout.ManifestPath(), encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	nested := filepath.Join(root, "organizations", "amazon", "repos")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested path: %v", err)
	}

	gotLayout, gotManifest, err := Discover(nested)
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}
	if gotLayout.Root() != root {
		t.Errorf("discovered root = %q, want %q", gotLayout.Root(), root)
	}
	if gotManifest.ID != manifest.ID {
		t.Errorf("discovered id = %q, want %q", gotManifest.ID, manifest.ID)
	}
}

func TestDiscoverIgnoresV2ADBDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".adb"), 0o755); err != nil {
		t.Fatalf("create v2 state dir: %v", err)
	}

	_, _, err := Discover(root)
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("discover error = %v, want ErrWorkspaceNotFound", err)
	}
}
