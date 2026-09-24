{{/*
Frontmatter contract (TASK-00049): every value whose provenance is
user-supplied text MUST go through yamlq. A raw value containing ": "
renders invalid YAML (plain scalars cannot hold ": ") and breaks every
strict reader (VS Code Markdown preview, adb validate). Machine-generated
keys (task, template_version, generated_at) stay raw on purpose: the drift
regex in ticketdrift.go reads them unquoted, and yamlq would emit them
unchanged anyway. Do not remove yamlq when editing this template.
*/}}{{if .TemplateVersion}}---
template: ticket/design
template_version: {{.TemplateVersion}}
task: {{.TaskID}}
title: {{yamlq .Title}}
generated_at: {{.CreatedAt}}
---

{{end}}

# Design Document: {{.Title}}

**Task ID:** {{.TaskID}}
**Created:** {{.CreatedAt}}

> **This file is for:** the technical design — root cause, architecture,
> interfaces, build plan. **Not for:** the problem statement (`context.md`) or
> progress narrative (`notes.md`). **Write protocol:** approved design before
> code; amend through the design loop, never silently while building.

## Overview

{{.Overview}}

## Architecture

### Components

{{.Components}}

### Data Flow

{{.DataFlow}}

### Dependencies

{{range .Dependencies}}
- {{.}}
{{else}}
- No external dependencies
{{end}}

## Implementation Plan

{{.ImplementationPlan}}

## Technical Decisions

{{.TechnicalDecisions}}

## Open Questions

{{.OpenQuestions}}
