package core

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
)

// This file implements the LAYERED TEMPLATE RESOLUTION (TASK-00031): templates
// are provisioned defaults, not baked behaviour. adb ships embedded defaults
// under templates/; `adb templates provision` copies them into
// $ADB_HOME/templates/; from then on the WORKSPACE copy wins and is user-owned
// (edit/add/remove without rebuilding adb), with the embedded copy as fallback.
//
// Resolution rule (mirrors `adb program next`'s resolved from:/write: design —
// templates are referenced by id + version, never a filesystem path):
//
//	render(templateId) =
//	  $ADB_HOME/templates/<name>   if present   ← user-owned, wins
//	  else embedded default                     ← adb ships it
//
// template_version is the first 8 hex chars of the SHA-256 of the template's
// BYTES — so a ticket stamped at bootstrap can later be compared against the
// currently resolved template and flagged as stale (validate-time drift
// detection; catches workspaces bootstrapped by stale adb binaries).

// TicketTemplateIDs are the template ids (and, per the flat layout, file names)
// layered resolution knows about, in the order `adb templates provision`
// exports them.
var TicketTemplates = []TemplateType{
	TemplateTypeContext,
	TemplateTypeNotes,
	TemplateTypeDesign,
	TemplateTypeHandoff,
	TemplateTypeStatus,
	TemplateTypeTaskContext,
}

// TemplateVersion returns the 8-hex-char version stamp for template content:
// the first 8 hex digits of SHA-256 over the template file's bytes. The stamp
// travels into ticket frontmatter at render time so drift detection can
// compare a ticket's stamped version against the currently resolved template.
func TemplateVersion(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:4])
}

// resolvedTemplate is one parsed template plus the metadata drift detection
// needs: where it came from and its content hash.
type resolvedTemplate struct {
	tmpl    *template.Template
	source  string // "embedded" or the workspace directory it was read from
	version string // TemplateVersion over the file bytes
}

// LayeredTemplateManager implements TemplateManager with workspace-first
// resolution: a template provisioned into workspaceDir wins; the embedded
// fallback ships with adb. The workspace layer is OPTIONAL — a workspace with
// no provisioned templates renders exactly as the embedded-only manager did.
type LayeredTemplateManager struct {
	fsys         fs.FS
	workspaceDir string // $ADB_HOME/templates; "" disables the workspace layer
	resolved     map[TemplateType]resolvedTemplate
}

// NewLayeredTemplateManager creates a layered manager over the embedded
// defaults in fsys, with workspace overrides read from workspaceDir
// (pass "" to disable the layer, e.g. in tests). All embedded ticket templates
// must parse at construction so a broken ship fails fast.
func NewLayeredTemplateManager(workspaceDir string, embedded fs.FS) (*LayeredTemplateManager, error) {
	lm := &LayeredTemplateManager{
		fsys:         embedded,
		workspaceDir: workspaceDir,
		resolved:     make(map[TemplateType]resolvedTemplate),
	}
	for _, tt := range TicketTemplates {
		if err := lm.load(tt); err != nil {
			return nil, fmt.Errorf("failed to load template %s: %w", tt, err)
		}
	}
	return lm, nil
}

// resolve returns the resolved template for templateType, preferring the
// workspace copy. It re-resolves on each miss so a template edited (or
// provisioned) mid-process is picked up without rebuilding the manager.
func (l *LayeredTemplateManager) resolve(templateType TemplateType) (resolvedTemplate, error) {
	if l.workspaceDir != "" {
		wsPath := filepath.Join(l.workspaceDir, string(templateType))
		if content, err := os.ReadFile(wsPath); err == nil {
			return l.parse(templateType, content, l.workspaceDir)
		} else if !os.IsNotExist(err) {
			return resolvedTemplate{}, fmt.Errorf("failed to read workspace template %s: %w", templateType, err)
		}
	}
	content, err := fs.ReadFile(l.fsys, string(templateType))
	if err != nil {
		return resolvedTemplate{}, fmt.Errorf("failed to read template file: %w", err)
	}
	return l.parse(templateType, content, "embedded")
}

func (l *LayeredTemplateManager) parse(templateType TemplateType, content []byte, source string) (resolvedTemplate, error) {
	tmpl, err := template.New(string(templateType)).Funcs(ticketFuncMap).Parse(string(content))
	if err != nil {
		return resolvedTemplate{}, fmt.Errorf("failed to parse template: %w", err)
	}
	rt := resolvedTemplate{tmpl: tmpl, source: source, version: TemplateVersion(content)}
	l.resolved[templateType] = rt
	return rt, nil
}

// load parses the EMBEDDED copy of templateType (the constructor's fail-fast
// path; workspace overrides are resolved lazily per render).
func (l *LayeredTemplateManager) load(templateType TemplateType) error {
	content, err := fs.ReadFile(l.fsys, string(templateType))
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}
	_, err = l.parse(templateType, content, "embedded")
	return err
}

// Version returns the version stamp of the template as currently resolved
// (workspace copy if present, else embedded). Unknown templates resolve lazily.
func (l *LayeredTemplateManager) Version(templateType TemplateType) (string, error) {
	rt, err := l.resolve(templateType)
	if err != nil {
		return "", err
	}
	return rt.version, nil
}

// Source returns where the template currently resolves from: "embedded" or the
// workspace directory holding the overriding copy.
func (l *LayeredTemplateManager) Source(templateType TemplateType) (string, error) {
	rt, err := l.resolve(templateType)
	if err != nil {
		return "", err
	}
	return rt.source, nil
}

// Render implements TemplateManager.
func (l *LayeredTemplateManager) Render(templateType TemplateType, data interface{}) (string, error) {
	b, err := l.RenderBytes(templateType, data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RenderBytes implements TemplateManager.
func (l *LayeredTemplateManager) RenderBytes(templateType TemplateType, data interface{}) ([]byte, error) {
	rt, err := l.resolve(templateType)
	if err != nil {
		return nil, fmt.Errorf("template %s not found", templateType)
	}
	var buf bytes.Buffer
	if err := rt.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// ProvisionTemplates copies the embedded ticket templates into destDir (the
// workspace template layer, $ADB_HOME/templates/). Semantics are the shared
// pack-installer set: unchanged / differ-skipped (never silently clobber a
// user edit) / --force overwrite, with DryRun planning only. Dirs are created
// 0o755, files written 0o644.
func ProvisionTemplates(embedded fs.FS, destDir string, opts HarnessInstallOptions) (HarnessInstallResult, error) {
	if destDir == "" {
		return HarnessInstallResult{}, fmt.Errorf("template destination dir not resolved")
	}
	res := HarnessInstallResult{ClaudeDir: destDir, DryRun: opts.DryRun}
	for _, tt := range TicketTemplates {
		content, err := fs.ReadFile(embedded, string(tt))
		if err != nil {
			return HarnessInstallResult{}, fmt.Errorf("reading embedded template %s: %w", tt, err)
		}
		dest := filepath.Join(destDir, string(tt))
		action, err := installHarnessFile(dest, content, opts)
		if err != nil {
			return HarnessInstallResult{}, err
		}
		res.Entries = append(res.Entries, HarnessInstallEntry{Kind: HarnessKind(tt), Dest: dest, Action: action})
	}
	return res, nil
}
