---
name: playwright
description: Browser automation using Playwright CLI for navigation, screenshots, and testing
argument-hint: "[url|install|test]"
allowed-tools: Bash, Read, Write
---

# Playwright Skill

Browser automation using the Playwright CLI (`npx playwright`).

## Steps

Based on the argument provided:

### No arguments

Launch interactive mode. Print the available sub-commands:

```
Available Playwright sub-commands:
  /playwright <url>       -- Navigate to URL and take a screenshot
  /playwright install     -- Install Playwright browsers
  /playwright test        -- Run Playwright test suite
  /playwright codegen     -- Open Playwright codegen for recording interactions

Append --headed to any URL command to run in visible browser mode.
```

### URL argument (starts with `http`)

Navigate to the URL and take a screenshot:

```bash
npx playwright screenshot <url> screenshot.png
```

- Runs in headless mode by default.
- If the user appends `--headed`, pass the flag to run in visible browser mode:
  ```bash
  npx playwright screenshot --headed <url> screenshot.png
  ```
- If the user specifies a custom output filename, use that instead of `screenshot.png`.
- Screenshots are saved to the current working directory by default.
- For authenticated sessions, mention that `--user-data-dir=./browser-profile` can persist login state across runs.

### "install" argument

Install Playwright browsers (chromium only for speed):

```bash
npx playwright install --with-deps chromium
```

Report the installed browser version when complete.

### "test" argument

Run the Playwright test suite:

```bash
npx playwright test
```

- If a specific test file is provided (e.g., `/playwright test login.spec.ts`), pass it through:
  ```bash
  npx playwright test login.spec.ts
  ```
- To run in headed mode: `npx playwright test --headed`
- Report pass/fail counts from the test output.

### "codegen" argument

Open Playwright codegen to record browser interactions:

```bash
npx playwright codegen <url>
```

This launches a browser and records user actions as code.

## Key Commands Reference

| Command | Description |
|---------|-------------|
| `npx playwright screenshot <url> <file>` | Navigate to URL and save screenshot |
| `npx playwright screenshot --headed <url> <file>` | Same but with visible browser |
| `npx playwright install --with-deps chromium` | Install chromium browser |
| `npx playwright test` | Run all Playwright tests |
| `npx playwright test <file>` | Run a specific test file |
| `npx playwright codegen <url>` | Record interactions as code |
| `npx playwright show-trace <file>` | View a Playwright trace file |

## Notes

- Headless mode is the default for all commands.
- Use `--user-data-dir=./browser-profile` to persist cookies, localStorage, and login sessions across runs.
- Screenshots are saved to the current directory unless an absolute path is specified.
- Playwright must be installed first (`/playwright install`) before other commands work.
