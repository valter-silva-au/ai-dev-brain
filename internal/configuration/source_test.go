package configuration

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverSourcesUsesOnlyExplicitContainedSemanticScopes(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "AWS")
	organizationRoot := filepath.Join(
		workspaceRoot,
		"organizations",
		"amazon",
	)
	hostRoot := filepath.Join(organizationRoot, "repos", "github.com")
	ownerRoot := filepath.Join(hostRoot, "valter-silva-au")
	repositoryRoot := filepath.Join(ownerRoot, "ai-dev-brain")
	ticketRoot := filepath.Join(repositoryRoot, "tickets", "ADB-39")
	for _, directory := range []string{
		workspaceRoot,
		organizationRoot,
		hostRoot,
		ownerRoot,
		repositoryRoot,
		ticketRoot,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create semantic scope %q: %v", directory, err)
		}
	}
	userGlobal := filepath.Join(base, "user-config.yaml")
	sources, err := DiscoverSources(ScopePaths{
		WorkspaceRoot:    workspaceRoot,
		UserGlobalPath:   userGlobal,
		OrganizationRoot: organizationRoot,
		HostRoot:         hostRoot,
		OwnerRoot:        ownerRoot,
		RepositoryRoot:   repositoryRoot,
		TicketRoot:       ticketRoot,
	})
	if err != nil {
		t.Fatalf("discover sources: %v", err)
	}

	gotKinds := make([]ScopeKind, 0, len(sources))
	gotPaths := make([]string, 0, len(sources))
	for _, source := range sources {
		gotKinds = append(gotKinds, source.Kind)
		gotPaths = append(gotPaths, source.Path)
	}
	wantKinds := []ScopeKind{
		ScopeUserGlobal,
		ScopeWorkspace,
		ScopeOrganization,
		ScopeHost,
		ScopeOwner,
		ScopeRepository,
		ScopeTicket,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("source kinds = %v, want %v", gotKinds, wantKinds)
	}
	canonical := func(path string) string {
		t.Helper()
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatalf("canonicalize %q: %v", path, err)
		}
		return resolved
	}
	wantPaths := []string{
		userGlobal,
		filepath.Join(canonical(workspaceRoot), ".aidb", "config.yaml"),
		filepath.Join(canonical(organizationRoot), ".aidb", "config.yaml"),
		filepath.Join(canonical(hostRoot), ".aidb", "config.yaml"),
		filepath.Join(canonical(ownerRoot), ".aidb", "config.yaml"),
		filepath.Join(canonical(repositoryRoot), ".aidb", "config.yaml"),
		filepath.Join(canonical(ticketRoot), ".aidb", "config.yaml"),
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("source paths = %v, want %v", gotPaths, wantPaths)
	}

	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}
	link := filepath.Join(repositoryRoot, "tickets", "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create escaping symlink: %v", err)
	}
	if _, err := DiscoverSources(ScopePaths{
		WorkspaceRoot:    workspaceRoot,
		OrganizationRoot: organizationRoot,
		HostRoot:         hostRoot,
		OwnerRoot:        ownerRoot,
		RepositoryRoot:   repositoryRoot,
		TicketRoot:       link,
	}); err == nil {
		t.Fatal("escaping ticket scope was accepted")
	}
}

func TestDiscoverSourcesRequiresExistingDirectoryRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths func(t *testing.T) ScopePaths
	}{
		{
			name: "missing workspace root",
			paths: func(t *testing.T) ScopePaths {
				return ScopePaths{
					WorkspaceRoot: filepath.Join(t.TempDir(), "missing"),
				}
			},
		},
		{
			name: "workspace root is a regular file",
			paths: func(t *testing.T) ScopePaths {
				fileRoot := filepath.Join(t.TempDir(), "workspace-file")
				if err := os.WriteFile(
					fileRoot,
					[]byte("not a directory"),
					0o600,
				); err != nil {
					t.Fatalf("write file root: %v", err)
				}
				return ScopePaths{WorkspaceRoot: fileRoot}
			},
		},
		{
			name: "workspace root is relative",
			paths: func(t *testing.T) ScopePaths {
				return ScopePaths{WorkspaceRoot: "relative/workspace"}
			},
		},
		{
			name: "user global path is relative",
			paths: func(t *testing.T) ScopePaths {
				workspaceRoot := filepath.Join(t.TempDir(), "workspace")
				if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
					t.Fatalf("create workspace root: %v", err)
				}
				return ScopePaths{
					WorkspaceRoot:  workspaceRoot,
					UserGlobalPath: "relative/config.yaml",
				}
			},
		},
		{
			name: "organization escapes workspace",
			paths: func(t *testing.T) ScopePaths {
				base := t.TempDir()
				workspaceRoot := filepath.Join(base, "workspace")
				organizationRoot := filepath.Join(base, "organization")
				for _, directory := range []string{
					workspaceRoot,
					organizationRoot,
				} {
					if err := os.MkdirAll(directory, 0o755); err != nil {
						t.Fatalf("create scope root %q: %v", directory, err)
					}
				}
				return ScopePaths{
					WorkspaceRoot:    workspaceRoot,
					OrganizationRoot: organizationRoot,
				}
			},
		},
		{
			name: "organization equals workspace",
			paths: func(t *testing.T) ScopePaths {
				workspaceRoot := filepath.Join(t.TempDir(), "workspace")
				if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
					t.Fatalf("create workspace root: %v", err)
				}
				return ScopePaths{
					WorkspaceRoot:    workspaceRoot,
					OrganizationRoot: workspaceRoot,
				}
			},
		},
		{
			name: "host scope omits organization parent",
			paths: func(t *testing.T) ScopePaths {
				workspaceRoot := filepath.Join(t.TempDir(), "workspace")
				hostRoot := filepath.Join(workspaceRoot, "host")
				if err := os.MkdirAll(hostRoot, 0o755); err != nil {
					t.Fatalf("create host root: %v", err)
				}
				return ScopePaths{
					WorkspaceRoot: workspaceRoot,
					HostRoot:      hostRoot,
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := DiscoverSources(test.paths(t)); err == nil {
				t.Fatal("invalid source roots were accepted")
			}
		})
	}
}
