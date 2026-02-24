package core

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed templates
var templateFS embed.FS

// docTemplates provides access to embedded documentation templates.
type docTemplates struct{}

// newDocTemplates creates a new docTemplates instance.
func newDocTemplates() *docTemplates {
	return &docTemplates{}
}

// StakeholdersTemplate returns the embedded stakeholders.md template content.
func (dt *docTemplates) StakeholdersTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/stakeholders.md")
	if err != nil {
		return "", fmt.Errorf("reading stakeholders template: %w", err)
	}
	return string(data), nil
}

// ContactsTemplate returns the embedded contacts.md template content.
func (dt *docTemplates) ContactsTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/contacts.md")
	if err != nil {
		return "", fmt.Errorf("reading contacts template: %w", err)
	}
	return string(data), nil
}

// GlossaryTemplate returns the embedded glossary.md template content.
func (dt *docTemplates) GlossaryTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/glossary.md")
	if err != nil {
		return "", fmt.Errorf("reading glossary template: %w", err)
	}
	return string(data), nil
}

// ADRTemplate returns the embedded ADR template content.
func (dt *docTemplates) ADRTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/adr.md")
	if err != nil {
		return "", fmt.Errorf("reading ADR template: %w", err)
	}
	return string(data), nil
}

// TaskconfigTemplate returns the embedded .taskconfig template content.
func (dt *docTemplates) TaskconfigTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/taskconfig.yaml")
	if err != nil {
		return "", fmt.Errorf("reading taskconfig template: %w", err)
	}
	return string(data), nil
}

// GetTemplate returns the content of an embedded template by filename.
// The name should be the filename within the templates/ directory
// (e.g., "claude-md.md", "readme-tickets.md").
func (dt *docTemplates) GetTemplate(name string) (string, error) {
	data, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", name, err)
	}
	return string(data), nil
}

// ScaffoldDocs creates the docs/ directory structure under basePath on first run.
// It writes stakeholders.md, contacts.md, glossary.md, and a decisions/ directory
// with a placeholder ADR template. It skips any file that already exists.
func (dt *docTemplates) ScaffoldDocs(basePath string) error {
	docsDir := filepath.Join(basePath, "docs")
	dirs := []string{
		docsDir,
		filepath.Join(docsDir, "wiki"),
		filepath.Join(docsDir, "decisions"),
		filepath.Join(docsDir, "runbooks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Write template files, skipping those that already exist.
	templates := map[string]func() (string, error){
		filepath.Join(docsDir, "stakeholders.md"):              dt.StakeholdersTemplate,
		filepath.Join(docsDir, "contacts.md"):                  dt.ContactsTemplate,
		filepath.Join(docsDir, "glossary.md"):                  dt.GlossaryTemplate,
		filepath.Join(docsDir, "decisions", "ADR-TEMPLATE.md"): dt.ADRTemplate,
	}

	for path, tmplFn := range templates {
		if _, err := os.Stat(path); err == nil {
			// File already exists, skip.
			continue
		}

		content, err := tmplFn()
		if err != nil {
			return err
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	return nil
}
