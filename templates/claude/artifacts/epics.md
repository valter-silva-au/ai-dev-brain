# Epics & Stories: {{.Title}}

**Task:** {{.TaskID}}
**Date:** {{.Date}}
**Author:** product-owner / scrum-master
**Status:** Draft

---

## Epic 1: [Epic Title]

**Description:** _What user-facing capability does this epic deliver?_
**PRD Requirements:** FR-001, FR-002
**Architecture Components:** [Component A], [Component B]

### Story 1.1: [Story Title]

**As a** [persona],
**I want to** [action],
**So that** [benefit].

**Priority:** Must-Have / Should-Have / Nice-to-Have

**Acceptance Criteria:**

```gherkin
Given [initial context]
When [action is performed]
Then [expected outcome]

Given [alternative context]
When [action is performed]
Then [alternative outcome]

Given [error context]
When [action is performed]
Then [error is handled gracefully]
```

**Technical Notes:** _Implementation hints from the architecture doc._
**Dependencies:** _Other stories this depends on._
**Estimated Size:** S / M / L

---

### Story 1.2: [Story Title]

**As a** [persona],
**I want to** [action],
**So that** [benefit].

**Priority:**

**Acceptance Criteria:**

```gherkin
Given [context]
When [action]
Then [result]
```

**Dependencies:**
**Estimated Size:**

---

## Epic 2: [Epic Title]

**Description:**
**PRD Requirements:**
**Architecture Components:**

### Story 2.1: [Story Title]

**As a** [persona],
**I want to** [action],
**So that** [benefit].

**Priority:**

**Acceptance Criteria:**

```gherkin
Given [context]
When [action]
Then [result]
```

**Dependencies:**
**Estimated Size:**

---

## Story Dependency Map

_Identify implementation order based on dependencies._

```
Story 1.1 (no deps)
  └── Story 1.2 (depends on 1.1)
Story 2.1 (no deps)
  └── Story 2.2 (depends on 2.1, 1.1)
```

## Requirement Traceability

| PRD Requirement | Epic | Stories |
|----------------|------|---------|
| FR-001 | Epic 1 | 1.1, 1.2 |
| FR-002 | Epic 1 | 1.3 |
| FR-003 | Epic 2 | 2.1 |

---

_These epics and stories were generated during the Stories phase. Next step: run the readiness checklist, then begin implementation._
