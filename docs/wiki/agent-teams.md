# Agent Teams and Routing Patterns

This guide explains how to configure and use multi-agent teams in AI Dev Brain (adb), including team routing patterns, metadata storage, and common workflows.

---

## Overview

Agent teams allow Claude Code to coordinate multiple specialized AI agents for complex tasks. Instead of one agent doing everything, work is divided among experts:

- `@team-lead` orchestrates the team
- `@analyst` gathers requirements
- `@product-owner` writes PRDs
- `@backend-dev` implements code
- `@code-reviewer` reviews changes

Teams can be configured at the project level (`.taskconfig`) or per-task (`status.yaml`).

---

## Agent Definitions

Agents are defined in `.claude/agents/*.json` files.

### Example: Backend Developer Agent

```json
{
  "name": "backend-dev",
  "model": "opus",
  "description": "Backend development specialist for Go services",
  "isolation": "worktree",
  "permissions": {
    "read": ["internal/", "pkg/", "go.mod", "go.sum"],
    "write": ["internal/", "pkg/"],
    "execute": ["go", "git"]
  },
  "instructions": "You are a backend developer specializing in Go. Focus on clean architecture, error handling, and testability. Always write tests for new code."
}
```

**Key fields**:
- `name`: Agent identifier (used as `@backend-dev` in chat)
- `model`: Model to use (`opus`, `sonnet`, `haiku`)
- `description`: What the agent does (shown in team listings)
- `isolation`: Boundary enforcement (`worktree`, `namespace`, or omit for none)
- `permissions`: Which files the agent can read/write, which commands it can execute
- `instructions`: System prompt for the agent's role

---

### Universal Agents (Shipped with adb)

The following agents are installed globally via `adb sync-claude-user`:

| Agent | Model | Description | Memory |
|-------|-------|-------------|--------|
| `team-lead` | opus | Orchestrates multi-agent teams, breaks down work, monitors progress | No |
| `analyst` | sonnet | Requirements elicitation, PRD creation, research | Yes (project) |
| `product-owner` | sonnet | PRD facilitation, epic/story decomposition, backlog prioritization | Yes (project) |
| `design-reviewer` | sonnet | Architecture validation, checklist certification, alignment checks | Yes (project) |
| `scrum-master` | sonnet | Sprint planning, story preparation, retrospectives, course correction | Yes (project) |
| `quick-flow-dev` | sonnet | Rapid spec + implementation with built-in adversarial review | Yes (project) |
| `go-tester` | sonnet | Runs tests, analyzes failures, writes missing test cases | Yes (project) |
| `code-reviewer` | sonnet | Reviews Go code for quality, security, correctness | Yes (project) |
| `architecture-guide` | sonnet | Explains architecture, guides design decisions | Yes (project) |
| `knowledge-curator` | sonnet | Maintains wiki, ADRs, glossary; extracts learnings | Yes (project) |
| `researcher` | sonnet | Deep investigation for spikes, technology evaluations | Yes (user) |
| `debugger` | sonnet | Root cause analysis for errors, test failures, runtime issues | Yes (project) |

**Memory types**:
- **Project memory**: Agent remembers context across conversations within the same project
- **User memory**: Agent remembers context globally across all projects
- **No memory**: Stateless agent (fresh context each invocation)

---

## Team Routing Configuration

### Global Routing (`.taskconfig`)

Define default team routing rules for all tasks:

```yaml
# .taskconfig
team_routing:
  security_review:
    trigger: ["security", "auth", "crypto"]
    agents: ["security-auditor", "code-reviewer"]
    required: true

  design_review:
    trigger: ["architecture", "design", "refactor"]
    agents: ["architecture-guide", "design-reviewer"]
    required: false

  backend_workflow:
    trigger: ["api", "database", "backend"]
    agents: ["backend-dev", "go-tester"]
    required: true
```

**Fields**:
- `trigger`: Keywords that activate this routing rule (matched against task title, tags, or prompt)
- `agents`: List of agents to involve (in order)
- `required`: If `true`, task cannot proceed without these agents; if `false`, it's advisory

---

### Per-Task Routing (`status.yaml`)

Override global routing for a specific task:

```yaml
# tickets/TASK-00042/status.yaml
id: TASK-00042
title: Add JWT authentication
type: feat
status: in_progress
priority: P1
owner: alice
repo: github.com/acme/backend
branch: feat/add-jwt-auth
created: 2025-01-15T10:00:00Z
updated: 2025-01-16T14:30:00Z

# Team metadata (added by adb or manually)
teams:
  - security_review
  - backend_workflow

team_metadata:
  security_review:
    agents: ["security-auditor", "code-reviewer"]
    status: pending
    assigned_at: 2025-01-16T14:30:00Z

  backend_workflow:
    agents: ["backend-dev", "go-tester"]
    status: in_progress
    assigned_at: 2025-01-15T10:00:00Z
    completed_at: null
```

**Team statuses**:
- `pending`: Team assigned but not started
- `in_progress`: Team actively working
- `completed`: Team finished their part
- `blocked`: Team waiting on external dependency

---

## Team Routing Patterns

### Pattern 1: Security Review (Required)

**Use case**: All authentication and crypto changes must be reviewed by security team.

**Configuration**:
```yaml
team_routing:
  security_review:
    trigger: ["security", "auth", "crypto", "token", "session"]
    agents: ["security-auditor", "code-reviewer"]
    required: true
```

**Workflow**:
1. Developer creates task: `adb feat add-jwt-auth --tags security,backend`
2. Task metadata detects "security" tag
3. `security_review` team automatically assigned (status: `pending`)
4. Before task can be marked `done`, `security_review` must be `completed`
5. `adb archive TASK-00042` fails if `security_review` is not `completed`

---

### Pattern 2: Design Review (Advisory)

**Use case**: Architecture changes should be reviewed, but don't block progress.

**Configuration**:
```yaml
team_routing:
  design_review:
    trigger: ["architecture", "design", "refactor", "api"]
    agents: ["architecture-guide", "design-reviewer"]
    required: false
```

**Workflow**:
1. Developer creates task: `adb refactor extract-auth-middleware`
2. `design_review` team assigned (status: `pending`)
3. Task can proceed to `done` even if `design_review` is `pending`
4. Team lead can invoke review manually: `@team-lead review design.md`
5. Design reviewer provides feedback in task comments

---

### Pattern 3: BMAD Workflow (Sequential Phases)

**Use case**: Implement Business, Method, Analysis, Design (BMAD) workflow for complex features.

**Configuration**:
```yaml
team_routing:
  bmad_discovery:
    trigger: ["epic", "initiative", "new-feature"]
    agents: ["analyst", "product-owner"]
    required: true

  bmad_design:
    trigger: ["epic", "initiative", "new-feature"]
    agents: ["architecture-guide", "design-reviewer"]
    required: true
    depends_on: ["bmad_discovery"]

  bmad_implementation:
    trigger: ["epic", "initiative", "new-feature"]
    agents: ["backend-dev", "go-tester"]
    required: true
    depends_on: ["bmad_design"]
```

**Workflow**:
1. Task tagged with `epic`
2. All three teams assigned
3. `bmad_discovery` must complete before `bmad_design` can start
4. `bmad_design` must complete before `bmad_implementation` can start
5. Task cannot be marked `done` until `bmad_implementation` is `completed`

**Task metadata**:
```yaml
teams:
  - bmad_discovery
  - bmad_design
  - bmad_implementation

team_metadata:
  bmad_discovery:
    agents: ["analyst", "product-owner"]
    status: completed
    assigned_at: 2025-01-10T09:00:00Z
    completed_at: 2025-01-11T16:00:00Z

  bmad_design:
    agents: ["architecture-guide", "design-reviewer"]
    status: in_progress
    assigned_at: 2025-01-11T16:00:00Z

  bmad_implementation:
    agents: ["backend-dev", "go-tester"]
    status: pending
    assigned_at: 2025-01-11T16:00:00Z
```

---

### Pattern 4: Quick Flow (Single Agent)

**Use case**: Small tasks that don't need team coordination.

**Configuration**:
```yaml
team_routing:
  quick_flow:
    trigger: ["hotfix", "docs", "lint"]
    agents: ["quick-flow-dev"]
    required: false
```

**Workflow**:
1. Task tagged with `hotfix`
2. `quick_flow` team assigned (single agent)
3. `quick-flow-dev` handles spec, implementation, and self-review
4. Task can be completed without external review

---

## Team Coordination Commands

### Check Team Status

```bash
adb status TASK-00042 --show-teams
```

**Output**:
```
TASK-00042: Add JWT authentication
Status: in_progress
Teams:
  security_review: pending (assigned 2 hours ago)
  backend_workflow: in_progress (assigned 1 day ago)
```

---

### Assign a Team Manually

```bash
adb team assign TASK-00042 security_review
```

Adds `security_review` to task's `teams` list and initializes metadata.

---

### Complete a Team's Work

```bash
adb team complete TASK-00042 security_review
```

Sets `team_metadata.security_review.status = completed` and timestamps `completed_at`.

---

### List All Teams

```bash
adb team list
```

**Output**:
```
Available teams (from .taskconfig):
  security_review
  design_review
  bmad_discovery
  bmad_design
  bmad_implementation
  quick_flow
```

---

## Invoking Team Agents

### Via Team Lead

```
@team-lead implement JWT authentication using the security_review team
```

Team lead will:
1. Check task's `teams` list
2. Invoke `security-auditor` and `code-reviewer` in sequence
3. Aggregate results
4. Report back to user

---

### Direct Agent Invocation

```
@security-auditor review internal/auth/jwt.go for vulnerabilities
```

Directly invokes the agent without team coordination.

---

### Team Context Awareness

Agents in a team share task context via `ADB_TASK_ID`:

```
@backend-dev check if security_review team has approved my changes
```

Agent can read `tickets/TASK-00042/status.yaml` to see team metadata.

---

## Team Memory and Context

### Project Memory

Agents with **project memory** (e.g., `analyst`, `code-reviewer`) remember:
- Previous conversations in the same project
- Decisions made in prior tasks
- Code patterns and conventions

**How it works**:
- Claude Code stores conversation history per project in `~/.claude/projects/`
- When agent is invoked, it loads relevant history

---

### User Memory

Agents with **user memory** (e.g., `researcher`) remember:
- Context across all projects
- User preferences
- Common workflows

**How it works**:
- Claude Code stores user-level history globally
- Agent sees patterns across projects (e.g., "user prefers TDD")

---

### Stateless Agents

Agents without memory (e.g., `team-lead`) start fresh each invocation:
- No conversation history loaded
- Faster response time
- Good for orchestration tasks

---

## Agent Isolation in Teams

### Worktree Isolation

Agents with `"isolation": "worktree"` can only edit files inside `ADB_WORKTREE_PATH`:

```json
{
  "name": "backend-dev",
  "isolation": "worktree"
}
```

**Enforced by**: PreToolUse hook (see `docs/wiki/worktree-isolation.md`)

---

### Namespace Isolation

Agents with `"isolation": "namespace"` are restricted to a directory:

```json
{
  "name": "frontend-dev",
  "isolation": "namespace",
  "namespace": "frontend/"
}
```

Agent can only edit files under `frontend/` within the worktree.

---

### No Isolation

Agents without `isolation` field can edit any file:

```json
{
  "name": "team-lead"
  // No isolation field
}
```

**Use cases**:
- Orchestration agents (don't edit files)
- Cross-cutting concerns (docs, CI config)

---

## Team Performance Considerations

### Memory Leak Issues (Claude Code < v2.1.50)

Multi-agent teams accumulate context quickly. In older versions:
- Agent team coordination retained full context indefinitely
- Memory usage >8GB after 2-3 hours
- Responses slowed down

**Fix**: Upgrade to Claude Code v2.1.50+ for automatic memory leak fixes.

---

### Team Depth Limit

**Problem**: Deeply nested teams slow down:
```
@team-lead -> @scrum-master -> @analyst -> @researcher
```

**Recommendation**: Limit team depth to 2 levels:
```
@team-lead -> @analyst
@team-lead -> @product-owner
```

---

### Fast Mode for Teams

Enable fast mode for faster team coordination:

```
/fast @team-lead plan this sprint with the backend_workflow team
```

Speeds up agent responses by 2-3x without sacrificing quality.

---

## Troubleshooting Team Issues

### Symptom: Agent Not Responding

**Checklist**:
1. **Verify agent exists**:
   ```bash
   ls -la ~/.claude/agents/backend-dev.json
   ```
2. **Check agent registration**:
   ```bash
   cat ~/.claude/settings.json | grep backend-dev
   ```
3. **Memory leak** (Claude Code < v2.1.50):
   - Restart Claude Code if session > 2 hours
   - Check memory usage: `ps aux | grep claude`

---

### Symptom: Team Metadata Not Updated

**Issue**: `adb team complete TASK-00042 security_review` does nothing.

**Cause**: Task's `status.yaml` does not have `teams` or `team_metadata` fields.

**Fix**:
```bash
# Manually add teams to status.yaml
cat >> tickets/TASK-00042/status.yaml <<EOF
teams:
  - security_review
team_metadata:
  security_review:
    agents: ["security-auditor", "code-reviewer"]
    status: pending
    assigned_at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
```

---

### Symptom: Agent Violates Isolation

**Issue**: Agent with `"isolation": "worktree"` edits files outside worktree.

**Cause**: PreToolUse hook not installed or `ADB_WORKTREE_PATH` not set.

**Fix**:
```bash
# Reinstall hook
adb resume TASK-XXXXX

# Verify environment
echo $ADB_WORKTREE_PATH
```

---

## Advanced: Custom Team Workflows

### Example: Security Gating

Block task completion until security review is done:

**Pre-archive hook** (`.claude/hooks/pre-archive.sh`):
```bash
#!/bin/bash
task_id="$1"

# Load task metadata
status_file="tickets/$task_id/status.yaml"
security_status=$(yq '.team_metadata.security_review.status' "$status_file")

if [[ "$security_status" != "completed" ]]; then
  echo "Error: Cannot archive task without completed security review" >&2
  exit 1
fi

exit 0
```

Register in `.taskconfig`:
```yaml
hooks:
  pre_archive: .claude/hooks/pre-archive.sh
```

---

### Example: Automated Team Assignment

Trigger team assignment on task creation:

**Post-create hook** (`.claude/hooks/post-create.sh`):
```bash
#!/bin/bash
task_id="$1"
task_file="tickets/$task_id/status.yaml"

# Check tags
tags=$(yq '.tags[]' "$task_file")

if echo "$tags" | grep -q "security"; then
  adb team assign "$task_id" security_review
fi

if echo "$tags" | grep -q "api"; then
  adb team assign "$task_id" design_review
fi
```

Register in `.taskconfig`:
```yaml
hooks:
  post_create: .claude/hooks/post-create.sh
```

---

## Team Routing Best Practices

1. **Use `required: true` for critical reviews** (security, compliance)
2. **Use `required: false` for advisory reviews** (design, performance)
3. **Limit team depth to 2 levels** for performance
4. **Enable fast mode** for team coordination (`/fast`)
5. **Monitor team memory usage** (upgrade to v2.1.50+ to avoid leaks)
6. **Document team routing rules** in project README
7. **Use agent isolation** to prevent cross-boundary edits
8. **Test team workflows** before using in production
9. **Track team metadata** in `status.yaml` for visibility
10. **Audit team completions** before archiving tasks

---

## Example Workflows

### Workflow 1: Feature Development with Security Review

1. Create task:
   ```bash
   adb feat add-oauth --tags security,backend
   ```

2. Task metadata assigns `security_review` and `backend_workflow` teams

3. Developer works with backend team:
   ```
   @backend-dev implement OAuth 2.0 flow in internal/auth/oauth.go
   @go-tester write tests for OAuth flow
   ```

4. Request security review:
   ```
   @team-lead request security_review team to audit OAuth implementation
   ```

5. Security team reviews:
   ```
   @security-auditor check for timing attacks in token validation
   @code-reviewer ensure secrets are not logged
   ```

6. Mark security review complete:
   ```bash
   adb team complete TASK-00042 security_review
   ```

7. Archive task (allowed now that security review is complete):
   ```bash
   adb archive TASK-00042
   ```

---

### Workflow 2: BMAD Epic

1. Create epic task:
   ```bash
   adb feat new-payment-system --tags epic,payment
   ```

2. Discovery phase:
   ```
   @analyst research payment gateway options and pricing
   @product-owner draft PRD for payment system
   ```

3. Complete discovery:
   ```bash
   adb team complete TASK-00050 bmad_discovery
   ```

4. Design phase:
   ```
   @architecture-guide design payment service architecture
   @design-reviewer validate against existing patterns
   ```

5. Complete design:
   ```bash
   adb team complete TASK-00050 bmad_design
   ```

6. Implementation phase:
   ```
   @backend-dev implement payment service
   @go-tester write integration tests
   ```

7. Complete implementation:
   ```bash
   adb team complete TASK-00050 bmad_implementation
   ```

8. Archive task:
   ```bash
   adb archive TASK-00050
   ```

---

## See Also

- **docs/wiki/worktree-isolation.md**: Agent isolation enforcement
- **docs/wiki/worktree-automation.md**: Lifecycle hooks and automation
- **docs/runbooks/troubleshooting.md**: Debugging agent team issues
- **CLAUDE.md**: Agent definitions and command reference
