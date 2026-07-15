# Project Context

## Overview
This workspace uses AI Dev Brain for task management and AI-assisted development.

## Structure
- `tickets/` - Task-specific context and notes
- `work/` - Git worktrees for task isolation
- `sessions/` - Captured session data
- `backlog.yaml` - Task backlog
- `.taskrc` - Workspace configuration

## Commands
- `adb task create` - Create new task with worktree
- `adb task resume <task-id>` - Resume task
- `adb task status` - View all tasks
- `adb team <name> <prompt>` - Launch multi-agent orchestration
- `adb agents` - List available agents
- `adb mcp check` - Validate MCP server health
