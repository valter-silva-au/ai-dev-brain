# Go Engineering Patterns

## Overview

This article captures Go-specific engineering patterns, conventions, and learnings accumulated across multiple adb development tasks. These patterns apply to the `adb` CLI codebase built with Go 1.24, Cobra, and Viper.

## Key Decisions

- **`CreateTaskOpts` struct pattern**: Optional task creation parameters are passed via an options struct rather than individual function arguments. This allows adding new optional fields without breaking callers (K-00020).
- **Shell delegation for Taskfile commands**: Taskfile commands are executed via `sh -c` on Linux/Mac and `cmd /c` on Windows, ensuring shell features (pipes, redirects, environment expansion) work correctly (K-00021).
- **97% coverage ceiling accepted**: The remaining gaps are unreachable defensive code (error paths that cannot be triggered through public APIs). Pursuing 100% coverage would require either removing safety checks or adding brittle test hacks (K-00022).

## Learnings

- **Archived task relocation**: Moving archived ticket folders from `tickets/TASK-XXXXX/` to `tickets/_archived/TASK-XXXXX/` declutters the VS Code file explorer and improves developer navigation. Active tasks remain in `tickets/` while archived tasks are physically separated (K-00023).
- **`--resume` flag for Claude Code**: The `launchWorkflow` function supports a `resume` parameter. `adb resume` passes `true` (launches Claude Code with `--resume` to continue the most recent conversation); task creation commands pass `false` (starts a fresh conversation) (K-00024).

## Gotchas

- Shell delegation means Taskfile commands inherit the system shell's behavior. Commands that work on Linux may behave differently on Windows due to `cmd /c` vs `sh -c` differences. Test cross-platform if Taskfile commands use shell-specific features.
- The options struct pattern requires nil/zero-value awareness. Callers that pass an empty `CreateTaskOpts{}` get all defaults, which may differ from passing explicit zero values.

---
*Sources: TASK-00029, TASK-00032*
