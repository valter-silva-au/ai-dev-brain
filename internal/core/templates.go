package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// TemplateManager defines the interface for applying and managing task templates.
type TemplateManager interface {
	ApplyTemplate(ticketPath string, templateType models.TaskType) error
	GetTemplate(taskType models.TaskType) (string, error)
	RegisterTemplate(taskType models.TaskType, templatePath string) error
}

// templateManager implements TemplateManager with built-in defaults
// and support for custom template overrides.
type templateManager struct {
	basePath        string
	customTemplates map[models.TaskType]string
}

// NewTemplateManager creates a new TemplateManager rooted at basePath.
func NewTemplateManager(basePath string) TemplateManager {
	return &templateManager{
		basePath:        basePath,
		customTemplates: make(map[models.TaskType]string),
	}
}

// ApplyTemplate writes the resolved template content for the given task type
// to notes.md and design.md inside ticketPath.
func (tm *templateManager) ApplyTemplate(ticketPath string, templateType models.TaskType) error {
	// Extract task ID from the ticket path (last path component).
	taskID := filepath.Base(ticketPath)
	data := templateData{TaskID: taskID}

	notesContent, err := tm.renderTemplate(templateType, "notes", data)
	if err != nil {
		return fmt.Errorf("rendering notes template for %s: %w", templateType, err)
	}

	designContent, err := tm.renderTemplate(templateType, "design", data)
	if err != nil {
		return fmt.Errorf("rendering design template for %s: %w", templateType, err)
	}

	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		return fmt.Errorf("creating ticket directory %s: %w", ticketPath, err)
	}

	notesPath := filepath.Join(ticketPath, "notes.md")
	if err := os.WriteFile(notesPath, []byte(notesContent), 0o644); err != nil {
		return fmt.Errorf("writing notes.md: %w", err)
	}

	designPath := filepath.Join(ticketPath, "design.md")
	if err := os.WriteFile(designPath, []byte(designContent), 0o644); err != nil {
		return fmt.Errorf("writing design.md: %w", err)
	}

	return nil
}

// GetTemplate returns the raw template string for the given task type.
// If a custom template has been registered, its file contents are returned.
// Otherwise the built-in default is returned.
func (tm *templateManager) GetTemplate(taskType models.TaskType) (string, error) {
	if customPath, ok := tm.customTemplates[taskType]; ok {
		data, err := os.ReadFile(customPath)
		if err != nil {
			return "", fmt.Errorf("reading custom template %s: %w", customPath, err)
		}
		return string(data), nil
	}

	tmpl, ok := builtinNotesTemplates[taskType]
	if !ok {
		return "", fmt.Errorf("no template found for task type %q", taskType)
	}
	return tmpl, nil
}

// RegisterTemplate registers a custom template file that overrides the
// built-in default for the given task type.
func (tm *templateManager) RegisterTemplate(taskType models.TaskType, templatePath string) error {
	absPath := templatePath
	if !filepath.IsAbs(templatePath) {
		absPath = filepath.Join(tm.basePath, templatePath)
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("custom template file %s: %w", absPath, err)
	}

	tm.customTemplates[taskType] = absPath
	return nil
}

// templateData holds values that can be referenced in Go templates.
type templateData struct {
	TaskID string
}

// renderTemplate resolves the appropriate template for the given task type and
// file kind ("notes" or "design") and returns the rendered content.
func (tm *templateManager) renderTemplate(taskType models.TaskType, fileKind string, data templateData) (string, error) {
	// Check for a custom template first (custom templates are notes-only).
	if customPath, ok := tm.customTemplates[taskType]; ok && fileKind == "notes" {
		raw, err := os.ReadFile(customPath)
		if err != nil {
			return "", fmt.Errorf("reading custom template %s: %w", customPath, err)
		}
		return string(raw), nil
	}

	var templateMap map[models.TaskType]string
	switch fileKind {
	case "notes":
		templateMap = builtinNotesTemplates
	case "design":
		templateMap = builtinDesignTemplates
	default:
		return "", fmt.Errorf("unknown template file kind %q", fileKind)
	}

	raw, ok := templateMap[taskType]
	if !ok {
		return "", fmt.Errorf("no %s template for task type %q", fileKind, taskType)
	}

	tmpl, err := template.New(fileKind).Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing %s template for %s: %w", fileKind, taskType, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing %s template for %s: %w", fileKind, taskType, err)
	}

	return buf.String(), nil
}

// builtinNotesTemplates contains the default notes.md templates per task type.
var builtinNotesTemplates = map[models.TaskType]string{
	models.TaskTypeFeat: `# Feature Notes

## Requirements
- [ ] Requirement 1
- [ ] Requirement 2

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Implementation Notes

## Open Questions
`,

	models.TaskTypeBug: `# Bug Notes

## Description

## Steps to Reproduce
1.
2.
3.

## Expected Behavior

## Actual Behavior

## Root Cause Analysis

## Fix Notes
`,

	models.TaskTypeSpike: `# Spike Notes

## Objective

## Research Questions
- [ ] Question 1
- [ ] Question 2

## Findings

## Recommendations

## Time-Box
`,

	models.TaskTypeRefactor: `# Refactor Notes

## Motivation

## Current State

## Target State

## Affected Components
-

## Risks
-

## Rollback Plan
`,
}

// builtinDesignTemplates contains the default design.md templates per task type.
var builtinDesignTemplates = map[models.TaskType]string{
	models.TaskTypeFeat: `# Technical Design: {{.TaskID}}

## Overview

## Architecture

## API Changes

## Data Model Changes

## Dependencies

## Testing Strategy

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
|          |           |      |
`,

	models.TaskTypeBug: `# Technical Design: {{.TaskID}}

## Root Cause

## Fix Approach

## Affected Components

## Regression Testing

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
|          |           |      |
`,

	models.TaskTypeSpike: `# Technical Design: {{.TaskID}}

## Investigation Scope

## Approaches Evaluated

## Proof of Concept

## Recommendations

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
|          |           |      |
`,

	models.TaskTypeRefactor: `# Technical Design: {{.TaskID}}

## Current Architecture

## Target Architecture

## Migration Plan

## Breaking Changes

## Performance Impact

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
|          |           |      |
`,
}
