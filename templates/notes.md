{{/*
Frontmatter contract (TASK-00049): every value whose provenance is
user-supplied text MUST go through yamlq. A raw value containing ": "
renders invalid YAML (plain scalars cannot hold ": ") and breaks every
strict reader (VS Code Markdown preview, adb validate). Machine-generated
keys (task, template_version, generated_at) stay raw on purpose: the drift
regex in ticketdrift.go reads them unquoted, and yamlq would emit them
unchanged anyway. Do not remove yamlq when editing this template.
*/}}{{if .TemplateVersion}}---
template: ticket/notes
template_version: {{.TemplateVersion}}
task: {{.TaskID}}
title: {{yamlq .Title}}
generated_at: {{.CreatedAt}}
---

{{end}}

# Notes: {{.Title}}

**Task ID:** {{.TaskID}}
**Created:** {{.CreatedAt}}

> **This file is for:** the progress log — what was tried, what was decided,
> what remains, dated per session. **Not for:** the problem statement
> (`context.md`), the technical design (`design.md`), decisions of record
> (`knowledge/decisions.yaml`), or a full session transcript (`sessions/`).
> **Write protocol:** append-only — add a `### YYYY-MM-DD — <topic>` entry at
> the END of Session Log; never rewrite or delete older entries. Keep a short
> "resume here" pointer at the top when pausing mid-loop.

## Session Log

### {{.CreatedAt}} — ticket created

- Ticket bootstrapped; the problem statement lives in `context.md`.
- Per-session logs go to `sessions/YYYY-MM-DD-<slug>.md`, linked below.

## References

- Sessions:
