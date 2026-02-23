# Roadmap: BMAD Method Integration into AI Dev Brain

This document describes the phased plan for integrating concepts from the
[BMAD Method](https://github.com/bmad-code-org/BMAD-METHOD) (Breakthrough
Method of Agile AI Driven Development) into AI Dev Brain (`adb`). It maps
BMAD's agent personas, phase-gated workflows, quality checklists, and
artifact templates to concrete enhancements in adb's architecture.

---

## Executive Summary

BMAD brings a structured, discipline-first approach to AI-assisted
development. Its core insight is that AI coding assistants produce better
results when they follow a phased workflow with quality gates, specialized
agent personas, and certified artifact handoffs — rather than jumping
straight from a vague request to code.

adb already has strong foundations in task lifecycle management, persistent
context, knowledge accumulation, and observability. What it lacks is the
**pre-implementation discipline** that BMAD enforces: structured requirements
gathering, architecture validation, story decomposition with acceptance
criteria, and artifact certification before code is written.

This integration does not replace adb's existing architecture. It extends
it by adding optional workflow phases, new agent personas, quality gate
checklists, and richer artifact templates — all following adb's established
patterns of file-based storage, interface-driven design, and CLI commands.

---

## What BMAD Brings to adb

### Current adb Strengths (Retained)

| Capability | adb Status |
|-----------|------------|
| Task lifecycle (backlog → archive) | Strong |
| Persistent context across sessions | Strong |
| Knowledge accumulation and extraction | Strong |
| Observability (events, metrics, alerts) | Strong |
| Multi-repo worktree management | Strong |
| Session summaries and continuity | Strong |
| Feedback loop and channel system | Emerging |

### Gaps BMAD Addresses

| Gap | BMAD Solution | Impact |
|-----|---------------|--------|
| No requirements gathering phase | Analyst persona + PRD workflow + product brief template | Prevents "code first, think later" failures |
| No architecture validation gate | Architect persona + architecture workflow + validation checklist | Catches design issues before implementation |
| No story decomposition | PM/Scrum Master personas + epic/story workflow + INVEST-criteria templates | Creates atomic, testable work units |
| No artifact certification | QA checklists for PRD, architecture, stories | Quality gates between phases |
| No implementation readiness check | Cross-artifact validation workflow | Ensures all planning artifacts align |
| No course correction mechanism | Correct-course workflow with impact analysis | Structured response when plans derail |
| No quick-flow for small tasks | Quick-spec + quick-dev workflows | Right-sized ceremony for task complexity |
| Code-only quality gates | Artifact-level checklists + adversarial review | Quality enforcement before and after code |
| Advisory-only architecture agent | Architect agent that produces certified artifacts | Binding design documents, not suggestions |
| No orchestrator routing | Intent-based routing to appropriate persona/workflow | User doesn't need to know which agent to invoke |

---

## Phase 1: Foundation — Workflow Phases and Agent Personas

**Goal**: Extend adb's task lifecycle with optional workflow phases and add
BMAD-inspired agent personas.

### 1.1 Task Workflow Phases

Add optional phase tracking to tasks. Phases represent the BMAD lifecycle
stages that a task can progress through before implementation. Phases are
advisory (not enforced) by default, with opt-in enforcement via
configuration.

```
discovery → requirements → architecture → stories → implementation → review → done
```

**Implementation**:
- Add `phase` field to `pkg/models/task.go` (`Task` struct)
- Add `phase` to `status.yaml` persistence in `internal/storage/`
- Add `adb phase <task-id> <phase>` CLI command
- Phase transitions are logged as observability events (`task.phase_changed`)
- Phases are independent of status — a task can be `in_progress` and in
  the `architecture` phase simultaneously

**New task statuses are NOT needed.** Phases overlay the existing status
system. A task in `in_progress` status with phase `requirements` means
"actively working on requirements gathering."

### 1.2 New Agent Personas

Add the following agents to `.claude/agents/`, following the existing
agent definition pattern:

| Agent | BMAD Equivalent | Role | Model |
|-------|-----------------|------|-------|
| `analyst` | Mary (Analyst) | Requirements elicitation, PRD creation, market/domain/technical research | sonnet |
| `product-owner` | John (PM) | PRD facilitation, epic/story creation, backlog prioritization, implementation readiness | sonnet |
| `design-reviewer` | Winston (Architect) + QA | Architecture validation, checklist certification, implementation readiness checks | sonnet |
| `scrum-master` | Bob (SM) | Sprint planning, story preparation, retrospectives, course correction | sonnet |
| `quick-flow-dev` | Barry (Quick Flow) | Rapid spec + implementation for small tasks with built-in adversarial review | sonnet |

**Existing agents are unchanged.** The new personas complement rather than
replace them:
- `architecture-guide` remains for advisory architecture questions
- `code-reviewer` remains for post-implementation code review
- `go-tester` remains for test execution
- `team-lead` gains awareness of new personas for orchestration

### 1.3 Orchestrator Enhancement

Enhance the `team-lead` agent to act as an orchestrator that routes user
intent to the appropriate persona and workflow, following BMAD's
orchestrator pattern:

- "I need to gather requirements" → routes to `analyst`
- "Let's create a PRD" → routes to `product-owner`
- "Review the architecture" → routes to `design-reviewer`
- "Plan the sprint" → routes to `scrum-master`
- "Quick fix for this bug" → routes to `quick-flow-dev`
- "Write the code" → routes to `developer` (existing dev workflow)

---

## Phase 2: Templates and Checklists

**Goal**: Add BMAD-style artifact templates and quality gate checklists
to adb's template system.

### 2.1 New Artifact Templates

Add templates that are generated in the task's ticket directory when the
corresponding phase begins:

| Template | Created When | Location | Purpose |
|----------|-------------|----------|---------|
| `product-brief.md` | Phase: discovery | `tickets/TASK-XXXXX/` | High-level product/feature vision |
| `prd.md` | Phase: requirements | `tickets/TASK-XXXXX/` | Product Requirements Document with FR/NFR, user personas, success metrics |
| `architecture-doc.md` | Phase: architecture | `tickets/TASK-XXXXX/` | Technical architecture with decisions, patterns, data models, API specs |
| `epics.md` | Phase: stories | `tickets/TASK-XXXXX/` | Epic breakdown with stories following INVEST criteria and Given/When/Then acceptance criteria |
| `tech-spec.md` | Quick flow | `tickets/TASK-XXXXX/` | Lightweight implementation spec for small tasks |

**Implementation**:
- Add template content to `internal/core/templates.go`
- Extend `TemplateManager` interface with `ApplyPhaseTemplate(ticketPath, phase)`
- Templates use Go `text/template` with task metadata placeholders
- Templates are opt-in: only generated when a phase is entered

### 2.2 Quality Gate Checklists

Add checklist files that agents use to validate artifacts before phase
transitions:

| Checklist | Validates | Gate Between |
|-----------|-----------|--------------|
| `prd-checklist.md` | PRD completeness, measurability, traceability | requirements → architecture |
| `architecture-checklist.md` | Architecture soundness, scalability, security, alignment with PRD | architecture → stories |
| `story-checklist.md` | INVEST criteria, acceptance criteria clarity, requirement coverage | stories → implementation |
| `readiness-checklist.md` | Cross-artifact alignment (PRD + architecture + stories) | stories → implementation |
| `code-review-checklist.md` | Implementation quality, test coverage, AC satisfaction | implementation → review |

**Implementation**:
- Store checklists as markdown files in `templates/checklists/` within the
  adb source tree
- Add `adb checklist run <checklist> <task-id>` CLI command
- Checklist results are stored in `tickets/TASK-XXXXX/checklists/`
- Results include pass/fail per item, timestamp, and certifying agent
- Phase transitions can optionally require checklist certification
  (configured in `.taskconfig`)

### 2.3 Checklist Configuration

```yaml
# .taskconfig addition
workflow:
  phases_enabled: true
  enforce_gates: false  # true = block phase transitions without checklist pass
  checklists:
    requirements_to_architecture: prd-checklist
    architecture_to_stories: architecture-checklist
    stories_to_implementation: readiness-checklist
    implementation_to_review: code-review-checklist
```

---

## Phase 3: Workflow Commands and Skills

**Goal**: Add CLI commands and Claude Code skills that implement BMAD's
structured workflows.

### 3.1 New CLI Commands

| Command | Description |
|---------|-------------|
| `adb phase <task-id> <phase>` | Set task workflow phase |
| `adb checklist run <name> <task-id>` | Run a quality gate checklist against a task |
| `adb checklist list` | List available checklists |
| `adb correct-course <task-id>` | Initiate course correction workflow (BMAD's change management) |

### 3.2 New Claude Code Skills

| Skill | Description | BMAD Equivalent |
|-------|-------------|-----------------|
| `create-brief` | Guided product brief creation with the analyst agent | Create Product Brief workflow |
| `create-prd` | Facilitated PRD creation with the product-owner agent | Create PRD workflow |
| `create-architecture` | Guided architecture doc creation with design-reviewer | Create Architecture workflow |
| `create-stories` | Epic/story decomposition from PRD and architecture | Create Epics and Stories workflow |
| `check-readiness` | Cross-artifact validation before implementation | Implementation Readiness workflow |
| `correct-course` | Structured change management when plans derail | Correct Course workflow |
| `quick-spec` | Rapid tech spec for small tasks | Quick-Spec workflow |
| `quick-dev` | Spec + implement + adversarial review for small tasks | Quick-Dev workflow |
| `run-checklist` | Run a named quality gate checklist | QA validation |
| `adversarial-review` | Self-review with adversarial findings | Adversarial Code Review task |

### 3.3 Phase-Aware Context Generation

Extend `AIContextGenerator` to include phase information in generated
context files:

- Current phase of each active task
- Checklist certification status
- Links to phase artifacts (PRD, architecture doc, stories)

This ensures AI assistants entering a session know what phase the task is
in and what artifacts exist.

---

## Phase 4: Quick Flow Integration

**Goal**: Implement BMAD's "Quick Flow" pattern for small tasks that don't
warrant full ceremony.

### 4.1 Scale-Adaptive Workflow

BMAD's key insight is that not every task needs a PRD and architecture doc.
The Quick Flow pattern provides right-sized ceremony:

| Task Complexity | Workflow | Artifacts |
|----------------|----------|-----------|
| Trivial (typo, config) | Direct implementation | None |
| Small (bug fix, small feature) | Quick-spec → quick-dev | tech-spec.md |
| Medium (feature, refactor) | Requirements → architecture → stories | PRD, architecture doc, epics |
| Large (new system, major feature) | Full BMAD lifecycle | All artifacts |

**Implementation**:
- Add `complexity` field to task metadata (auto-detected or user-specified)
- `adb feat` with `--quick` flag skips to Quick Flow
- `adb spike` naturally maps to the discovery/research phase
- Task type informs default complexity:
  - `bug` → small (Quick Flow)
  - `feat` → medium (configurable)
  - `spike` → discovery only
  - `refactor` → medium (Quick Flow or full)

### 4.2 Adversarial Review

Adopt BMAD's adversarial review pattern as a built-in self-check:

- After implementation, construct a diff from the baseline commit
- Invoke a review agent with only the diff (no implementation context)
- Present findings with severity ratings
- Require resolution of critical/high findings before marking done

This can be integrated as:
- A new skill: `/adversarial-review`
- A hook on `TaskCompleted` that triggers adversarial review
- An optional gate before `review` status

---

## Phase 5: Advanced Integration

**Goal**: Deeper structural integration following BMAD's proven patterns.

### 5.1 Party Mode (Multi-Agent Collaboration)

BMAD's "Party Mode" brings multiple personas into a single session for
collaborative discussion. Map this to adb's existing team system:

- `adb team create --party analyst,architect,pm` creates a temporary
  multi-agent session
- Agents discuss and debate rather than simply executing
- Useful for architectural decisions, trade-off analysis, and planning

### 5.2 Research Workflows

Add structured research workflows inspired by BMAD's analysis phase:

| Workflow | Steps | Output |
|----------|-------|--------|
| Market research | Customer behavior → pain points → decisions → competitive analysis | Research report |
| Domain research | Domain analysis → competitive landscape → regulatory → technical trends | Domain brief |
| Technical research | Technical overview → integration patterns → architectural patterns → synthesis | Technical assessment |

These extend adb's existing `spike` task type with structured step-by-step
investigation.

### 5.3 Sprint Planning and Retrospectives

Add agile ceremony support:

- `adb sprint plan <task-id>` — sequence stories for implementation
- `adb sprint status` — show progress against planned stories
- `adb retro <task-id>` — structured retrospective with multi-agent review

### 5.4 Document Project

Adopt BMAD's `document-project` workflow that analyzes an existing codebase
to produce structured documentation:

- Project overview with architecture diagrams
- Source tree analysis
- Deep-dive documentation per component
- Scan report with findings

This complements adb's existing `sync-context` command with richer
documentation generation.

---

## Implementation Priority

| Priority | Phase | Rationale |
|----------|-------|-----------|
| P1 | Phase 1.2: Agent personas | Immediate value with minimal architecture changes |
| P1 | Phase 2.1: Artifact templates | Templates are standalone files, no code changes needed |
| P1 | Phase 3.2: Skills (quick-spec, quick-dev, adversarial-review) | High-impact skills using existing Claude Code skill system |
| P2 | Phase 1.1: Workflow phases | Requires model and storage changes |
| P2 | Phase 2.2: Quality gate checklists | Requires new CLI command and storage |
| P2 | Phase 3.1: CLI commands (phase, checklist) | Depends on Phase 1.1 and 2.2 |
| P2 | Phase 4.1: Scale-adaptive workflow | Depends on phase system |
| P2 | Phase 4.2: Adversarial review | Can be a standalone skill first |
| P3 | Phase 1.3: Orchestrator enhancement | Builds on all other phases |
| P3 | Phase 3.3: Phase-aware context generation | Depends on phase system |
| P3 | Phase 5.x: Advanced integration | Long-term enhancements |

---

## Architecture Impact

### Model Changes (`pkg/models/task.go`)

```go
type Task struct {
    // ... existing fields ...
    Phase       TaskPhase `yaml:"phase,omitempty"`
    Complexity  string    `yaml:"complexity,omitempty"`
}

type TaskPhase string

const (
    PhaseDiscovery     TaskPhase = "discovery"
    PhaseRequirements  TaskPhase = "requirements"
    PhaseArchitecture  TaskPhase = "architecture"
    PhaseStories       TaskPhase = "stories"
    PhaseImplementation TaskPhase = "implementation"
    PhaseReview        TaskPhase = "review"
    PhaseDone          TaskPhase = "done"
)
```

### New Interfaces

```go
// internal/core/checklist.go
type ChecklistRunner interface {
    Run(taskID string, checklistName string) (ChecklistResult, error)
    ListChecklists() ([]ChecklistInfo, error)
}

// internal/core/phase.go
type PhaseManager interface {
    SetPhase(taskID string, phase TaskPhase) error
    GetPhase(taskID string) (TaskPhase, error)
    ValidateTransition(from, to TaskPhase) error
}
```

### Storage Changes

New directory in each task ticket:

```
tickets/TASK-XXXXX/
  checklists/           # Checklist run results
    2025-01-15-prd-checklist.yaml
  product-brief.md      # Phase: discovery (optional)
  prd.md                # Phase: requirements (optional)
  architecture-doc.md   # Phase: architecture (optional)
  epics.md              # Phase: stories (optional)
  tech-spec.md          # Quick flow (optional)
```

### Observability Events

| Event Type | Trigger |
|------------|---------|
| `task.phase_changed` | Phase transition |
| `checklist.run` | Checklist execution |
| `checklist.certified` | All items passed |
| `checklist.failed` | One or more items failed |

---

## Compatibility

- **Backward compatible**: All changes are additive. Existing tasks without
  phases continue to work as before.
- **Opt-in ceremony**: Phases and checklists are optional by default. Teams
  choose their level of process.
- **Gradual adoption**: Each phase of this roadmap can be implemented
  independently. Phase 1 (agents + templates) provides value without any
  Go code changes.
- **Configuration-driven**: Enforcement levels are controlled via
  `.taskconfig`, following adb's existing configuration patterns.

---

## Key Principles from BMAD Applied to adb

1. **Think before code** — Phases ensure requirements and design happen
   before implementation.
2. **Persona separation** — Each agent has clear boundaries and expertise.
   The analyst doesn't architect. The developer doesn't design.
3. **Artifact-driven handoffs** — Each phase produces a document that
   serves as the contract for the next phase.
4. **Quality gates** — Checklists catch issues at phase boundaries rather
   than after implementation.
5. **Scale-adaptive ceremony** — Quick Flow for small tasks, full lifecycle
   for large ones. The right amount of process for the task at hand.
6. **Course correction** — A structured response when plans need to change,
   not ad-hoc improvisation.
7. **Adversarial review** — Self-check with information asymmetry catches
   issues the implementer is blind to.

---

*This roadmap was produced by analyzing the BMAD Method v6 repository
against adb's current architecture. See ADR records in `docs/decisions/`
for individual integration decisions as they are implemented.*
