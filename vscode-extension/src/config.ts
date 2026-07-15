// Pure config normalizers for the adb extension. NO vscode import so these
// are unit-testable via the hand-rolled harness. extension.ts does the impure
// getConfig().get<T>() reads and passes the raw values here; the pure planner
// (launch.ts + orggroup.ts) and the terminal-env builder consume the
// normalized result.

export interface StartAllConfig {
  groupByOrg: boolean;
  orderByPriority: boolean;
  cap: number;
  orgOrder: string[];
}

export interface TmuxConfig {
  enabled: boolean;
  sessionPrefix: string;
}

// Raw shapes mirror what getConfig().get<T>() may hand back — every field is
// optional and the runtime type may not match the declared type (VS Code
// falls back to the schema default, but user-edited settings.json can lie).
export interface RawStartAll {
  groupByOrg?: boolean;
  orderByPriority?: boolean;
  cap?: number;
  orgOrder?: string[];
}

export interface RawTmux {
  enabled?: boolean;
  sessionPrefix?: string;
}

// normalizeStartAllConfig converts a raw workspace-config object into a
// typed StartAllConfig. Boolean fields default to true (schema default);
// cap is clamped to a non-negative integer (0 = unlimited); orgOrder is
// defensively copied to insulate callers from later mutation of the input.
export function normalizeStartAllConfig(raw: RawStartAll): StartAllConfig {
  const cap =
    typeof raw.cap === "number" && Number.isFinite(raw.cap) && raw.cap > 0
      ? Math.floor(raw.cap)
      : 0;
  return {
    groupByOrg: raw.groupByOrg !== false, // undefined → true
    orderByPriority: raw.orderByPriority !== false,
    cap,
    orgOrder: Array.isArray(raw.orgOrder) ? raw.orgOrder.slice() : [],
  };
}

// sanitizePrefix mirrors the CLI's tmuxSessionName sanitizer intent: only
// [A-Za-z0-9_-] survive; anything else → '-'. Runs of '-' are NOT collapsed
// here (the CLI collapses on the full <prefix><basename>); we only guard
// against an argv-breaking prefix (defence in depth — the Go side re-runs
// the same sanitizer, see internal/cli/launch.go tmuxSessionNameWithPrefix).
function sanitizePrefix(s: string): string {
  return s.replace(/[^A-Za-z0-9_-]/g, "-");
}

// normalizeTmuxConfig converts a raw workspace-config object into a typed
// TmuxConfig. enabled defaults to true (schema default). sessionPrefix is
// trimmed and sanitized; empty/whitespace-only falls back to "cc-" so we
// never emit an argv-empty prefix.
export function normalizeTmuxConfig(raw: RawTmux): TmuxConfig {
  const trimmed = (raw.sessionPrefix ?? "").trim();
  const prefix = trimmed === "" ? "cc-" : sanitizePrefix(trimmed);
  return {
    enabled: raw.enabled !== false,
    sessionPrefix: prefix,
  };
}
