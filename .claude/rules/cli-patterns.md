---
paths:
  - "internal/cli/**/*.go"
---

CLI patterns for Cobra commands in this project:

- Each command in its own file: internal/cli/commandname.go
- Package-level variables for DI: TaskMgr, UpdateGen, AICtxGen, Executor, Runner
- Use cobra.ExactArgs(N) or cobra.MinimumNArgs(N) for arg validation
- Commands with DisableFlagParsing must manually handle --help/-h
- Use RunE (not Run) to return errors properly
- Format output for terminal: tables for status, plain text for messages
- Wiring happens in internal/app.go NewApp() then sets cli.TaskMgr = app.TaskMgr etc.
- Keep commands thin: validate input, call core service, format output
- Error messages should be user-friendly, not raw Go errors
