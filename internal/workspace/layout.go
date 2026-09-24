package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")

type Layout struct {
	root string
}

func NewLayout(root string) (Layout, error) {
	if root == "" {
		return Layout{}, errors.New("workspace root is required")
	}
	if !filepath.IsAbs(root) {
		return Layout{}, fmt.Errorf("workspace root must be absolute: %q", root)
	}

	return Layout{root: filepath.Clean(root)}, nil
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

func (layout Layout) StatePath() string {
	return filepath.Join(layout.ControlDir(), "state.sqlite")
}

func (layout Layout) EventsDir() string {
	return filepath.Join(layout.ControlDir(), "events")
}

func (layout Layout) CacheDir() string {
	return filepath.Join(layout.ControlDir(), "cache")
}

func (layout Layout) OrganizationsDir() string {
	return filepath.Join(layout.root, "organizations")
}

func (layout Layout) ResolveRole(role string) (string, error) {
	if role == "" {
		return "", errors.New("workspace role path is required")
	}
	if filepath.IsAbs(role) || filepath.VolumeName(role) != "" {
		return "", fmt.Errorf("workspace role path must be relative: %q", role)
	}

	target := filepath.Clean(filepath.Join(layout.root, role))
	relative, err := filepath.Rel(layout.root, target)
	if err != nil {
		return "", fmt.Errorf("resolve workspace role %q: %w", role, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace role path escapes root: %q", role)
	}

	return target, nil
}

func Discover(start string) (Layout, Manifest, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return Layout{}, Manifest{}, fmt.Errorf("resolve discovery start %q: %w", start, err)
	}

	current := filepath.Clean(absolute)
	if info, statErr := os.Stat(current); statErr == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}

	for {
		layout, layoutErr := NewLayout(current)
		if layoutErr != nil {
			return Layout{}, Manifest{}, layoutErr
		}

		manifest, readErr := ReadManifest(layout.ManifestPath())
		if readErr == nil {
			if validateErr := manifest.Validate(layout); validateErr != nil {
				return Layout{}, Manifest{}, fmt.Errorf(
					"validate workspace manifest %q: %w",
					layout.ManifestPath(),
					validateErr,
				)
			}
			return layout, manifest, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return Layout{}, Manifest{}, readErr
		}

		parent := filepath.Dir(current)
		if parent == current {
			return Layout{}, Manifest{}, fmt.Errorf(
				"%w from %q",
				ErrWorkspaceNotFound,
				start,
			)
		}
		current = parent
	}
}
