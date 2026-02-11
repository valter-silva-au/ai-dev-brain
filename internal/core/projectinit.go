package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// InitConfig holds the parameters for initializing a project workspace.
type InitConfig struct {
	BasePath string
	Name     string
	AI       string
	Prefix   string
}

// InitResult holds a summary of what was created vs. skipped.
type InitResult struct {
	Created []string
	Skipped []string
}

// ProjectInitializer defines the interface for initializing a full
// project workspace with the recommended directory structure.
type ProjectInitializer interface {
	Init(config InitConfig) (*InitResult, error)
}

type projectInitializer struct {
	docTemplates *DocTemplates
}

// NewProjectInitializer creates a new ProjectInitializer.
func NewProjectInitializer() ProjectInitializer {
	return &projectInitializer{
		docTemplates: NewDocTemplates(),
	}
}

// Init creates the full project workspace directory structure, configuration
// files, and documentation templates. It is safe to run on existing projects:
// files and directories that already exist are skipped and not overwritten.
func (pi *projectInitializer) Init(config InitConfig) (*InitResult, error) {
	result := &InitResult{}

	if config.AI == "" {
		config.AI = "claude"
	}
	if config.Prefix == "" {
		config.Prefix = "TASK"
	}
	if config.Name == "" {
		config.Name = filepath.Base(config.BasePath)
	}

	// Create all directories.
	dirs := []string{
		config.BasePath,
		filepath.Join(config.BasePath, "tickets"),
		filepath.Join(config.BasePath, "work"),
		filepath.Join(config.BasePath, "tools"),
		filepath.Join(config.BasePath, ".claude"),
		filepath.Join(config.BasePath, ".claude", "rules"),
		filepath.Join(config.BasePath, ".claude", "skills", "commit"),
		filepath.Join(config.BasePath, ".claude", "skills", "task-status"),
		filepath.Join(config.BasePath, ".claude", "skills", "push"),
		filepath.Join(config.BasePath, ".claude", "skills", "pr"),
		filepath.Join(config.BasePath, ".claude", "skills", "review"),
		filepath.Join(config.BasePath, ".claude", "skills", "sync"),
		filepath.Join(config.BasePath, ".claude", "skills", "changelog"),
		filepath.Join(config.BasePath, ".claude", "agents"),
		filepath.Join(config.BasePath, "docs"),
		filepath.Join(config.BasePath, "docs", "wiki"),
		filepath.Join(config.BasePath, "docs", "decisions"),
		filepath.Join(config.BasePath, "docs", "runbooks"),
	}
	for _, dir := range dirs {
		created, err := ensureDir(dir)
		if err != nil {
			return nil, fmt.Errorf("initializing project: creating directory %s: %w", dir, err)
		}
		if created {
			result.Created = append(result.Created, dir)
		} else {
			result.Skipped = append(result.Skipped, dir)
		}
	}

	// Write .taskconfig from template (rendered with text/template).
	taskconfigPath := filepath.Join(config.BasePath, ".taskconfig")
	if err := pi.writeFileIfNotExists(taskconfigPath, func() ([]byte, error) {
		return pi.renderTemplate("taskconfig.yaml", config)
	}, result); err != nil {
		return nil, err
	}

	// Write .task_counter.
	counterPath := filepath.Join(config.BasePath, ".task_counter")
	if err := pi.writeFileIfNotExists(counterPath, func() ([]byte, error) {
		return []byte("0"), nil
	}, result); err != nil {
		return nil, err
	}

	// Write .gitignore.
	gitignorePath := filepath.Join(config.BasePath, ".gitignore")
	if err := pi.writeStaticTemplate(gitignorePath, "gitignore", result); err != nil {
		return nil, err
	}

	// Write backlog.yaml.
	backlogPath := filepath.Join(config.BasePath, "backlog.yaml")
	if err := pi.writeFileIfNotExists(backlogPath, func() ([]byte, error) {
		return []byte("version: \"1.0\"\ntasks: {}\n"), nil
	}, result); err != nil {
		return nil, err
	}

	// Write CLAUDE.md (rendered with text/template).
	claudePath := filepath.Join(config.BasePath, "CLAUDE.md")
	if err := pi.writeFileIfNotExists(claudePath, func() ([]byte, error) {
		return pi.renderTemplate("claude-md.md", config)
	}, result); err != nil {
		return nil, err
	}

	// Write directory README files (static templates).
	readmeFiles := []struct {
		template string
		target   string
	}{
		{"readme-tickets.md", filepath.Join("tickets", "README.md")},
		{"readme-work.md", filepath.Join("work", "README.md")},
		{"readme-tools.md", filepath.Join("tools", "README.md")},
		{"readme-docs.md", filepath.Join("docs", "README.md")},
	}
	for _, rf := range readmeFiles {
		target := filepath.Join(config.BasePath, rf.target)
		if err := pi.writeStaticTemplate(target, rf.template, result); err != nil {
			return nil, err
		}
	}

	// Write .claude/ configuration files.
	claudeFiles := []struct {
		template string
		target   string
		render   bool // true if text/template rendering is needed
	}{
		{"claude-settings.json", filepath.Join(".claude", "settings.json"), false},
		{"claude-rules-workspace.md", filepath.Join(".claude", "rules", "workspace.md"), true},
		{"claude-skill-commit.md", filepath.Join(".claude", "skills", "commit", "SKILL.md"), false},
		{"claude-skill-task-status.md", filepath.Join(".claude", "skills", "task-status", "SKILL.md"), false},
		{"claude-skill-push.md", filepath.Join(".claude", "skills", "push", "SKILL.md"), false},
		{"claude-skill-pr.md", filepath.Join(".claude", "skills", "pr", "SKILL.md"), false},
		{"claude-skill-review.md", filepath.Join(".claude", "skills", "review", "SKILL.md"), false},
		{"claude-skill-sync.md", filepath.Join(".claude", "skills", "sync", "SKILL.md"), false},
		{"claude-skill-changelog.md", filepath.Join(".claude", "skills", "changelog", "SKILL.md"), false},
		{"claude-agent-code-reviewer.md", filepath.Join(".claude", "agents", "code-reviewer.md"), false},
	}
	for _, cf := range claudeFiles {
		target := filepath.Join(config.BasePath, cf.target)
		if cf.render {
			if err := pi.writeFileIfNotExists(target, func() ([]byte, error) {
				return pi.renderTemplate(cf.template, config)
			}, result); err != nil {
				return nil, err
			}
		} else {
			if err := pi.writeStaticTemplate(target, cf.template, result); err != nil {
				return nil, err
			}
		}
	}

	// Scaffold docs/ template files (reuses existing ScaffoldDocs which
	// handles skip-if-exists internally). Directories are already created
	// above, so only track the files here.
	docsFiles := []string{
		filepath.Join(config.BasePath, "docs", "stakeholders.md"),
		filepath.Join(config.BasePath, "docs", "contacts.md"),
		filepath.Join(config.BasePath, "docs", "glossary.md"),
		filepath.Join(config.BasePath, "docs", "decisions", "ADR-TEMPLATE.md"),
	}
	existsBefore := make(map[string]bool)
	for _, p := range docsFiles {
		if _, err := os.Stat(p); err == nil {
			existsBefore[p] = true
		}
	}

	if err := pi.docTemplates.ScaffoldDocs(config.BasePath); err != nil {
		return nil, fmt.Errorf("initializing project: scaffolding docs: %w", err)
	}

	for _, p := range docsFiles {
		if existsBefore[p] {
			result.Skipped = append(result.Skipped, p)
		} else {
			result.Created = append(result.Created, p)
		}
	}

	// Initialize git repository if not already one.
	gitDir := filepath.Join(config.BasePath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		cmd := exec.Command("git", "init")
		cmd.Dir = config.BasePath
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("initializing project: running git init: %w", err)
		}
		result.Created = append(result.Created, gitDir)
	} else {
		result.Skipped = append(result.Skipped, gitDir)
	}

	return result, nil
}

// ensureDir creates a directory if it does not exist. Returns true if created.
func ensureDir(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return false, err
	}
	return true, nil
}

// writeFileIfNotExists writes content from contentFn if the file does not exist.
// It records created/skipped in the result.
func (pi *projectInitializer) writeFileIfNotExists(path string, contentFn func() ([]byte, error), result *InitResult) error {
	if _, err := os.Stat(path); err == nil {
		result.Skipped = append(result.Skipped, path)
		return nil
	}
	content, err := contentFn()
	if err != nil {
		return fmt.Errorf("initializing project: generating content for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("initializing project: writing %s: %w", path, err)
	}
	result.Created = append(result.Created, path)
	return nil
}

// writeStaticTemplate reads a template by name and writes it to target path
// without any text/template rendering. Records created/skipped in result.
func (pi *projectInitializer) writeStaticTemplate(target, templateName string, result *InitResult) error {
	return pi.writeFileIfNotExists(target, func() ([]byte, error) {
		content, err := pi.docTemplates.GetTemplate(templateName)
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	}, result)
}

// renderTemplate reads a template by name, renders it with text/template
// using the given data, and returns the rendered bytes.
func (pi *projectInitializer) renderTemplate(templateName string, data interface{}) ([]byte, error) {
	tmplContent, err := pi.docTemplates.GetTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("loading template %s: %w", templateName, err)
	}
	tmpl, err := template.New(templateName).Parse(tmplContent)
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", templateName, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", templateName, err)
	}
	return buf.Bytes(), nil
}
