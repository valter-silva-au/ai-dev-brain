package core

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed templates/stakeholders.md templates/contacts.md templates/glossary.md templates/adr.md
var templateFS embed.FS

// DocTemplates provides access to embedded documentation templates.
type DocTemplates struct{}

// NewDocTemplates creates a new DocTemplates instance.
func NewDocTemplates() *DocTemplates {
	return &DocTemplates{}
}

// StakeholdersTemplate returns the embedded stakeholders.md template content.
func (dt *DocTemplates) StakeholdersTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/stakeholders.md")
	if err != nil {
		return "", fmt.Errorf("reading stakeholders template: %w", err)
	}
	return string(data), nil
}

// ContactsTemplate returns the embedded contacts.md template content.
func (dt *DocTemplates) ContactsTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/contacts.md")
	if err != nil {
		return "", fmt.Errorf("reading contacts template: %w", err)
	}
	return string(data), nil
}

// GlossaryTemplate returns the embedded glossary.md template content.
func (dt *DocTemplates) GlossaryTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/glossary.md")
	if err != nil {
		return "", fmt.Errorf("reading glossary template: %w", err)
	}
	return string(data), nil
}

// ADRTemplate returns the embedded ADR template content.
func (dt *DocTemplates) ADRTemplate() (string, error) {
	data, err := templateFS.ReadFile("templates/adr.md")
	if err != nil {
		return "", fmt.Errorf("reading ADR template: %w", err)
	}
	return string(data), nil
}

// ScaffoldDocs creates the docs/ directory structure under basePath on first run.
// It writes stakeholders.md, contacts.md, glossary.md, and a decisions/ directory
// with a placeholder ADR template. It skips any file that already exists.
func (dt *DocTemplates) ScaffoldDocs(basePath string) error {
	docsDir := filepath.Join(basePath, "docs")
	dirs := []string{
		docsDir,
		filepath.Join(docsDir, "wiki"),
		filepath.Join(docsDir, "decisions"),
		filepath.Join(docsDir, "runbooks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Write template files, skipping those that already exist.
	templates := map[string]func() (string, error){
		filepath.Join(docsDir, "stakeholders.md"): dt.StakeholdersTemplate,
		filepath.Join(docsDir, "contacts.md"):     dt.ContactsTemplate,
		filepath.Join(docsDir, "glossary.md"):      dt.GlossaryTemplate,
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

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	return nil
}
