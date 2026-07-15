package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// newProvisionWorktree makes a temp dir that looks like a git checkout (a real
// .git dir so the VCS-exclude step has somewhere to write) seeded with files.
func newProvisionWorktree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatalf("mkdir .git/info: %v", err)
	}
	writeFiles(t, root, files) // helper from serena_langdetect_test.go
	return root
}

func TestSerenaProvisioner_WritesValidConfig(t *testing.T) {
	root := newProvisionWorktree(t, map[string]string{
		"main.go":          "package main",
		"internal/x.go":    "package internal",
		"scripts/build.sh": "#!/bin/sh",
	})

	p := NewSerenaProvisioner()
	if err := p.Provision(root, "TASK-00042-demo"); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".serena", "project.yml"))
	if err != nil {
		t.Fatalf("reading provisioned config: %v", err)
	}
	var cfg serenaProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("provisioned config is not valid YAML: %v", err)
	}
	if cfg.ProjectName != "TASK-00042-demo" {
		t.Errorf("project_name = %q, want TASK-00042-demo", cfg.ProjectName)
	}
	if len(cfg.Languages) == 0 || cfg.Languages[0] != "go" {
		t.Errorf("languages = %v, want go first (Go-heavy tree)", cfg.Languages)
	}
	if !cfg.IgnoreAllFilesInGitignore {
		t.Errorf("ignore_all_files_in_gitignore should be true")
	}
	if len(cfg.IgnoredPaths) == 0 {
		t.Errorf("ignored_paths should carry a non-empty noise safety-net")
	}

	// The generated .serena/ is excluded from the checkout's VCS.
	exclude, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("reading git exclude: %v", err)
	}
	if !strings.Contains(string(exclude), ".serena/") {
		t.Errorf("git exclude should contain .serena/, got: %q", string(exclude))
	}
}

func TestSerenaProvisioner_NonClobbering(t *testing.T) {
	root := newProvisionWorktree(t, map[string]string{"main.go": "package main"})

	// A repo that commits its own config (like this one): pre-seed it.
	serenaDir := filepath.Join(root, ".serena")
	if err := os.MkdirAll(serenaDir, 0o755); err != nil {
		t.Fatalf("mkdir .serena: %v", err)
	}
	sentinel := []byte("project_name: hand-authored\nlanguages: [go, typescript, bash]\n")
	cfgPath := filepath.Join(serenaDir, "project.yml")
	if err := os.WriteFile(cfgPath, sentinel, 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	p := NewSerenaProvisioner()
	if err := p.Provision(root, "TASK-00043-demo"); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("existing config was clobbered.\n got: %q\nwant: %q", string(got), string(sentinel))
	}
}

func TestSerenaProvisioner_Idempotent(t *testing.T) {
	root := newProvisionWorktree(t, map[string]string{"main.go": "package main"})
	p := NewSerenaProvisioner()
	if err := p.Provision(root, "demo"); err != nil {
		t.Fatalf("first Provision failed: %v", err)
	}
	cfgPath := filepath.Join(root, ".serena", "project.yml")
	first, _ := os.ReadFile(cfgPath)

	if err := p.Provision(root, "demo-changed"); err != nil {
		t.Fatalf("second Provision failed: %v", err)
	}
	second, _ := os.ReadFile(cfgPath)
	if string(first) != string(second) {
		t.Errorf("second Provision changed an existing config (should be idempotent)")
	}
}

func TestSerenaProvisioner_EmptyWorktreePathErrors(t *testing.T) {
	if err := NewSerenaProvisioner().Provision("", "demo"); err == nil {
		t.Error("Provision(\"\") should error")
	}
}
