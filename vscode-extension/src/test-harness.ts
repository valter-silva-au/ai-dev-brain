// Tiny dependency-free test harness. Each test module imports `t` and calls
// t.test(label, fn). At the bottom of run-tests.ts we summarize and exit.
//
// Why hand-rolled: the extension's only devDeps are typescript + @types; we
// don't want to drag in mocha/jest just for a handful of pure-function tests,
// and c8 wraps any node command for coverage so we don't need a framework
// integration. This stays portable and offline-installable.

interface Failure {
  label: string;
  module: string;
  err: unknown;
}

class Runner {
  passed = 0;
  failed = 0;
  failures: Failure[] = [];
  currentModule = "<unknown>";

  setModule(name: string): void {
    this.currentModule = name;
  }

  async test(label: string, fn: () => void | Promise<void>): Promise<void> {
    try {
      await fn();
      this.passed++;
    } catch (err) {
      this.failed++;
      this.failures.push({ label, module: this.currentModule, err });
    }
  }

  // Synchronous-only convenience for cases that don't need async.
  testSync(label: string, fn: () => void): void {
    try {
      fn();
      this.passed++;
    } catch (err) {
      this.failed++;
      this.failures.push({ label, module: this.currentModule, err });
    }
  }
}

export const t = new Runner();

// Minimal assertion helpers — throw on failure so the runner records them.
export function eq<T>(actual: T, expected: T, msg?: string): void {
  if (actual !== expected) {
    throw new Error(
      `${msg ? msg + ": " : ""}expected ${JSON.stringify(
        expected
      )}, got ${JSON.stringify(actual)}`
    );
  }
}

export function deepEq<T>(actual: T, expected: T, msg?: string): void {
  const a = JSON.stringify(actual);
  const b = JSON.stringify(expected);
  if (a !== b) {
    throw new Error(`${msg ? msg + ": " : ""}expected ${b}, got ${a}`);
  }
}

export function ok(cond: unknown, msg?: string): void {
  if (!cond) {
    throw new Error(msg || "assertion failed");
  }
}

export function notOk(cond: unknown, msg?: string): void {
  if (cond) {
    throw new Error(msg || "expected falsy, got truthy");
  }
}
