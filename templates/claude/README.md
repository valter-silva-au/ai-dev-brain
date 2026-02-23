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
- Copies `statusline.sh` → `.claude/statusline.sh` (made executable)

## `adb sync-claude-user`

Syncs universal skills, agents, and status line to `~/.claude/`:

- Copies `skills/*/SKILL.md` → `~/.claude/skills/*/SKILL.md`
- Copies `agents/*.md` → `~/.claude/agents/*.md`
- Copies `statusline.sh` → `~/.claude/statusline.sh` (made executable)
- Merges `statusLine` config into `~/.claude/settings.json`

With `--mcp` flag, also merges MCP servers into `~/.claude.json`:

- Reads `mcp-servers.json` and merges each server into the user config
- Existing servers are updated, new ones added, unrelated keys preserved

## Status Line

The `statusline.sh` script provides tiered context in Claude Code's status bar:

| Tier | Gate | Data shown |
|------|------|-----------|
| 1 (always) | none | project name, model, context%, cost, lines +/-, duration, agent |
| 2 (git) | `git rev-parse` | branch, dirty count, ahead/behind |
| 3 (adb) | `.taskconfig` found | task ID/type/priority/status, portfolio counts, alerts |

## New machine setup

```bash
adb sync-claude-user --mcp
export CONTEXT7_API_KEY="your-key-here"  # add to shell profile
```

## Directory Structure

```
templates/claude/
├── README.md                  # This file
├── embed.go                   # Embeds all templates into the binary at compile time
├── claudeignore.template      # Default .claudeignore
├── settings.template.json     # Default .claude/settings.json
├── statusline.sh              # Universal status line script (tiered enrichment)
├── mcp-servers.json           # MCP servers to merge into ~/.claude.json
├── agents/
│   └── code-reviewer.md       # Generic code review agent
├── skills/
│   ├── commit/SKILL.md        # Conventional commit creation
│   ├── pr/SKILL.md            # Pull request creation
│   ├── push/SKILL.md          # Branch push with tracking
│   ├── review/SKILL.md        # Self-review before commit
│   ├── sync/SKILL.md          # Branch sync via rebase
│   └── changelog/SKILL.md     # Changelog generation
└── rules/
    └── workspace.template.md  # Generic workspace rule template
```
