package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SerenaProvisioner writes a per-worktree Serena project config so code-nav
// activates each worktree as its own project (#202). Implementations MUST be
// idempotent and non-clobbering (a repo that commits its own .serena/project.yml
// is left byte-unchanged), and callers treat them fail-open — a provisioning
// error must never block worktree/branch/ticket creation.
type SerenaProvisioner interface {
	// Provision writes <worktreePath>/.serena/project.yml when absent, seeding
	// project_name and detecting languages from the worktree's files.
	Provision(worktreePath, projectName string) error
}

// serenaProjectConfig is the subset of Serena's project.yml schema adb writes.
// Field order is the marshalled order (yaml.v3), matching a hand-written config.
type serenaProjectConfig struct {
	ProjectName               string   `yaml:"project_name"`
	Languages                 []string `yaml:"languages"`
	Encoding                  string   `yaml:"encoding"`
	IgnoreAllFilesInGitignore bool     `yaml:"ignore_all_files_in_gitignore"`
	IgnoredPaths              []string `yaml:"ignored_paths"`
	ReadOnly                  bool     `yaml:"read_only"`
}

// serenaIgnoredPaths is a cross-language build-noise safety net layered on top
// of ignore_all_files_in_gitignore, for repos whose .gitignore is thin (#202).
func serenaIgnoredPaths() []string {
	return []string{"node_modules", "vendor", "dist", "build", "target", ".venv", "__pycache__"}
}

// DefaultSerenaProvisioner writes .serena/project.yml, choosing languages via
// the #201 detector. It only *configures* Serena — it never installs or invokes
// a language server (honest gate, like tmux/gitleaks/gh).
type DefaultSerenaProvisioner struct{}

// NewSerenaProvisioner returns the default file-writing SerenaProvisioner.
func NewSerenaProvisioner() *DefaultSerenaProvisioner { return &DefaultSerenaProvisioner{} }

// Provision writes <worktreePath>/.serena/project.yml when one does not already
// exist. Non-clobbering (an existing config — e.g. a repo that commits its own —
// is left untouched) and, when it does write, it excludes the generated .serena/
// from the checkout's VCS so it isn't a spurious change-to-commit.
func (p *DefaultSerenaProvisioner) Provision(worktreePath, projectName string) error {
	if worktreePath == "" {
		return fmt.Errorf("worktreePath cannot be empty")
	}
	configPath := filepath.Join(worktreePath, ".serena", "project.yml")

	// Non-clobbering: leave any existing config byte-unchanged.
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	langs, err := DetectWorktreeLanguages(worktreePath)
	if err != nil {
		return fmt.Errorf("failed to detect worktree languages: %w", err)
	}

	data, err := yaml.Marshal(serenaProjectConfig{
		ProjectName:               projectName,
		Languages:                 langs,
		Encoding:                  "utf-8",
		IgnoreAllFilesInGitignore: true,
		IgnoredPaths:              serenaIgnoredPaths(),
		ReadOnly:                  false,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal serena config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("failed to create .serena directory: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write serena config: %w", err)
	}

	// We just created .serena/ (the repo didn't track one), so keep it out of
	// the checkout's version control — best-effort, never fatal.
	if err := excludeFromWorktreeVCS(worktreePath, ".serena/"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: wrote %s but could not exclude .serena/ from VCS: %v\n", configPath, err)
	}
	return nil
}

// excludeFromWorktreeVCS appends pattern to the worktree repo's git exclude file
// (idempotently) so an adb-generated path doesn't surface as an untracked
// change. It resolves the common git dir from the worktree's .git (a dir for a
// normal checkout, a "gitdir:" pointer file for a linked worktree). Pure file
// IO — no git binary is invoked.
func excludeFromWorktreeVCS(worktreePath, pattern string) error {
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return fmt.Errorf("no .git in worktree: %w", err)
	}

	var commonGitDir string
	if info.IsDir() {
		commonGitDir = gitPath
	} else {
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return fmt.Errorf("failed to read .git pointer: %w", err)
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(content)), "gitdir:"))
		// gitdir points at <repo>/.git/worktrees/<name>; the shared exclude
		// lives in the common <repo>/.git.
		commonGitDir = filepath.Dir(filepath.Dir(gitdir))
	}

	excludePath := filepath.Join(commonGitDir, "info", "exclude")
	if existing, err := os.ReadFile(excludePath); err == nil {
		// Already excluded — nothing to do.
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == pattern {
				return nil
			}
		}
		if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
			pattern = "\n" + pattern
		}
	}

	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("failed to create git info dir: %w", err)
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open git exclude: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(pattern + "\n"); err != nil {
		return fmt.Errorf("failed to append to git exclude: %w", err)
	}
	return nil
}
