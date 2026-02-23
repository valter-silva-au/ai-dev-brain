---
name: ui-review
description: Orchestrate parallel browser QA by scanning story files and spawning browser-qa agents for each
argument-hint: "[stories-directory]"
allowed-tools: Task, Read, Glob, Grep
---

# UI Review Skill

Scan a directory for `*.md` story files and execute them all in parallel using browser-qa agents. This is an orchestration skill -- it does not perform browser work itself, it coordinates agents that do.

## Steps

### 1. Determine Target Directory

Based on the argument provided:

- **No arguments**: Default to `./stories/` directory
- **Directory argument**: Use the specified directory path

### 2. Discover Story Files

Use Glob to find all `*.md` files in the target directory:
```
<target-directory>/*.md
```

If no story files are found, report "No story files found in <directory>" and stop.

### 3. Execute Stories in Parallel

For each story file found, spawn a `browser-qa` agent using the Task tool:

- **subagent_type**: `browser-qa`
- **prompt**: Include the full contents of the story file (read it first with the Read tool)
- **description**: "Execute UI story: <filename>"

Spawn ALL agents in a single message so they run in parallel. Do not wait for one agent to finish before spawning the next.

### 4. Collect Results

Wait for all browser-qa agents to complete. Each agent will return:
- Pass/fail status
- Any assertion failures or errors
- Screenshots taken (if applicable)

### 5. Generate Summary Report

Print a summary report to stdout:

```
=== UI Review Summary ===

Directory: <target-directory>
Stories:   <total> total, <passed> passed, <failed> failed

Results:
  [PASS] story-login.md
  [FAIL] story-checkout.md -- Assertion failed: expected "Order Confirmed" in page title
  [PASS] story-navigation.md

Failed stories:
  1. story-checkout.md
     - Step 3: Click "Place Order" -- expected "Order Confirmed" in page title, got "Error Page"
```

If all stories pass, end with: "All stories passed."
If any stories fail, end with: "<N> story(ies) failed. Review the details above."
