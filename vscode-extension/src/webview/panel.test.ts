// Tests for the pure HTML scaffold builder that populates a WebviewPanel.
// The scaffold must contain the two read-only regions (feed + overview),
// carry a strict CSP with the caller-supplied nonce, and be free of inline
// event handlers (so CSP without 'unsafe-inline' actually blocks nothing we
// need).
import { t, ok, notOk, eq } from "../test-harness";
import {
  renderPanelHtml,
  makeNonce,
  MessageKind,
  PanelToWebviewMessage,
  WebviewToPanelMessage,
} from "./panel";

t.setModule("panel");

// ---- renderPanelHtml: structural anchors --------------------------------
t.testSync("renderPanelHtml: contains #feed and #overview regions", () => {
  const html = renderPanelHtml("nonce123");
  ok(html.includes('id="feed"'), "expected #feed region");
  ok(html.includes('id="overview"'), "expected #overview region");
});

t.testSync("renderPanelHtml: includes a #chat region (F4)", () => {
  // F4 adds the chat region. Every button in it goes through a modal
  // confirm on the extension side — the region itself is inert
  // decoration until postMessage traffic starts.
  const html = renderPanelHtml("nonce123");
  ok(html.includes('id="chat"'), "F4 must ship a chat region");
  // Presence of the input+send button so the smoke test catches accidental
  // regressions (e.g. tab-nav loses the send button).
  ok(html.includes('id="chat-input"'), "expected #chat-input");
  ok(html.includes('id="chat-send"'), "expected #chat-send");
  ok(html.includes('id="chat-log"'), "expected #chat-log");
});

t.testSync("renderPanelHtml: has a Content-Security-Policy meta tag", () => {
  const html = renderPanelHtml("nonce123");
  ok(
    html.includes('http-equiv="Content-Security-Policy"'),
    "expected CSP meta tag"
  );
});

t.testSync("renderPanelHtml: CSP includes the caller-supplied nonce", () => {
  const html = renderPanelHtml("abcXYZ");
  ok(html.includes("nonce-abcXYZ"), "CSP must reference the nonce");
});

t.testSync("renderPanelHtml: CSP default-src 'none' (strict)", () => {
  const html = renderPanelHtml("nonce123");
  ok(html.includes("default-src 'none'"), "expected strict default-src 'none'");
});

t.testSync("renderPanelHtml: script tag carries the nonce", () => {
  const html = renderPanelHtml("nonce123");
  // The one script we ship must be nonce'd, not inline-unsafe.
  ok(
    /<script[^>]+nonce="nonce123"/.test(html),
    "expected <script nonce=…> in the rendered html"
  );
});

t.testSync("renderPanelHtml: no inline onclick/onload/onerror handlers", () => {
  const html = renderPanelHtml("nonce123");
  // A blanket onXxx= scan — CSP without 'unsafe-inline' would already block
  // these at runtime, but we forbid them at authoring time so we don't
  // accidentally rely on a permissive CSP later.
  notOk(/\son[a-z]+=/i.test(html), "no inline event handlers allowed");
});

t.testSync("renderPanelHtml: no javascript: URLs", () => {
  const html = renderPanelHtml("nonce123");
  notOk(html.toLowerCase().includes("javascript:"), "no javascript: URLs");
});

t.testSync("renderPanelHtml: has a <title> and viewport meta", () => {
  const html = renderPanelHtml("nonce123");
  ok(html.includes("<title>"), "expected a <title>");
  ok(html.includes('name="viewport"'), "expected a viewport meta");
});

t.testSync("renderPanelHtml: doctype + html lang set", () => {
  const html = renderPanelHtml("nonce123");
  ok(html.trimStart().toLowerCase().startsWith("<!doctype html"), "expected <!DOCTYPE html>");
  ok(/<html[^>]+lang=/i.test(html), "expected lang attribute on <html>");
});

// ---- makeNonce: cryptographically-scoped-enough per-open value ----------
t.testSync("makeNonce: returns a 16+ char alphanum string", () => {
  const n = makeNonce();
  ok(/^[A-Za-z0-9]+$/.test(n), `nonce must be alphanum, got ${n}`);
  ok(n.length >= 16, `nonce should be ≥16 chars, got ${n.length}`);
});

t.testSync("makeNonce: two calls produce different nonces (best-effort)", () => {
  const a = makeNonce();
  const b = makeNonce();
  ok(a !== b, "two nonces should differ");
});

// ---- Message-protocol types compile & round-trip ------------------------
// The pure module also owns the message shapes so the extension glue and
// the (nonce'd) webview script share a single source of truth. These are
// compile-check tests — if the type surface breaks, tsc fails before the
// runner runs.
t.testSync("MessageKind enum-like contains F3 + F4 kinds", () => {
  const kinds: MessageKind[] = [
    "event",
    "overview",
    "hello",
    "chat.reply",
    "chat.error",
  ];
  eq(kinds.length, 5);
});

t.testSync("PanelToWebviewMessage: chat.reply carries text + proposals", () => {
  const msg: PanelToWebviewMessage = {
    kind: "chat.reply",
    text: "hi",
    proposals: [{ id: 0, summary: "TASK-1: status → blocked", executable: true }],
  };
  eq(msg.kind, "chat.reply");
  if (msg.kind === "chat.reply") {
    eq(msg.proposals[0].id, 0);
    eq(msg.proposals[0].executable, true);
  }
});

t.testSync("WebviewToPanelMessage: chat.confirm carries a proposalId", () => {
  const msg: WebviewToPanelMessage = { kind: "chat.confirm", proposalId: 3 };
  eq(msg.kind, "chat.confirm");
  if (msg.kind === "chat.confirm") {
    eq(msg.proposalId, 3);
  }
});

t.testSync("PanelToWebviewMessage: event carries a raw JSONL line", () => {
  const msg: PanelToWebviewMessage = { kind: "event", raw: "{\"type\":\"task.created\"}" };
  eq(msg.kind, "event");
  ok("raw" in msg);
});

t.testSync("PanelToWebviewMessage: overview carries an OverviewCard[]", () => {
  const msg: PanelToWebviewMessage = { kind: "overview", cards: [] };
  eq(msg.kind, "overview");
  ok(Array.isArray(msg.cards));
});

t.testSync("WebviewToPanelMessage: hello (F3 read-only surface)", () => {
  const msg: WebviewToPanelMessage = { kind: "hello" };
  eq(msg.kind, "hello");
});
