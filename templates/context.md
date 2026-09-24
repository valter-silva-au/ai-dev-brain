{{/*
Frontmatter contract (TASK-00049): every value whose provenance is
user-supplied text MUST go through yamlq. A raw value containing ": "
renders invalid YAML (plain scalars cannot hold ": ") and breaks every
strict reader (VS Code Markdown preview, adb validate). Machine-generated
keys (task, template_version, generated_at) stay raw on purpose: the drift
regex in ticketdrift.go reads them unquoted, and yamlq would emit them
unchanged anyway. Do not remove yamlq when editing this template.
*/}}{{if .TemplateVersion}}---
template: ticket/context
template_version: {{.TemplateVersion}}
task: {{.TaskID}}
title: {{yamlq .Title}}
generated_at: {{.CreatedAt}}
---

{{end}}

# Context: {{.Title}}

**Task ID:** {{.TaskID}}
**Status:** {{.Status}}
**Created:** {{.CreatedAt}}

> **This file is for:** the WHY — the problem statement, root cause, scope and
> acceptance criteria. It is the sole carrier of the task description. **Not
> for:** progress narrative (`notes.md`), the technical design (`design.md`),
> or point-in-time state (`status.yaml`). **Write protocol:** amend in place
> when scope changes; keep the description the single source others cite.

## Description

{{.Description}}

## Acceptance Criteria

{{range .AcceptanceCriteria}}
- [ ] {{.}}
{{else}}
- [ ] Define acceptance criteria
{{end}}

## Dependencies

{{range .Dependencies}}
- {{.}}
{{else}}
- No dependencies
{{end}}

## Related Tasks

{{.RelatedTasks}}
