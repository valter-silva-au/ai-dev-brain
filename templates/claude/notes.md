# Notes: {{.Title}}

**Task ID:** {{.TaskID}}
**Created:** {{.CreatedAt}}

## Context

{{.Context}}

## Acceptance Criteria

{{range .AcceptanceCriteria}}
- [ ] {{.}}
{{else}}
- [ ] Define acceptance criteria
{{end}}

## Notes

{{.Notes}}

## References

{{.References}}
