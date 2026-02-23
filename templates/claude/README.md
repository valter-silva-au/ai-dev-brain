# Claude Code Templates

These templates are the canonical source of truth for Claude Code configuration
managed by `adb`. All templates are embedded into the `adb` binary at compile time
via `embed.go`, making the tool self-contained with no external dependencies.

Templates are used by two commands:

## `adb init-claude`

Bootstraps a `.claude/` directory for a new repository:

- Copies `claudeignore.template` → `.claudeignore`
- Copies `settings.template.json` → `.claude/settings.json`
- Creates `.claude/rules/workspace.md` from project analysis

## `adb sync-claude-user`

Syncs all agents, skills, checklists, artifact templates, and hooks to `~/.claude/`:

- Copies `agents/*.md` → `~/.claude/agents/*.md` (18 agents)
- Copies `skills/*/SKILL.md` → `~/.claude/skills/*/SKILL.md` (36 skills)
- Copies `checklists/*.md` → `~/.claude/checklists/*.md` (5 quality gate checklists)
- Copies `artifacts/*.md` → `~/.claude/artifacts/*.md` (5 artifact templates)
- Installs `hooks/adb-session-capture.sh` and SessionEnd hook entry

With `--mcp` flag, also merges MCP servers into `~/.claude.json`:

- Reads `mcp-servers.json` and merges each server into the user config
- Existing servers are updated, new ones added, unrelated keys preserved

## New machine setup

```bash
adb sync-claude-user --mcp
export CONTEXT7_API_KEY="your-key-here"  # add to shell profile
```

## Directory Structure

```
templates/claude/
├── README.md                       # This file
├── embed.go                        # Embeds all templates into the binary
├── claudeignore.template           # Default .claudeignore
├── settings.template.json          # Default .claude/settings.json
├── mcp-servers.json                # MCP servers for ~/.claude.json
│
├── agents/                         # BMAD agent personas + supporting agents
│   ├── analyst.md                  # Requirements elicitation, PRD creation
│   ├── architecture-guide.md       # Architecture guidance and patterns
│   ├── browser-qa.md               # Browser-based QA testing
│   ├── code-reviewer.md            # Code quality review
│   ├── debugger.md                 # Root cause analysis
│   ├── design-reviewer.md          # Architecture validation, checklists
│   ├── doc-writer.md               # Documentation generation
│   ├── go-tester.md                # Go test execution and writing
│   ├── knowledge-curator.md        # Wiki, ADR, glossary maintenance
│   ├── observability-reporter.md   # Health dashboards, metrics
│   ├── playwright-browser.md       # Playwright browser automation
│   ├── product-owner.md            # PRD facilitation, story decomposition
│   ├── quick-flow-dev.md           # Rapid spec + implementation
│   ├── release-manager.md          # Release and version management
│   ├── researcher.md               # Deep investigation, spikes
│   ├── scrum-master.md             # Sprint planning, retrospectives
│   ├── security-auditor.md         # Security vulnerability scanning
│   └── team-lead.md                # Multi-agent orchestration
│
├── skills/                         # Claude Code skills (/command invocable)
│   │
│   │ # Git workflow
│   ├── commit/SKILL.md             # Conventional commit creation
│   ├── pr/SKILL.md                 # Pull request creation
│   ├── push/SKILL.md               # Branch push with tracking
│   ├── review/SKILL.md             # Self-review before commit
│   ├── sync/SKILL.md               # Branch sync via rebase
│   ├── changelog/SKILL.md          # Changelog generation
│   │
│   │ # BMAD workflow
│   ├── create-brief/SKILL.md       # Product brief creation (Discovery)
│   ├── create-prd/SKILL.md         # PRD creation (Requirements)
│   ├── create-architecture/SKILL.md # Architecture doc (Architecture)
│   ├── create-stories/SKILL.md     # Epic/story decomposition (Stories)
│   ├── check-readiness/SKILL.md    # Cross-artifact validation
│   ├── correct-course/SKILL.md     # Change management
│   ├── run-checklist/SKILL.md      # Quality gate execution
│   ├── quick-spec/SKILL.md         # Rapid tech spec (Quick Flow)
│   ├── quick-dev/SKILL.md          # Rapid implementation (Quick Flow)
│   ├── adversarial-review/SKILL.md # Self-review with hostile intent
│   │
│   │ # Project tools
│   ├── build/SKILL.md              # Build binary
│   ├── test/SKILL.md               # Run tests
│   ├── lint/SKILL.md               # Linting and formatting
│   ├── security/SKILL.md           # Security scans
│   ├── docker/SKILL.md             # Docker operations
│   ├── release/SKILL.md            # Release preparation
│   ├── add-command/SKILL.md        # Scaffold CLI command
│   ├── add-interface/SKILL.md      # Scaffold interface
│   ├── coverage-report/SKILL.md    # Test coverage report
│   ├── status-check/SKILL.md       # Quick health check
│   ├── health-dashboard/SKILL.md   # Comprehensive health dashboard
│   ├── context-refresh/SKILL.md    # Update task context
│   ├── dependency-check/SKILL.md   # Task dependency analysis
│   ├── knowledge-extract/SKILL.md  # Knowledge extraction
│   ├── onboard/SKILL.md            # Onboarding guide
│   ├── standup/SKILL.md            # Daily standup summary
│   ├── retrospective/SKILL.md      # Task retrospective
│   ├── browser-automate/SKILL.md   # Browser automation
│   ├── playwright/SKILL.md         # Playwright scripts
│   └── ui-review/SKILL.md          # UI/UX review
│
├── checklists/                     # Quality gate checklists (BMAD)
│   ├── prd-checklist.md            # Requirements → Architecture gate
│   ├── architecture-checklist.md   # Architecture → Stories gate
│   ├── story-checklist.md          # Stories → Implementation gate
│   ├── readiness-checklist.md      # Cross-artifact alignment gate
│   └── code-review-checklist.md    # Implementation → Done gate
│
├── artifacts/                      # Phase artifact templates (BMAD)
│   ├── product-brief.md            # Discovery phase output
│   ├── prd.md                      # Requirements phase output
│   ├── architecture-doc.md         # Architecture phase output
│   ├── epics.md                    # Stories phase output
│   └── tech-spec.md                # Quick Flow spec output
│
├── hooks/
│   └── adb-session-capture.sh      # SessionEnd hook for session capture
│
└── rules/
    └── workspace.template.md       # Generic workspace rules
```

## BMAD Workflow

The templates implement the BMAD (Breakthrough Method of Agile AI-Driven
Development) workflow:

```
Discovery → Requirements → Architecture → Stories → Implementation → Review → Done
```

Each phase has:
- An **agent persona** that specializes in that phase's work
- A **skill** that guides the agent through structured artifact creation
- An **artifact template** that standardizes the output
- A **quality gate checklist** that validates before progressing

### Quick Flow (for small tasks)

```
Quick Spec → Quick Dev → Adversarial Review → Done
```

Small tasks (bug fixes, small features) skip the full ceremony and use
the Quick Flow pattern with a lightweight tech spec.
