# Work

Git worktrees for active tasks. Each task with a `--repo` flag gets an isolated working directory here, allowing parallel development across tasks without stashing or branch switching.

## Structure

```
work/
└── TASK-00001/              # Git worktree for task
```

## How It Works

- Worktrees are created automatically when tasks specify `--repo`: `adb feat my-feature --repo github.com/org/repo`
- Each worktree is branched from the repository's default branch
- Worktrees provide full isolation so you can switch between tasks freely
- The worktree path is stored in the task's `status.yaml` and injected as `ADB_WORKTREE_PATH` into tool commands

## Notes

- Do not manually create or delete worktree directories -- use `adb` commands
- Resume a task to get its worktree path: `adb resume <task-id>`
