# Architecture Document: {{.Title}}

**Author:** {{.Owner}}
**Date:** {{.Date}}
**Task:** {{.TaskID}}
**Status:** Draft

## Overview

High-level description of the architectural approach and its rationale.

### Context

What existing systems, constraints, and requirements inform this architecture.

### Scope

What this architecture covers and what it explicitly excludes.

## System Architecture

### Component Diagram

```
[Component A] --> [Component B] --> [Component C]
```

### Component Responsibilities

| Component | Responsibility | Package |
|-----------|---------------|---------|
|           |               |         |

### Interface Contracts

#### [Interface Name]

```go
type InterfaceName interface {
    Method(param Type) (ReturnType, error)
}
```

**Contract**: Description of the behavioral contract.

## Data Models

### [Model Name]

```go
type ModelName struct {
    Field Type `yaml:"field"`
}
```

**Persistence**: Where and how this data is stored.

## API Design

### [Endpoint/Command Name]

- **Input**: Parameters and their types
- **Output**: Return values and their types
- **Errors**: Error conditions and handling
- **Example**: Usage example

## Technical Decisions

| Decision | Options Considered | Choice | Rationale |
|----------|-------------------|--------|-----------|
|          |                   |        |           |

## Patterns and Conventions

### Followed Patterns

- **[Pattern Name]**: How it's applied and why

### Deviations from Standard Patterns

- **[Deviation]**: Why the standard pattern doesn't apply here

## Error Handling Strategy

How errors are propagated, wrapped, and surfaced to users.

## Security Considerations

- Input validation boundaries
- Authentication/authorization approach
- Secret management
- Attack surface analysis

## Performance Considerations

- Expected load characteristics
- Bottleneck analysis
- Caching strategy (if applicable)
- Resource usage estimates

## Testing Strategy

| Test Type | Scope | Tools |
|-----------|-------|-------|
| Unit | Individual functions | `testing.T`, table-driven |
| Property | Invariants | `pgregory.net/rapid` |
| Integration | Cross-package | `internal/integration_test.go` |

## Migration and Compatibility

### Backward Compatibility

How existing functionality is preserved.

### Migration Steps

1. Step-by-step migration plan if applicable

## Dependencies

| Dependency | Version | Purpose | Risk |
|-----------|---------|---------|------|
|           |         |         |      |

## Open Questions

| Question | Impact | Status |
|----------|--------|--------|
|          |        | Open   |

## Revision History

| Date | Author | Changes |
|------|--------|---------|
| {{.Date}} | {{.Owner}} | Initial draft |
