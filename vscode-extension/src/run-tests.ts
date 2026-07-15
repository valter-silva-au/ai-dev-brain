// Test runner. Imports each *.test module (which registers cases on the
// shared Runner via t.test/t.testSync), then prints a summary and exits
// non-zero on any failure. `npm test` compiles + runs this; `npm run
// coverage` runs it under c8.
import { t } from "./test-harness";
// Import side-effectfully — each module registers its tests synchronously.
import "./terminals.test";
import "./launch.test";
import "./launchRequest.test";
import "./icons.test";
import "./orggroup.test";
import "./config.test";
import "./feedpath.test";
import "./webview/feed.test";
import "./webview/overview.test";
import "./webview/panel.test";
import "./webview/source.test";
import "./webview/chat.test";
import "./webview/actions.test";

async function main(): Promise<void> {
  // Give any pending async tests a tick to settle (none are async right now,
  // but keeping this future-proof).
  await Promise.resolve();
  const total = t.passed + t.failed;
  if (t.failed > 0) {
    console.error(`\n${t.failed} test(s) failed:`);
    for (const f of t.failures) {
      const msg = f.err instanceof Error ? f.err.message : String(f.err);
      console.error(`  [${f.module}] ${f.label}\n      → ${msg}`);
    }
    console.error(`\nFAIL: ${t.failed}/${total} failed, ${t.passed} passed`);
    process.exit(1);
  }
  console.log(`PASS: ${t.passed}/${total} tests passed`);
}

main().catch((e) => {
  console.error("test runner crashed:", e);
  process.exit(2);
});
