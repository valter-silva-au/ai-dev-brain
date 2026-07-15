# Design Document: {{.Title}}

**Task ID:** {{.TaskID}}
**Created:** {{.CreatedAt}}

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
