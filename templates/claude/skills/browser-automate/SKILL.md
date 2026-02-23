---
name: browser-automate
description: Execute a browser automation workflow by spawning a playwright-browser agent with the workflow context
argument-hint: "<workflow-file-or-instructions>"
allowed-tools: Task, Read, Bash
---

# Browser Automate Skill

Execute a browser automation workflow by delegating to a playwright-browser agent. This is a Higher Order Prompt (HOP) -- it wraps user intent and delegates to a specialized agent.

## Steps

### 1. Determine Input Type

Examine the argument to decide whether it is a file path or inline instructions:

- **File path**: If the argument contains `/` or `\`, or ends in `.md`, `.yaml`, `.yml`, or `.txt`, treat it as a file path. Read the file contents using the Read tool.
- **Inline instructions**: Otherwise, treat the entire argument as inline workflow instructions.

### 2. Spawn Playwright Browser Agent

Use the Task tool to spawn a `playwright-browser` agent:

- **subagent_type**: `playwright-browser`
- **prompt**: The workflow instructions (either file contents or inline text)
- **description**: "Browser automation: <brief summary of the workflow>"

The playwright-browser agent will:
- Launch a Chromium browser via Playwright MCP
- Execute the workflow steps (navigate, click, fill forms, take screenshots, etc.)
- Return results including any captured data or screenshots

### 3. Return Results

Pass through the results from the playwright-browser agent directly. Include:
- Any output or captured data the agent produced
- Screenshot file paths if screenshots were taken
- Any errors encountered during execution

## Examples

When invoked as `/browser-automate workflows/login-test.md`:
1. Detects file path (contains `/` and ends in `.md`)
2. Reads `workflows/login-test.md`
3. Spawns playwright-browser agent with the file contents
4. Returns the agent's results

When invoked as `/browser-automate Go to example.com and take a screenshot`:
1. Detects inline instructions (no path separators, no file extension)
2. Spawns playwright-browser agent with the text as instructions
3. Returns the agent's results
