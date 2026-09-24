package repository

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

var (
	componentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	hostPattern      = regexp.MustCompile(
		`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`,
	)
	windowsDevicePattern = regexp.MustCompile(
		`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$`,
	)
)

type Layout struct {
	organizationRoot string
	repositoriesRoot string
	host             string
	owner            string
	name             string
	root             string
}

func NewLayout(
	organizationLayout organization.Layout,
	repositoriesRole string,
	host string,
	owner string,
	name string,
) (Layout, error) {
	if err := ValidateHost(host); err != nil {
		return Layout{}, err
	}
	if err := ValidateComponent(owner); err != nil {
		return Layout{}, fmt.Errorf("validate repository owner: %w", err)
	}
	if err := ValidateComponent(name); err != nil {
		return Layout{}, fmt.Errorf("validate repository name: %w", err)
	}
	repositoriesRoot, err := organizationLayout.ResolveRole(repositoriesRole)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve organization repository role: %w", err)
	}
	root := filepath.Join(repositoriesRoot, host, owner, name)
	if err := requireContainedPath(organizationLayout.Root(), root); err != nil {
		return Layout{}, err
	}
	return Layout{
		organizationRoot: organizationLayout.Root(),
		repositoriesRoot: repositoriesRoot,
		host:             host,
		owner:            owner,
		name:             name,
		root:             root,
	}, nil
}

func ValidateHost(host string) error {
	if host == "" {
		return errors.New("repository host is required")
	}
	if host != strings.ToLower(host) {
		return fmt.Errorf("repository host %q must be lowercase", host)
	}
	if err := ValidateComponent(host); err != nil {
		return fmt.Errorf("validate repository host: %w", err)
	}
	if !hostPattern.MatchString(host) ||
		strings.Contains(host, "..") {
		return fmt.Errorf("repository host %q is not a valid DNS-shaped name", host)
	}
	return nil
}

func ValidateComponent(value string) error {
	switch {
	case value == "":
		return errors.New("repository path component is required")
	case value == "." || value == "..":
		return fmt.Errorf("repository path component %q is reserved", value)
	case filepath.IsAbs(value) || filepath.VolumeName(value) != "":
		return fmt.Errorf(
			"repository path component %q must not be absolute",
			value,
		)
	case strings.ContainsAny(value, `/\:`):
		return fmt.Errorf(
			"repository path component %q contains a path separator or volume marker",
			value,
		)
	case strings.HasSuffix(value, ".") || strings.HasSuffix(value, " "):
		return fmt.Errorf(
			"repository path component %q has an unsafe trailing character",
			value,
		)
	case windowsDevicePattern.MatchString(value):
		return fmt.Errorf(
			"repository path component %q is a reserved Windows device name",
			value,
		)
	case !componentPattern.MatchString(value):
		return fmt.Errorf(
			"repository path component %q must use letters, digits, dot, dash, or underscore",
			value,
		)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"repository path component %q contains a control character",
				value,
			)
		}
	}
	return nil
}

func (layout Layout) OrganizationRoot() string {
	return layout.organizationRoot
}

func (layout Layout) RepositoriesRoot() string {
	return layout.repositoriesRoot
}

func (layout Layout) Host() string {
	return layout.host
}

func (layout Layout) Owner() string {
	return layout.owner
}

func (layout Layout) Name() string {
	return layout.name
}

func (layout Layout) Key() string {
	return layout.host + "/" + layout.owner + "/" + layout.name
}

func (layout Layout) Root() string {
	return layout.root
}

func (layout Layout) HostRoot() string {
	return filepath.Join(layout.repositoriesRoot, layout.host)
}

func (layout Layout) OwnerRoot() string {
	return filepath.Join(layout.HostRoot(), layout.owner)
}

func (layout Layout) HostConfigPath() string {
	return filepath.Join(layout.HostRoot(), ".aidb", "config.yaml")
}

func (layout Layout) OwnerConfigPath() string {
	return filepath.Join(layout.OwnerRoot(), ".aidb", "config.yaml")
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

func (layout Layout) CloneDir() string {
	return filepath.Join(layout.root, "repo")
}

func (layout Layout) TicketsDir() string {
	return filepath.Join(layout.root, "tickets")
}

func (layout Layout) WorktreesDir() string {
	return filepath.Join(layout.root, "work")
}

func (layout Layout) KnowledgeDir() string {
	return filepath.Join(layout.root, "knowledge")
}

func (layout Layout) ResolveRole(role string) (string, error) {
	if role == "" {
		return "", errors.New("repository role path is required")
	}
	if filepath.IsAbs(role) || filepath.VolumeName(role) != "" {
		return "", fmt.Errorf("repository role path must be relative: %q", role)
	}
	target := filepath.Clean(filepath.Join(layout.root, role))
	if err := requireContainedPath(layout.root, target); err != nil {
		return "", fmt.Errorf("resolve repository role %q: %w", role, err)
	}
	return target, nil
}

func requireContainedPath(root string, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve contained repository path: %w", err)
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repository path %q escapes root %q", target, root)
	}
	return nil
}
