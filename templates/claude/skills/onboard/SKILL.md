---
name: onboard
description: Generate an onboarding guide for new contributors or new AI coding sessions
allowed-tools: Read, Glob, Grep
---

# Onboard Skill

Generate a concise onboarding guide that helps new contributors or new AI sessions quickly understand the project state and get productive.

## Steps

### 1. Project Overview
Read `CLAUDE.md` to extract:
- Project description and purpose
- Technology stack
- Key commands (build, test, lint)

### 2. Current State
Read `backlog.yaml` to determine:
- Active tasks (in_progress, blocked, review)
- Upcoming tasks (backlog)
- Recently completed tasks (done)

### 3. Architecture Summary
Read `docs/architecture.md` for:
- Package layout and responsibilities
- Key patterns (local interfaces, adapters)
- Dependency flow rules

### 4. Recent Decisions
Scan `docs/decisions/` for recent ADRs (sorted by number, last 5).

### 5. Active Conventions
Scan `docs/wiki/` for convention-related articles.
Read `docs/glossary.md` for project terminology.

### 6. Generate Guide

```
=== Onboarding Guide ===
Generated: YYYY-MM-DD

-- Project --
AI Dev Brain (adb): [brief description]
Stack: Go 1.24, Cobra, Viper, yaml.v3

-- Quick Start --
Build:  go build -o adb ./cmd/adb/
Test:   go test ./... -count=1
Lint:   golangci-lint run
Run:    go run ./cmd/adb/ [command]

-- Active Tasks --
[TASK-XXXXX] description (status, priority)
[TASK-XXXXX] description (status, priority)

-- Architecture --
Layers: CLI -> Core -> Storage/Integration
Key pattern: Local interfaces in core/, adapters in app.go
Rule: core/ never imports storage/ or integration/

-- Recent Decisions --
ADR-XXXX: title (status)

-- Key Files to Read First --
1. CLAUDE.md -- Full project context
2. internal/app.go -- Dependency wiring
3. internal/core/taskmanager.go -- Core business logic
4. internal/cli/root.go -- CLI entry point

-- Conventions --
- Error wrapping: fmt.Errorf("context: %w", err)
- File permissions: dirs 0o755, files 0o644
- Timestamps: time.Now().UTC()
- Tests: t.TempDir() for isolation
```
