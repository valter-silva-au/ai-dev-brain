package organization

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Layout struct {
	workspaceRoot string
	slug          string
	root          string
}

func NewLayout(workspaceRoot string, slug string) (Layout, error) {
	return NewLayoutForRole(
		workspaceRoot,
		workspace.DefaultRoles().Organizations,
		slug,
	)
}

func NewLayoutForRole(
	workspaceRoot string,
	organizationsRole string,
	slug string,
) (Layout, error) {
	workspaceLayout, err := workspace.NewLayout(workspaceRoot)
	if err != nil {
		return Layout{}, err
	}
	if err := ValidateSlug(slug); err != nil {
		return Layout{}, err
	}
	organizationsRoot, err := workspaceLayout.ResolveRole(organizationsRole)
	if err != nil {
		return Layout{}, fmt.Errorf(
			"resolve workspace organizations role: %w",
			err,
		)
	}

	return Layout{
		workspaceRoot: workspaceLayout.Root(),
		slug:          slug,
		root:          filepath.Join(organizationsRoot, slug),
	}, nil
}

func ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf(
			"organization slug %q must be 1-63 lowercase letters, digits, or interior dashes",
			slug,
		)
	}
	return nil
}

func (layout Layout) WorkspaceRoot() string {
	return layout.workspaceRoot
}

func (layout Layout) Slug() string {
	return layout.slug
}

func (layout Layout) Root() string {
	return layout.root
}

func (layout Layout) ControlDir() string {
	return filepath.Join(layout.root, ".aidb")
}

func (layout Layout) ManifestPath() string {
	return filepath.Join(layout.ControlDir(), "manifest.yaml")
}

func (layout Layout) ConfigPath() string {
	return filepath.Join(layout.ControlDir(), "config.yaml")
}

func (layout Layout) AgentsPath() string {
	return filepath.Join(layout.root, "AGENTS.md")
}

func (layout Layout) KnowledgeDir() string {
	return filepath.Join(layout.root, "knowledge")
}

func (layout Layout) StakeholdersDir() string {
	return filepath.Join(layout.root, "stakeholders")
}

func (layout Layout) TicketsDir() string {
	return filepath.Join(layout.root, "tickets")
}

func (layout Layout) RepositoriesDir() string {
	return filepath.Join(layout.root, "repos")
}

func (layout Layout) ResolveRole(role string) (string, error) {
	if role == "" {
		return "", errors.New("organization role path is required")
	}
	if filepath.IsAbs(role) || filepath.VolumeName(role) != "" {
		return "", fmt.Errorf(
			"organization role path must be relative: %q",
			role,
		)
	}

	target := filepath.Clean(filepath.Join(layout.root, role))
	relative, err := filepath.Rel(layout.root, target)
	if err != nil {
		return "", fmt.Errorf("resolve organization role %q: %w", role, err)
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"organization role path escapes root: %q",
			role,
		)
	}
	return target, nil
}

// InspectRole physically inspects a managed organization role from the
// workspace root.
func (layout Layout) InspectRole(
	role string,
) (workspace.RoleInspection, error) {
	target, err := layout.ResolveRole(role)
	if err != nil {
		return workspace.RoleInspection{}, err
	}
	return workspace.InspectPath(layout.workspaceRoot, target)
}

func AgentsPointer(layout Layout) (string, error) {
	workspaceAgents := filepath.Join(layout.WorkspaceRoot(), "AGENTS.md")
	relative, err := filepath.Rel(layout.Root(), workspaceAgents)
	if err != nil {
		return "", fmt.Errorf("resolve workspace AGENTS.md pointer: %w", err)
	}
	relative = filepath.ToSlash(relative)
	return "# Organization instructions\n\n" +
		"Canonical workspace instructions: [AGENTS.md](" +
		relative +
		").\n", nil
}
