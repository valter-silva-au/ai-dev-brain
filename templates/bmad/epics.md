# {{.Title}} - Epic Breakdown

**Task:** {{.TaskID}}
**Date:** {{.Date}}
**Author:** {{.Owner}}

## Overview

This document provides the complete epic and story breakdown, decomposing the requirements from the PRD and architecture document into implementable stories.

## Requirements Inventory

### Functional Requirements

| ID | Requirement | Priority | Covered By |
|----|-------------|----------|-----------|
| FR-001 | | | Epic N, Story N.M |

### Non-Functional Requirements

| ID | Requirement | Covered By |
|----|-------------|-----------|
| NFR-001 | | Epic N, Story N.M |

### Requirements Coverage Map

| Requirement | Epic | Story | Status |
|------------|------|-------|--------|
| FR-001 | Epic 1 | 1.1 | Covered |

## Epic List

| # | Epic | Stories | Priority |
|---|------|---------|----------|
| 1 | [Epic Title] | N stories | P1 |

---

## Epic 1: [Epic Title]

**Goal:** What this epic delivers when all stories are complete.

**Dependencies:** Any prerequisite epics or external dependencies.

### Story 1.1: [Story Title]

As a [user type],
I want [capability],
So that [value/benefit].

**Acceptance Criteria:**

**Given** [precondition]
**When** [action]
**Then** [expected outcome]

**Technical Notes:**
- Files to modify:
- Patterns to follow:
- Test requirements:

---

### Story 1.2: [Story Title]

As a [user type],
I want [capability],
So that [value/benefit].

**Acceptance Criteria:**

**Given** [precondition]
**When** [action]
**Then** [expected outcome]

---

## Implementation Sequence

Recommended story implementation order based on dependencies:

| Order | Story | Dependencies | Risk |
|-------|-------|-------------|------|
| 1 | 1.1 | None | Low |
| 2 | 1.2 | 1.1 | Medium |

## Definition of Done

- [ ] All acceptance criteria pass
- [ ] Unit tests written and passing
- [ ] Property tests for invariants (where applicable)
- [ ] No regressions in existing tests
- [ ] Code follows project patterns (error wrapping, local interfaces, constructors return interfaces)
- [ ] `go vet` and `golangci-lint` pass
