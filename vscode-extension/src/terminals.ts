// Pure terminal-name helpers for the adb extension — no vscode import, so they
// are unit-testable outside the VS Code runtime. extension.ts builds terminal
// names as `${task.id} ${task.type} ${task.priority}` (e.g. "TASK-00005 refactor P2").

// taskTerminalName is the canonical terminal name for a task.
export function taskTerminalName(id: string, type: string, priority: string): string {
  return `${id} ${type} ${priority}`;
}

// isTaskTerminal reports whether a terminal name belongs to the given task id.
// Matches on the leading "TASK-NNNNN " token so it's robust to type/priority.
export function isTaskTerminal(name: string, taskId: string): boolean {
  return name === taskId || name.startsWith(taskId + " ");
}

// isAnyTaskTerminal reports whether a terminal name was opened for an adb task
// (used by "Close Terminals" to find adb-owned terminals to dispose).
export function isAnyTaskTerminal(name: string): boolean {
  return /^TASK-\d+(\s|$)/.test(name);
}

// orgHeaderName is the display name of a Start-All org-divider terminal:
// "━━ awslabs (3) ━━". Purely visual; it must NOT start with "TASK-" so
// isAnyTaskTerminal never treats it as a task terminal (Close Terminals guard).
export function orgHeaderName(org: string, count: number): string {
  return `━━ ${org} (${count}) ━━`;
}

// orgHeaderBannerCommand is the shell line a divider terminal runs: print a
// banner, then (via composeShellArgs' tail) hand back an interactive login shell
// so the tab stays alive. No tmux — a header is a cheap, disposable marker, not
// a durable session. printf keeps it portable across bash/zsh. org/count come
// from trusted internal values (deriveOrg/counts), never raw user text, but we
// still JSON.stringify the payload so nothing shell-active is interpolated.
export function orgHeaderBannerCommand(org: string, count: number): string {
  const line = `━━━━━━  ${org}  (${count} ticket${count === 1 ? "" : "s"})  ━━━━━━`;
  return `printf '\\n%s\\n\\n' ${JSON.stringify(line)}`;
}
