# Handoff: {{.Title}}

**Task ID:** {{.TaskID}}
**Completed:** {{.CompletedAt}}

## Summary

{{.Summary}}

## What Was Done

{{range .CompletedItems}}
- {{.}}
{{else}}
- No items completed
{{end}}

## Key Decisions

{{range .Decisions}}
### {{.Title}}

{{.Description}}

**Rationale:** {{.Rationale}}

{{end}}

## Open Items

{{range .OpenItems}}
- [ ] {{.}}
{{else}}
- No open items
{{end}}

## Next Steps

{{.NextSteps}}

## References

{{.References}}
