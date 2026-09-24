package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ScopeKind string

const (
	ScopeBuiltin      ScopeKind = "builtin"
	ScopeUserGlobal   ScopeKind = "user_global"
	ScopeWorkspace    ScopeKind = "workspace"
	ScopeOrganization ScopeKind = "organization"
	ScopeHost         ScopeKind = "host"
	ScopeOwner        ScopeKind = "owner"
	ScopeRepository   ScopeKind = "repository"
	ScopeTicket       ScopeKind = "ticket"
	ScopeEnvironment  ScopeKind = "environment"
	ScopeRequest      ScopeKind = "request"
)

type Source struct {
	Kind     ScopeKind `json:"kind" yaml:"kind"`
	Name     string    `json:"name" yaml:"name"`
	Path     string    `json:"path,omitempty" yaml:"path,omitempty"`
	Portable bool      `json:"portable" yaml:"portable"`
}

type Layer struct {
	Source   Source   `json:"source" yaml:"source"`
	Document Document `json:"document" yaml:"document"`
}

type Override struct {
	Name   string `json:"name" yaml:"name"`
	Path   string `json:"path" yaml:"path"`
	Value  any    `json:"value" yaml:"value"`
	Secret bool   `json:"secret" yaml:"secret"`
}

type ScopePaths struct {
	WorkspaceRoot    string `json:"workspace_root" yaml:"workspace_root"`
	UserGlobalPath   string `json:"user_global_path,omitempty" yaml:"user_global_path,omitempty"`
	OrganizationRoot string `json:"organization_root,omitempty" yaml:"organization_root,omitempty"`
	HostRoot         string `json:"host_root,omitempty" yaml:"host_root,omitempty"`
	OwnerRoot        string `json:"owner_root,omitempty" yaml:"owner_root,omitempty"`
	RepositoryRoot   string `json:"repository_root,omitempty" yaml:"repository_root,omitempty"`
	TicketRoot       string `json:"ticket_root,omitempty" yaml:"ticket_root,omitempty"`
}

func DiscoverSources(paths ScopePaths) ([]Source, error) {
	workspaceRoot, err := canonicalScopeRoot(
		"workspace",
		paths.WorkspaceRoot,
	)
	if err != nil {
		return nil, err
	}
	sources := make([]Source, 0, 7)
	if paths.UserGlobalPath != "" {
		if !filepath.IsAbs(paths.UserGlobalPath) {
			return nil, fmt.Errorf(
				"user-global configuration path must be absolute: %q",
				paths.UserGlobalPath,
			)
		}
		sources = append(sources, Source{
			Kind: ScopeUserGlobal,
			Name: "user-global",
			Path: filepath.Clean(paths.UserGlobalPath),
		})
	}
	sources = append(sources, Source{
		Kind:     ScopeWorkspace,
		Name:     "workspace",
		Path:     configPath(workspaceRoot),
		Portable: true,
	})

	organizationRoot, err := appendScopeSource(
		&sources,
		ScopeOrganization,
		"organization",
		paths.OrganizationRoot,
		workspaceRoot,
	)
	if err != nil {
		return nil, err
	}
	hostRoot, err := appendScopeSource(
		&sources,
		ScopeHost,
		"source host",
		paths.HostRoot,
		organizationRoot,
	)
	if err != nil {
		return nil, err
	}
	ownerRoot, err := appendScopeSource(
		&sources,
		ScopeOwner,
		"account owner",
		paths.OwnerRoot,
		hostRoot,
	)
	if err != nil {
		return nil, err
	}
	repositoryRoot, err := appendScopeSource(
		&sources,
		ScopeRepository,
		"repository",
		paths.RepositoryRoot,
		ownerRoot,
	)
	if err != nil {
		return nil, err
	}
	ticketParent := repositoryRoot
	if ticketParent == "" {
		ticketParent = organizationRoot
	}
	if _, err := appendScopeSource(
		&sources,
		ScopeTicket,
		"ticket",
		paths.TicketRoot,
		ticketParent,
	); err != nil {
		return nil, err
	}
	return sources, nil
}

func appendScopeSource(
	sources *[]Source,
	kind ScopeKind,
	name string,
	root string,
	parent string,
) (string, error) {
	if root == "" {
		return "", nil
	}
	if parent == "" {
		return "", fmt.Errorf(
			"%s scope requires its parent semantic scope",
			name,
		)
	}
	canonical, err := canonicalScopeRoot(name, root)
	if err != nil {
		return "", err
	}
	if err := requireContainedScope(parent, canonical); err != nil {
		return "", fmt.Errorf("validate %s scope: %w", name, err)
	}
	*sources = append(*sources, Source{
		Kind:     kind,
		Name:     name,
		Path:     configPath(canonical),
		Portable: true,
	})
	return canonical, nil
}

func canonicalScopeRoot(name string, root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%s root is required", name)
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%s root must be absolute: %q", name, root)
	}
	cleaned := filepath.Clean(root)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve %s root %q: %w", name, root, err)
	}
	cleaned = filepath.Clean(resolved)
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("inspect %s root %q: %w", name, root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s root must be a directory: %q", name, root)
	}
	return cleaned, nil
}

func requireContainedScope(parent string, child string) error {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return fmt.Errorf("resolve scope containment: %w", err)
	}
	if relative == "." {
		return errors.New("semantic scope cannot equal its parent")
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf(
			"scope %q escapes parent %q",
			child,
			parent,
		)
	}
	return nil
}

func configPath(root string) string {
	return filepath.Join(root, ".aidb", "config.yaml")
}
