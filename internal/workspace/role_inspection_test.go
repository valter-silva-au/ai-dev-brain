package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectRoleReportsContainedExistingPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "organizations")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create role directory: %v", err)
	}
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	inspection, err := layout.InspectRole("organizations")
	if err != nil {
		t.Fatalf("inspect role: %v", err)
	}
	if inspection.State != RoleInspectionContained {
		t.Fatalf("state = %q, want %q", inspection.State, RoleInspectionContained)
	}
	if inspection.Target != target {
		t.Fatalf("target = %q, want %q", inspection.Target, target)
	}
	if inspection.DeepestExisting != target {
		t.Fatalf("deepest existing = %q, want %q", inspection.DeepestExisting, target)
	}
	if inspection.FirstMissing != "" {
		t.Fatalf("first missing = %q, want empty", inspection.FirstMissing)
	}
}

func TestInspectRoleReportsMissingTailUnderContainedParents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existing := filepath.Join(root, "organizations")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatalf("create existing parent: %v", err)
	}
	target := filepath.Join(existing, "amazon", "repos")

	inspection, err := InspectPath(root, target)
	if err != nil {
		t.Fatalf("inspect path: %v", err)
	}
	if inspection.State != RoleInspectionMissingTail {
		t.Fatalf("state = %q, want %q", inspection.State, RoleInspectionMissingTail)
	}
	if inspection.Target != target {
		t.Fatalf("target = %q, want %q", inspection.Target, target)
	}
	if inspection.DeepestExisting != existing {
		t.Fatalf("deepest existing = %q, want %q", inspection.DeepestExisting, existing)
	}
	if want := filepath.Join(existing, "amazon"); inspection.FirstMissing != want {
		t.Fatalf("first missing = %q, want %q", inspection.FirstMissing, want)
	}
}

func TestInspectRoleFunctionSupportsOrganizationRoots(t *testing.T) {
	t.Parallel()

	organizationRoot := t.TempDir()
	repositories := filepath.Join(organizationRoot, "repos")
	if err := os.Mkdir(repositories, 0o755); err != nil {
		t.Fatalf("create repositories role: %v", err)
	}

	inspection, err := InspectRole(organizationRoot, "repos")
	if err != nil {
		t.Fatalf("inspect organization role: %v", err)
	}
	if inspection.State != RoleInspectionContained {
		t.Fatalf("state = %q, want %q", inspection.State, RoleInspectionContained)
	}
	if inspection.Target != repositories {
		t.Fatalf("target = %q, want %q", inspection.Target, repositories)
	}
}

func TestInspectRoleRejectsSymlinkComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		linkTarget func(root string, external string) string
		role       string
	}{
		{
			name: "outside root intermediate",
			linkTarget: func(_ string, external string) string {
				return external
			},
			role: filepath.Join("organizations", "amazon"),
		},
		{
			name: "inside root intermediate",
			linkTarget: func(root string, _ string) string {
				return filepath.Join(root, "real-organizations")
			},
			role: filepath.Join("organizations", "amazon"),
		},
		{
			name: "outside root final",
			linkTarget: func(_ string, external string) string {
				return external
			},
			role: "organizations",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			external := t.TempDir()
			inside := filepath.Join(root, "real-organizations")
			if err := os.Mkdir(inside, 0o755); err != nil {
				t.Fatalf("create inside target: %v", err)
			}
			if err := os.Symlink(
				test.linkTarget(root, external),
				filepath.Join(root, "organizations"),
			); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			layout, err := NewLayout(root)
			if err != nil {
				t.Fatalf("new layout: %v", err)
			}

			_, err = layout.InspectRole(test.role)
			assertRoleViolation(
				t,
				err,
				RoleViolationSymlink,
				"organizations",
			)
		})
	}
}

func TestInspectRoleRejectsLexicalEscape(t *testing.T) {
	t.Parallel()

	layout, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	_, err = layout.InspectRole(filepath.Join("..", "outside"))
	assertRoleViolation(
		t,
		err,
		RoleViolationLexicalEscape,
		filepath.Join("..", "outside"),
	)
}

func TestInspectRoleRejectsUninspectableAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blockingFile := filepath.Join(root, "organizations")
	if err := os.WriteFile(blockingFile, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	_, err := InspectPath(root, filepath.Join(blockingFile, "amazon"))
	assertRoleViolation(
		t,
		err,
		RoleViolationUninspectable,
		"organizations",
	)
}

func TestInspectRoleAcceptsFinalRegularFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	control := filepath.Join(root, ".aidb")
	if err := os.Mkdir(control, 0o755); err != nil {
		t.Fatalf("create control directory: %v", err)
	}
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	for _, role := range []string{".aidb/config.yaml", ".aidb/state.sqlite"} {
		role := role
		t.Run(role, func(t *testing.T) {
			target := filepath.Join(root, filepath.FromSlash(role))
			if err := os.WriteFile(target, []byte("healthy\n"), 0o644); err != nil {
				t.Fatalf("create role file: %v", err)
			}

			inspection, err := layout.InspectRole(filepath.FromSlash(role))
			if err != nil {
				t.Fatalf("inspect final regular file: %v", err)
			}
			if inspection.State != RoleInspectionContained {
				t.Fatalf(
					"state = %q, want %q",
					inspection.State,
					RoleInspectionContained,
				)
			}
			if inspection.Target != target {
				t.Fatalf("target = %q, want %q", inspection.Target, target)
			}
		})
	}
}

func TestInspectRoleTrustsConfiguredRootAlias(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatalf("create real root: %v", err)
	}
	if err := os.Mkdir(filepath.Join(realRoot, "organizations"), 0o755); err != nil {
		t.Fatalf("create role directory: %v", err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Skipf("create root alias: %v", err)
	}
	layout, err := NewLayout(alias)
	if err != nil {
		t.Fatalf("new aliased layout: %v", err)
	}

	inspection, err := layout.InspectRole("organizations")
	if err != nil {
		t.Fatalf("inspect role through root alias: %v", err)
	}
	if inspection.State != RoleInspectionContained {
		t.Fatalf("state = %q, want %q", inspection.State, RoleInspectionContained)
	}
}

func assertRoleViolation(
	t *testing.T,
	err error,
	reason RoleViolationReason,
	component string,
) {
	t.Helper()

	if err == nil {
		t.Fatal("expected role inspection violation")
	}
	var violation *RoleViolation
	if !errors.As(err, &violation) {
		t.Fatalf("error type = %T, want *RoleViolation: %v", err, err)
	}
	if violation.Reason != reason {
		t.Fatalf("reason = %q, want %q", violation.Reason, reason)
	}
	if violation.Component != component {
		t.Fatalf("component = %q, want %q", violation.Component, component)
	}
}
