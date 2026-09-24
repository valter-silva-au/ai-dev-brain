package core

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"text/template"
)

// TemplateType represents the type of template to render
type TemplateType string

const (
	// TemplateTypeNotes represents a notes.md template
	TemplateTypeNotes TemplateType = "notes.md"
	// TemplateTypeDesign represents a design.md template
	TemplateTypeDesign TemplateType = "design.md"
	// TemplateTypeHandoff represents a handoff.md template
	TemplateTypeHandoff TemplateType = "handoff.md"
	// TemplateTypeStatus represents a status.yaml template
	TemplateTypeStatus TemplateType = "status.yaml"
	// TemplateTypeContext represents a context.md template
	TemplateTypeContext TemplateType = "context.md"
	// TemplateTypeTaskContext represents a task-context.md template for .claude/rules/
	TemplateTypeTaskContext TemplateType = "task-context.md"
)

// TemplateVersioner is implemented by template managers that can report the
// version stamp of the template they currently resolve (8-hex content hash).
// The bootstrap render path uses it to stamp ticket frontmatter.
type TemplateVersioner interface {
	// Version returns the version stamp of the resolved template.
	Version(templateType TemplateType) (string, error)
}

// TemplateID maps a template type to its stable template id — the frontmatter
// reference baked into rendered tickets. Ids are stable NAMES, never paths:
// where the template actually lives (workspace copy vs embedded default) is
// resolved by the layered manager, mirroring `adb program next`'s resolved
// from:/write: design (TASK-00031).
func TemplateID(templateType TemplateType) string {
	switch templateType {
	case TemplateTypeContext:
		return "ticket/context"
	case TemplateTypeNotes:
		return "ticket/notes"
	case TemplateTypeDesign:
		return "ticket/design"
	case TemplateTypeHandoff:
		return "ticket/handoff"
	case TemplateTypeStatus:
		return "ticket/status"
	case TemplateTypeTaskContext:
		return "ticket/task-context"
	}
	return "ticket/" + string(templateType)
}

// ticketFuncMap is the FuncMap every ticket template is parsed with.
// `yamlq` emitter-quotes a value for a YAML scalar slot (title, assignee,
// tags…): YAML plain scalars cannot contain ": ", so a raw {{.Title}} with a
// colon renders an invalid nested mapping (TASK-00049). yamlq is MANDATORY for
// any frontmatter value whose provenance is user-supplied text. yaml.v3 emits
// a plain scalar wherever the grammar allows, so machine-generated values
// (task ids, enums, RFC3339 timestamps) pass through unchanged.
var ticketFuncMap = template.FuncMap{"yamlq": YAMLScalar}

// TemplateManager defines the interface for rendering templates
type TemplateManager interface {
	// Render renders a template with the given data
	Render(templateType TemplateType, data interface{}) (string, error)
	// RenderBytes renders a template with the given data and returns bytes
	RenderBytes(templateType TemplateType, data interface{}) ([]byte, error)
}

// EmbedTemplateManager implements TemplateManager using embedded templates
type EmbedTemplateManager struct {
	fsys      fs.FS
	templates map[TemplateType]*template.Template
}

// NewEmbedTemplateManager creates a new template manager backed by fsys.
// In production fsys is claude.FS (an embed.FS, which satisfies fs.FS); tests can
// pass any fs.FS carrying the template files.
func NewEmbedTemplateManager(fsys fs.FS) (*EmbedTemplateManager, error) {
	tm := &EmbedTemplateManager{
		fsys:      fsys,
		templates: make(map[TemplateType]*template.Template),
	}

	// Pre-parse all templates
	templateTypes := []TemplateType{
		TemplateTypeNotes,
		TemplateTypeDesign,
		TemplateTypeHandoff,
		TemplateTypeStatus,
		TemplateTypeContext,
		TemplateTypeTaskContext,
	}

	for _, tt := range templateTypes {
		if err := tm.loadTemplate(tt); err != nil {
			return nil, fmt.Errorf("failed to load template %s: %w", tt, err)
		}
	}

	return tm, nil
}

// loadTemplate loads and parses a template file from the embedded filesystem
func (tm *EmbedTemplateManager) loadTemplate(templateType TemplateType) error {
	// Use path.Join (not filepath.Join) for embed.FS paths
	templatePath := path.Join(string(templateType))

	// Read template content from the backing filesystem
	content, err := fs.ReadFile(tm.fsys, templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse template — with the shared ticket FuncMap so templates can
	// emitter-quote YAML frontmatter values ({{yamlq .Title}}).
	tmpl, err := template.New(string(templateType)).Funcs(ticketFuncMap).Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	tm.templates[templateType] = tmpl
	return nil
}

// Render renders a template with the given data and returns the result as a string
func (tm *EmbedTemplateManager) Render(templateType TemplateType, data interface{}) (string, error) {
	bytes, err := tm.RenderBytes(templateType, data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// RenderBytes renders a template with the given data and returns the result as bytes
func (tm *EmbedTemplateManager) RenderBytes(templateType TemplateType, data interface{}) ([]byte, error) {
	tmpl, ok := tm.templates[templateType]
	if !ok {
		// Not in the pre-parsed set: try loading it on demand so a template
		// dropped into the embedded filesystem becomes renderable without a code
		// change. If the file genuinely doesn't exist, fall back to the original
		// "not found" contract.
		if err := tm.loadTemplate(templateType); err != nil {
			return nil, fmt.Errorf("template %s not found", templateType)
		}
		tmpl = tm.templates[templateType]
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// Version implements TemplateVersioner: the 8-hex stamp of the embedded
// template file's bytes.
func (tm *EmbedTemplateManager) Version(templateType TemplateType) (string, error) {
	content, err := fs.ReadFile(tm.fsys, path.Join(string(templateType)))
	if err != nil {
		return "", fmt.Errorf("template %s not found", templateType)
	}
	return TemplateVersion(content), nil
}
