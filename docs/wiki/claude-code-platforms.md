# Claude Code Platform Comparison

This document compares Claude Code across different platforms and clarifies which adb integration features are supported on each.

---

## Platform Overview

Claude Code is available on six platforms:

1. **CLI** (command-line interface)
2. **Desktop** (standalone Electron app)
3. **VS Code Extension**
4. **JetBrains Plugin** (IntelliJ, PyCharm, etc.)
5. **Web** (claude.ai/chat in browser)
6. **Slack** (via Claude Slack app)

Each platform has different capabilities and integration points with adb.

---

## Feature Availability Matrix

| Feature | CLI | Desktop | VS Code | JetBrains | Web | Slack |
|---------|-----|---------|---------|-----------|-----|-------|
| **Core Features** |
| Interactive chat | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Tool use (Read, Edit, Write, Bash) | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Multi-agent teams | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Project context | ✅ | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| Conversation history | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **ADB Integration** |
| `adb exec claude` | ✅ | ⚠️ | ❌ | ❌ | ❌ | ❌ |
| `--resume` flag | ✅ | ⚠️ | ⚠️ | ⚠️ | ❌ | ❌ |
| `-p` / `--print` flag | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `--simple` flag | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `--worktree` flag | ✅ | ⚠️ | ⚠️ | ⚠️ | ❌ | ❌ |
| Session capture (auto) | ✅ | ⚠️ | ⚠️ | ⚠️ | ❌ | ❌ |
| Session capture (manual) | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Task env injection (ADB_*) | ✅ | ⚠️ | ⚠️ | ⚠️ | ❌ | ❌ |
| Worktree hooks (PreToolUse) | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Agent isolation: worktree | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| MCP server support | ✅ | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **Performance** |
| Fast mode (`/fast`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Memory leak fixes (v2.1.50+) | ✅ | ✅ | ✅ | ✅ | N/A | N/A |
| Opus 4.6 (1M context) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

**Legend**:
- ✅ Fully supported
- ⚠️ Partial support or requires manual setup
- ❌ Not supported

---

## Platform Details

### CLI (Recommended for ADB)

**What it is**: Native command-line tool installed via `curl | sh` or Homebrew.

**ADB Integration**: **Full support**

- `adb exec claude` launches CLI in subprocess
- `-p` / `--print` flag for non-interactive CI execution
- `--resume` continues conversation from JSONL transcript
- `--worktree` enforces worktree boundary (passed as flag)
- SessionEnd hook captures sessions automatically
- Task environment variables (`ADB_TASK_ID`, `ADB_BRANCH`, etc.) injected

**Advantages**:
- First-class adb integration
- Fastest performance
- Best for CI/CD pipelines
- Works in SSH sessions
- No GUI dependencies

**Disadvantages**:
- No visual editor integration
- No inline code preview
- Terminal-only interface

**Installation**:
```bash
curl -fsSL https://install.anthropic.com | sh

# Or via Homebrew
brew install claude
```

**Version check**:
```bash
claude --version
```

**Recommended for**:
- CI/CD workflows
- Server/remote development
- Terminal-first workflows
- Scripting and automation
- Maximum adb integration

---

### Desktop (Standalone App)

**What it is**: Electron-based GUI application for macOS, Windows, Linux.

**ADB Integration**: **Partial support**

- Cannot be launched via `adb exec claude` (no subprocess support)
- `--resume` requires manual conversation switching
- Session capture requires manual export (no SessionEnd hook)
- Task environment not injected automatically
- Worktree hooks work if project opened in correct directory
- MCP servers work via `.mcp.json` config

**Advantages**:
- Rich GUI with inline code preview
- Side-by-side editor and chat
- Better for visual learners
- Integrated file browser

**Disadvantages**:
- Cannot integrate with `adb exec`
- No automatic session capture
- Manual task context setup
- Slower than CLI

**Workaround for adb integration**:
1. Manually open worktree directory in Desktop app
2. Set environment variables in shell, then launch Desktop:
   ```bash
   export ADB_TASK_ID=TASK-00042
   export ADB_BRANCH=$(git branch --show-current)
   export ADB_WORKTREE_PATH=$(pwd)
   open -a "Claude Desktop"  # macOS
   ```
3. Manually export conversation transcripts for session capture:
   ```bash
   # Find transcript
   cp ~/.claude/projects/*/transcript.jsonl /tmp/session.jsonl

   # Manually capture
   adb session capture --transcript /tmp/session.jsonl --session-id manual-001
   ```

**Recommended for**:
- Visual code exploration
- Learning new codebases
- One-off tasks not tracked by adb
- Developers preferring GUI over terminal

---

### VS Code Extension

**What it is**: Claude Code integrated into Visual Studio Code as an extension.

**ADB Integration**: **Partial support**

- Cannot be launched via `adb exec` (runs inside VS Code)
- `--resume` not applicable (extension maintains state)
- Session capture requires manual export (no hook support)
- Task environment requires VS Code terminal config
- Worktree hooks work if workspace root is worktree
- MCP servers work via extension settings

**Advantages**:
- Integrated with VS Code editor
- Inline suggestions and completions
- Uses VS Code's file tree and terminal
- Familiar interface for VS Code users

**Disadvantages**:
- Tightly coupled to VS Code
- No automatic session capture
- Task environment setup is manual
- Performance tied to VS Code instance

**Workaround for adb integration**:
1. Open worktree folder as VS Code workspace:
   ```bash
   code work/TASK-00042/
   ```
2. Configure VS Code integrated terminal env:
   ```json
   // .vscode/settings.json
   {
     "terminal.integrated.env.linux": {
       "ADB_TASK_ID": "TASK-00042",
       "ADB_BRANCH": "feat/add-auth",
       "ADB_WORKTREE_PATH": "${workspaceFolder}"
     }
   }
   ```
3. Export transcripts manually for session capture (same as Desktop)

**Recommended for**:
- VS Code power users
- Projects already using VS Code
- Teams standardized on VS Code
- When editor integration is more important than adb automation

---

### JetBrains Plugin

**What it is**: Claude Code plugin for IntelliJ IDEA, PyCharm, WebStorm, etc.

**ADB Integration**: **Partial support** (similar to VS Code)

- Cannot be launched via `adb exec`
- Session capture requires manual export
- Task environment requires IDE run configuration
- Worktree hooks work if project root is worktree

**Advantages**:
- Integrated with JetBrains ecosystem
- Language-specific features (Java, Python, etc.)
- Familiar for JetBrains users

**Disadvantages**:
- Same limitations as VS Code extension
- Heavier resource usage than CLI
- IDE restart required for environment changes

**Workaround for adb integration**:
Similar to VS Code, configure run configurations with environment variables.

**Recommended for**:
- JetBrains IDE users
- Java, Kotlin, Python, or Scala projects
- Teams already using JetBrains tooling

---

### Web (claude.ai/chat)

**What it is**: Browser-based Claude interface at claude.ai/chat.

**ADB Integration**: **Not supported**

- No tool use (Read, Edit, Write, Bash)
- No local file access
- No project context
- No session capture
- No MCP servers (limited preview)
- No multi-agent teams

**Advantages**:
- Zero installation
- Works on any device with browser
- Fast for quick questions
- Good for general assistant usage

**Disadvantages**:
- Cannot integrate with adb at all
- No file operations
- No code execution
- No project awareness

**Use cases**:
- Quick questions unrelated to code
- Mobile access
- Devices where CLI cannot be installed
- Documentation research

**Not recommended for**: Any adb-integrated workflows.

---

### Slack

**What it is**: Claude Slack app for team collaboration.

**ADB Integration**: **Not supported**

- No tool use
- No file operations
- No project context
- No session capture
- Conversation history tied to Slack workspace

**Advantages**:
- Team-accessible
- Centralized in Slack
- Good for team Q&A

**Disadvantages**:
- No adb integration
- No code execution
- No file editing
- Limited context

**Use cases**:
- Team questions
- Shared knowledge base
- Onboarding new team members

**Not recommended for**: Development workflows with adb.

---

## Session Capture Platform Support

Session capture is the most platform-dependent adb feature.

### Fully Automatic (CLI Only)

**Platform**: CLI

**How it works**:
1. `adb resume TASK-XXXXX` launches CLI with environment variables set
2. User works in Claude Code session
3. User exits Claude Code
4. SessionEnd hook fires automatically
5. `adb session capture --from-hook` runs
6. Session stored in `sessions/S-XXXXX/`
7. If `ADB_TASK_ID` set, symlink created in task's `sessions/` directory

**Requirements**:
- Claude Code CLI v2.1.50+
- adb v1.6.0+ for session capture
- SessionEnd hook installed (`adb sync-claude-user`)

---

### Semi-Automatic (Desktop, VS Code, JetBrains)

**Platforms**: Desktop, VS Code, JetBrains

**How it works**:
1. User manually exports conversation transcript from IDE/app
2. User runs `adb session capture` manually with transcript path:
   ```bash
   adb session capture --transcript /path/to/transcript.jsonl --session-id manual-001
   ```
3. Session stored in `sessions/S-XXXXX/`

**Requirements**:
- adb v1.6.0+
- Manual transcript export per session
- No automatic task linking (unless `ADB_TASK_ID` set in shell before manual capture)

---

### Not Supported (Web, Slack)

**Platforms**: Web, Slack

**Reason**: No access to conversation transcripts via local filesystem. Conversations stored in Anthropic's cloud.

**Workaround**: Copy-paste conversation manually into a task session file:
```bash
# Manual session summary (not captured session)
adb session save TASK-XXXXX
# Then edit the generated markdown file
```

---

## Worktree Isolation Platform Support

Worktree isolation (via `--worktree` flag and PreToolUse hooks) depends on local filesystem access.

### Supported (CLI, Desktop, VS Code, JetBrains)

**How it works**:
1. `.claude/hooks/pre-edit-validate.sh` installed in worktree
2. Hook registered in `.claude/settings.json`
3. Before every Edit/Write tool use, hook validates file path is inside worktree
4. If path outside worktree, tool call is blocked

**Configuration** (`.claude/settings.json`):
```json
{
  "hooks": {
    "PreToolUse": {
      "command": "bash",
      "args": [".claude/hooks/pre-edit-validate.sh"]
    }
  }
}
```

**Platforms**:
- ✅ CLI (full support, `--worktree` flag)
- ✅ Desktop (manual project selection)
- ✅ VS Code (workspace root must be worktree)
- ✅ JetBrains (project root must be worktree)

---

### Not Supported (Web, Slack)

**Reason**: No local filesystem access, no hook execution.

---

## MCP Server Platform Support

### Supported (CLI, Desktop, VS Code, JetBrains)

**Configuration**: `.mcp.json` in project root or `~/.claude.json` globally.

```json
{
  "mcpServers": {
    "aws-knowledge": {
      "type": "http",
      "url": "https://knowledge-mcp.global.api.aws"
    },
    "context7": {
      "type": "http",
      "url": "https://mcp.context7.com/mcp"
    }
  }
}
```

**Platforms**:
- ✅ CLI (via `~/.claude.json` or project `.mcp.json`)
- ✅ Desktop (via settings UI or `.mcp.json`)
- ✅ VS Code (via extension settings or `.mcp.json`)
- ✅ JetBrains (via plugin settings or `.mcp.json`)

### Limited Support (Web)

**Status**: MCP preview available for select servers (AWS, Google, etc.) but not user-configurable.

### Not Supported (Slack)

**Reason**: No MCP integration in Slack app.

---

## Recommendation by Use Case

### Use Case: CI/CD Automation

**Recommended Platform**: CLI

**Why**:
- `-p` / `--print` flag for non-interactive execution
- `--simple` flag for cleaner CI logs
- Subprocess support via `adb exec claude`
- Fastest execution (no GUI overhead)

**Example**:
```bash
adb exec claude -p "/fast run tests and report failures" --simple
```

---

### Use Case: Daily Development (Terminal-First)

**Recommended Platform**: CLI

**Why**:
- Full adb integration (automatic session capture, task env injection)
- Works in tmux/SSH sessions
- Best performance
- `adb resume` workflow

**Example**:
```bash
adb resume TASK-00042
# Claude Code launches automatically
```

---

### Use Case: Daily Development (IDE-First)

**Recommended Platform**: VS Code Extension or JetBrains Plugin

**Why**:
- Integrated with familiar editor
- Inline code preview
- No context switching

**Tradeoff**: Manual session capture, manual environment setup.

---

### Use Case: Exploring New Codebases

**Recommended Platform**: Desktop or VS Code Extension

**Why**:
- Visual file browser
- Side-by-side code and chat
- Better for visual learners

**Tradeoff**: No automatic adb integration.

---

### Use Case: Quick Questions (Non-Code)

**Recommended Platform**: Web (claude.ai)

**Why**:
- Zero setup
- Works anywhere (mobile, tablet)
- Fast for one-off questions

**Limitation**: No file operations or project context.

---

### Use Case: Team Knowledge Sharing

**Recommended Platform**: Slack

**Why**:
- Centralized in team's communication tool
- Shared conversation history
- Good for onboarding

**Limitation**: No code execution or adb integration.

---

## Migration Between Platforms

### From Web to CLI

1. Install Claude Code CLI:
   ```bash
   curl -fsSL https://install.anthropic.com | sh
   ```
2. No conversation history migrates (Web conversations stay in cloud)
3. Start fresh conversations in CLI

---

### From Desktop/VS Code to CLI

1. Export conversation transcripts manually (if needed)
2. Install CLI
3. Use `adb resume` workflow going forward
4. Enable SessionEnd hook for automatic capture:
   ```bash
   adb sync-claude-user
   ```

---

### From CLI to Desktop/VS Code

1. Conversation history in JSONL format can be opened in Desktop/VS Code (same format)
2. Lose automatic session capture
3. Lose `adb exec` integration
4. Lose automatic task environment injection

---

## Platform-Specific Issues

### CLI: JSONL Transcript Corruption

**Symptom**: `adb session capture` fails with "malformed JSON".

**Fix**: Claude Code v2.1.50+ fixes this. Upgrade:
```bash
curl -fsSL https://install.anthropic.com | sh
```

---

### Desktop: Environment Variables Not Visible

**Symptom**: Claude cannot see `ADB_TASK_ID` in tools.

**Fix**: Set environment in terminal before launching Desktop:
```bash
export ADB_TASK_ID=TASK-00042
open -a "Claude Desktop"
```

---

### VS Code: Extension Conflicts

**Symptom**: Claude Code extension not responding.

**Fix**: Disable other AI assistant extensions (GitHub Copilot, Tabnine, etc.):
```bash
code --disable-extension github.copilot
```

---

### JetBrains: Plugin Not Found

**Symptom**: Claude Code plugin not available in plugin marketplace.

**Fix**: Install manually from JetBrains marketplace or Claude's website.

---

## See Also

- **docs/runbooks/troubleshooting.md**: Platform-specific troubleshooting
- **docs/wiki/performance-tuning.md**: CLI performance optimization
- **docs/runbooks/ci-integration.md**: CLI-specific CI setup
- **CLAUDE.md**: Full CLI command reference
