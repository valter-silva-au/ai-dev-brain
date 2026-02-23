# Architecture Document: {{.Title}}

**Task:** {{.TaskID}}
**Date:** {{.Date}}
**Author:** design-reviewer
**Status:** Draft

---

## 1. Overview

### 1.1 Purpose

_What does this architecture cover? Reference the PRD._

### 1.2 System Context

_Where does this system/feature sit in the broader landscape? What are the external actors and systems it interacts with?_

```
[External Actor] --> [This System] --> [External System]
```

---

## 2. Key Decisions

### ADR-001: [Decision Title]

**Status:** Proposed / Accepted / Deprecated
**Context:** _What is the situation that requires a decision?_
**Decision:** _What did we decide?_
**Alternatives Considered:**
1. _Alternative A_ — Pros: ... / Cons: ...
2. _Alternative B_ — Pros: ... / Cons: ...

**Rationale:** _Why this option?_
**Consequences:** _What are the implications?_

### ADR-002: [Decision Title]

**Status:**
**Context:**
**Decision:**
**Rationale:**

---

## 3. Component Design

### 3.1 Component Overview

_List the major components and their responsibilities._

| Component | Responsibility | Package/Module |
|-----------|---------------|----------------|
| | | |

### 3.2 Component Interactions

_How do components communicate? Describe the data flow._

```
[Component A] --request--> [Component B] --query--> [Database]
```

### 3.3 Component Details

#### [Component Name]

**Responsibility:** _Single-sentence description._
**Interface:**
```
// Key methods/endpoints this component exposes
```
**Dependencies:** _What does this component depend on?_

---

## 4. Data Model

### 4.1 Entities

| Entity | Description | Key Fields |
|--------|-------------|------------|
| | | |

### 4.2 Relationships

_Describe entity relationships and cardinality._

### 4.3 Storage

| Data | Store | Rationale |
|------|-------|-----------|
| | | |

---

## 5. API Design

### 5.1 Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| | | | |

### 5.2 Request/Response Formats

_Key payload structures._

---

## 6. Security

### 6.1 Authentication

_How are users/systems authenticated?_

### 6.2 Authorization

_How is access controlled? Roles, permissions, policies._

### 6.3 Data Protection

_Encryption at rest, in transit, sensitive data handling._

---

## 7. Deployment & Operations

### 7.1 Deployment Strategy

_How is this deployed? Zero-downtime strategy, rollback plan._

### 7.2 Monitoring

| What | How | Alert Threshold |
|------|-----|-----------------|
| | | |

### 7.3 Failure Modes

| Failure | Impact | Recovery |
|---------|--------|----------|
| | | |

---

## 8. Testing Strategy

| Layer | Scope | Tools |
|-------|-------|-------|
| Unit | Individual functions/methods | |
| Integration | Component interactions | |
| E2E | User-facing flows | |

---

## 9. Open Questions

| # | Question | Impact | Owner |
|---|----------|--------|-------|
| 1 | | | |

---

_This architecture document was generated during the Architecture phase. Next step: decompose into epics and stories._
