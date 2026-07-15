// Pure task-type → VS Code codicon mapping. No vscode/fs imports so it can be
// unit-tested outside the extension host (the repo's "pure core + thin glue"
// pattern). extension.ts imports TYPE_ICONS/iconForType and wraps the string in
// a vscode.ThemeIcon.
//
// Codicon ids: https://code.visualstudio.com/api/references/icons-in-labels

// TYPE_ICONS maps each adb task type to a VS Code codicon id. Covers the WS-B
// Conventional set (feat, fix, refactor, docs, chore, test, perf, spike) plus
// the legacy `bug` alias so old backlog entries still render an icon.
export const TYPE_ICONS: Record<string, string> = {
  feat: "add",
  fix: "bug",
  refactor: "wrench",
  docs: "book",
  chore: "gear",
  test: "beaker",
  perf: "dashboard",
  spike: "beaker",
  bug: "bug", // legacy alias — pre-taxonomy entries
};

// FALLBACK_ICON is used for any type not in TYPE_ICONS.
export const FALLBACK_ICON = "tasklist";

// iconForType returns the codicon id for a task type, falling back to
// FALLBACK_ICON for unknown or missing types.
export function iconForType(type?: string): string {
  if (type && TYPE_ICONS[type]) {
    return TYPE_ICONS[type];
  }
  return FALLBACK_ICON;
}
