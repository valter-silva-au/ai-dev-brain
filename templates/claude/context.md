# Context: {{.Title}}

**Task ID:** {{.TaskID}}
**Status:** {{.Status}}
**Created:** {{.CreatedAt}}

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
