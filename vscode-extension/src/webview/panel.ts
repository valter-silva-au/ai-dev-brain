// Pure HTML scaffold + message-protocol types for the AI Dev Brain webview
// dashboard. The extension glue (extension.ts) creates a WebviewPanel and
// sets `webview.html = renderPanelHtml(nonce)` — this module never touches
// the vscode API, so the whole scaffold + protocol is unit-testable under
// `node`.
//
// Threat model / CSP: the webview runs in a heavily sandboxed frame; we
// still lock it down with a strict CSP (default-src 'none' + nonce'd script
// + nonce'd style). We refuse inline event handlers at authoring time so
// nothing accidentally relies on a laxer CSP later.
import type { OverviewCard } from "./overview";

// ---- Message protocol ----------------------------------------------------
// Kinds the extension → webview stream carries. Kept as a string-literal
// union rather than an enum so it survives the erase-only tsc translation
// used by the extension.
export type MessageKind =
  | "event"
  | "overview"
  | "hello"
  | "chat.reply"
  | "chat.error";

// Extension → webview messages. Read-only display updates (event/overview),
// plus F4's chat.reply / chat.error responses. All security-critical
// mutation choices happen extension-side — the webview only sees the
// text of the reply and the summarize()d proposals.
export type PanelToWebviewMessage =
  | { kind: "event"; raw: string } // one JSONL line off adb events tail
  | { kind: "overview"; cards: OverviewCard[] }
  | {
      // Reply from `adb chat --message`. `text` is the full LLM reply
      // (already trimmed by observability.Chat); `proposals` are the
      // parsed-and-summarized SteerActions the webview renders as
      // buttons. Clicking a button posts back a chat.confirm message —
      // the extension then shows a MODAL confirm and only executes on
      // Apply. NEVER auto-executed.
      kind: "chat.reply";
      text: string;
      proposals: readonly ChatProposal[];
    }
  | { kind: "chat.error"; message: string };

// ChatProposal is the webview-safe view of a SteerAction — the ONLY thing
// crossing the postMessage boundary is a stable id (index in the reply's
// action list) + a human summary + whether it's executable at all. The
// full SteerAction stays on the extension side; this way a hostile
// webview cannot craft a payload that skips the allowlist.
export interface ChatProposal {
  id: number; // stable index into the extension-side action list
  summary: string; // pre-computed via chat.summarize(action)
  executable: boolean; // false → button is disabled (unknown/wiki.capture)
}

// Webview → extension messages. F3 = "hello" (read-only announce);
// F4 adds "chat.send" (user typed a message) and "chat.confirm" (user
// clicked Apply on a proposed action). Anything else is dropped by the
// extension's message handler.
export type WebviewToPanelMessage =
  | { kind: "hello" }
  | { kind: "chat.send"; message: string }
  | { kind: "chat.confirm"; proposalId: number };

// ---- Nonce ---------------------------------------------------------------
// Alphanumeric nonce ≥16 chars. Not cryptographically strong (Math.random),
// but it never leaves the local process and only has to be unpredictable
// enough to survive CSP nonce-checking within a single panel lifetime — the
// standard vscode-webview idiom.
export function makeNonce(): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let out = "";
  for (let i = 0; i < 32; i++) {
    out += alphabet[Math.floor(Math.random() * alphabet.length)];
  }
  return out;
}

// ---- HTML scaffold -------------------------------------------------------
// Static HTML the webview loads. All styling/scripting is nonce'd; the
// script wires up window.addEventListener('message', …) and postMessage —
// no other side effects. We keep the JS deliberately tiny and inline (as a
// nonce'd <script>) so we don't have to ship a bundle or resolve a webview
// resource URI here — the pure scaffold has zero vscode dependencies.
export function renderPanelHtml(nonce: string): string {
  const csp = [
    "default-src 'none'",
    `style-src 'nonce-${nonce}'`,
    `script-src 'nonce-${nonce}'`,
    "img-src data:",
    "font-src data:",
  ].join("; ");
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>AI Dev Brain — Dashboard</title>
<style nonce="${nonce}">
  body { font-family: var(--vscode-font-family, sans-serif); color: var(--vscode-foreground); background: var(--vscode-editor-background); margin: 0; padding: 12px; }
  h2 { font-size: 13px; text-transform: uppercase; letter-spacing: 0.05em; margin: 0 0 8px 0; opacity: 0.7; }
  section { margin-bottom: 20px; }
  #overview { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 8px; }
  .card { border: 1px solid var(--vscode-panel-border, #444); border-radius: 4px; padding: 8px 10px; }
  .card .org { font-weight: 600; margin-bottom: 4px; }
  .card .total { font-size: 12px; opacity: 0.7; margin-bottom: 6px; }
  .card .row { display: flex; justify-content: space-between; font-size: 12px; padding: 2px 0; }
  .card .row.zero { opacity: 0.4; }
  #feed { list-style: none; padding: 0; margin: 0; max-height: 60vh; overflow-y: auto; border: 1px solid var(--vscode-panel-border, #444); border-radius: 4px; }
  #feed li { padding: 4px 10px; border-bottom: 1px solid var(--vscode-panel-border, #333); font-size: 12px; font-family: var(--vscode-editor-font-family, monospace); display: flex; gap: 8px; align-items: baseline; }
  #feed li:last-child { border-bottom: none; }
  #feed .ts { opacity: 0.5; white-space: nowrap; }
  #feed .type { opacity: 0.7; min-width: 12ch; }
  #feed .taskid { color: var(--vscode-textLink-foreground, #4af); min-width: 12ch; }
  .empty { padding: 12px; opacity: 0.5; font-style: italic; }
  /* Chat region (F4). All controls are keyboard-focusable; buttons are the
     ONLY way to trigger a proposed steer action, and every click funnels
     through a modal confirm on the extension side. */
  #chat { display: flex; flex-direction: column; gap: 8px; }
  #chat-log { border: 1px solid var(--vscode-panel-border, #444); border-radius: 4px; padding: 8px; min-height: 60px; max-height: 40vh; overflow-y: auto; font-size: 12px; white-space: pre-wrap; }
  #chat-log .turn { margin-bottom: 8px; }
  #chat-log .turn .who { font-weight: 600; opacity: 0.7; }
  #chat-log .proposals { display: flex; flex-direction: column; gap: 4px; margin-top: 4px; }
  #chat-log .proposals button { text-align: left; padding: 4px 8px; background: var(--vscode-button-secondaryBackground, #333); color: var(--vscode-button-secondaryForeground, #eee); border: 1px solid var(--vscode-panel-border, #555); border-radius: 3px; cursor: pointer; font-size: 12px; }
  #chat-log .proposals button:hover:not(:disabled) { background: var(--vscode-button-secondaryHoverBackground, #444); }
  #chat-log .proposals button:disabled { opacity: 0.5; cursor: not-allowed; }
  #chat-form { display: flex; gap: 6px; }
  #chat-input { flex: 1; padding: 6px 8px; background: var(--vscode-input-background, #333); color: var(--vscode-input-foreground, #eee); border: 1px solid var(--vscode-input-border, #555); border-radius: 3px; font-family: inherit; font-size: 12px; }
  #chat-send { padding: 6px 14px; background: var(--vscode-button-background, #06c); color: var(--vscode-button-foreground, #fff); border: none; border-radius: 3px; cursor: pointer; }
  #chat-send:disabled { opacity: 0.5; cursor: not-allowed; }
  #chat-error { color: var(--vscode-errorForeground, #f66); font-size: 12px; padding: 4px 8px; display: none; }
</style>
</head>
<body>
<section>
  <h2>Overview</h2>
  <div id="overview" role="region" aria-label="Per-org overview cards">
    <div class="empty">Loading…</div>
  </div>
</section>
<section>
  <h2>Event Feed</h2>
  <ul id="feed" role="log" aria-live="polite" aria-label="Real-time adb event feed">
    <li class="empty">Waiting for events…</li>
  </ul>
</section>
<section>
  <h2>Chat</h2>
  <div id="chat" role="region" aria-label="ADB orchestrator chat">
    <div id="chat-log" role="log" aria-live="polite" aria-label="Chat transcript">
      <div class="empty">Ask ADB about your workspace. Every proposed change requires an explicit Apply confirmation.</div>
    </div>
    <div id="chat-error" role="alert"></div>
    <form id="chat-form" aria-label="Send chat message">
      <input id="chat-input" type="text" autocomplete="off" placeholder="e.g. what should I look at next?" aria-label="Message" />
      <button id="chat-send" type="submit">Send</button>
    </form>
  </div>
</section>
<script nonce="${nonce}">
  (function () {
    const vscode = acquireVsCodeApi();
    const feedEl = document.getElementById('feed');
    const overviewEl = document.getElementById('overview');
    const MAX_FEED = 500;

    function fmtTs(iso) {
      if (!iso) { return ''; }
      // Best-effort HH:MM:SS from an ISO string.
      const m = iso.match(/T(\\d{2}:\\d{2}:\\d{2})/);
      return m ? m[1] : iso;
    }

    function renderFeedItem(item) {
      const li = document.createElement('li');
      const ts = document.createElement('span'); ts.className = 'ts'; ts.textContent = fmtTs(item.timestamp);
      const type = document.createElement('span'); type.className = 'type'; type.textContent = item.type;
      const tid = document.createElement('span'); tid.className = 'taskid'; tid.textContent = item.taskId || '';
      const summary = document.createElement('span'); summary.textContent = item.summary || '';
      li.appendChild(ts); li.appendChild(type); li.appendChild(tid); li.appendChild(summary);
      return li;
    }

    function prependFeed(item) {
      const empty = feedEl.querySelector('.empty');
      if (empty) { empty.remove(); }
      feedEl.insertBefore(renderFeedItem(item), feedEl.firstChild);
      while (feedEl.children.length > MAX_FEED) {
        feedEl.removeChild(feedEl.lastChild);
      }
    }

    function renderOverview(cards) {
      // Clear via safe DOM removal (avoid innerHTML for XSS-hygiene, even
      // though '' is inert — keeps the file's threat surface simple).
      while (overviewEl.firstChild) { overviewEl.removeChild(overviewEl.firstChild); }
      if (!cards || cards.length === 0) {
        const d = document.createElement('div'); d.className = 'empty'; d.textContent = 'No tasks';
        overviewEl.appendChild(d); return;
      }
      for (const c of cards) {
        const card = document.createElement('div'); card.className = 'card';
        const org = document.createElement('div'); org.className = 'org'; org.textContent = c.org;
        const total = document.createElement('div'); total.className = 'total'; total.textContent = c.total + ' tasks';
        card.appendChild(org); card.appendChild(total);
        for (const s of Object.keys(c.byStatus)) {
          const n = c.byStatus[s];
          const row = document.createElement('div');
          row.className = 'row' + (n === 0 ? ' zero' : '');
          const label = document.createElement('span'); label.textContent = s;
          const val = document.createElement('span'); val.textContent = String(n);
          row.appendChild(label); row.appendChild(val);
          card.appendChild(row);
        }
        overviewEl.appendChild(card);
      }
    }

    // We can't import feed.ts into the webview at runtime (no bundler), so
    // parseEventLine is inlined here. Its role is to survive garbage lines
    // and produce the same FeedItem shape the pure module produces — the
    // tests in feed.test.ts pin the semantics we mirror.
    function eventToFeedItem(ev) {
      const summarize = SUMMARY[ev.type];
      const raw = summarize ? summarize(ev.data || {}) : ev.type;
      const summary = String(raw).replace(/[\\r\\n]+/g, ' ').replace(/\\s+/g, ' ').trim() || ev.type;
      return {
        timestamp: ev.timestamp || '',
        type: ev.type,
        taskId: (ev.data && typeof ev.data.task_id === 'string') ? ev.data.task_id : undefined,
        summary,
      };
    }
    const SUMMARY = {
      'task.created': (d) => 'task created' + (typeof d.title === 'string' ? ' — ' + d.title : ''),
      'task.completed': () => 'task completed',
      'task.status_changed': (d) => 'status ' + (d.old_status || '?') + ' → ' + (d.new_status || '?'),
      'task.archived': () => 'task archived',
      'task.unarchived': () => 'task unarchived',
      'task.priority_changed': (d) => 'priority ' + (d.old_priority || '?') + ' → ' + (d.new_priority || '?'),
      'task.deleted': () => 'task deleted',
      'worktree.created': () => 'worktree created',
      'worktree.removed': () => 'worktree removed',
      'agent.session_started': () => 'agent session started',
      'agent.session_ended': () => 'agent session ended',
      'knowledge.extracted': () => 'knowledge extracted',
      'issue.synced': (d) => 'issue synced' + (typeof d.direction === 'string' ? ' (' + d.direction + ')' : ''),
      'issue.conflict': () => 'issue conflict',
      'issue.skipped': () => 'issue skipped',
    };

    // ---- Chat (F4) ------------------------------------------------------
    // The webview NEVER dispatches actions directly. Its role is:
    //   1. Send the user's typed message to the extension.
    //   2. Render the reply text.
    //   3. Render one button per proposed action (server-truncated summary
    //      the extension pre-computed via chat.summarize()).
    //   4. On button click: post {kind:'chat.confirm', proposalId} — the
    //      extension shows a MODAL confirm and only then executes.
    // Nothing about the action shape lives here; the extension keeps the
    // typed SteerAction and the id → action map. Even if this script were
    // compromised, it can only ask for actions by their sequential id, and
    // the extension re-runs the allowlist check before any mutation.
    const chatLogEl = document.getElementById('chat-log');
    const chatFormEl = document.getElementById('chat-form');
    const chatInputEl = document.getElementById('chat-input');
    const chatSendEl = document.getElementById('chat-send');
    const chatErrorEl = document.getElementById('chat-error');

    function appendUserTurn(text) {
      const empty = chatLogEl.querySelector('.empty');
      if (empty) { empty.remove(); }
      const div = document.createElement('div'); div.className = 'turn';
      const who = document.createElement('div'); who.className = 'who'; who.textContent = 'you';
      const body = document.createElement('div'); body.textContent = text;
      div.appendChild(who); div.appendChild(body);
      chatLogEl.appendChild(div);
      chatLogEl.scrollTop = chatLogEl.scrollHeight;
    }

    function appendReplyTurn(text, proposals) {
      const empty = chatLogEl.querySelector('.empty');
      if (empty) { empty.remove(); }
      const div = document.createElement('div'); div.className = 'turn';
      const who = document.createElement('div'); who.className = 'who'; who.textContent = 'adb';
      const body = document.createElement('div'); body.textContent = text;
      div.appendChild(who); div.appendChild(body);
      if (Array.isArray(proposals) && proposals.length > 0) {
        const list = document.createElement('div'); list.className = 'proposals';
        for (const p of proposals) {
          const btn = document.createElement('button');
          btn.type = 'button';
          btn.textContent = p.summary || '(no summary)';
          btn.disabled = !p.executable;
          btn.setAttribute('data-proposal-id', String(p.id));
          btn.addEventListener('click', function () {
            // Confirm gate is on the EXTENSION side (modal). We just ask.
            vscode.postMessage({ kind: 'chat.confirm', proposalId: Number(p.id) });
          });
          list.appendChild(btn);
        }
        div.appendChild(list);
      }
      chatLogEl.appendChild(div);
      chatLogEl.scrollTop = chatLogEl.scrollHeight;
    }

    function showChatError(text) {
      chatErrorEl.textContent = text;
      chatErrorEl.style.display = 'block';
    }
    function clearChatError() {
      chatErrorEl.textContent = '';
      chatErrorEl.style.display = 'none';
    }

    function setChatBusy(busy) {
      chatSendEl.disabled = busy;
      chatInputEl.disabled = busy;
    }

    chatFormEl.addEventListener('submit', function (ev) {
      ev.preventDefault();
      const text = (chatInputEl.value || '').trim();
      if (text.length === 0) { return; }
      clearChatError();
      appendUserTurn(text);
      chatInputEl.value = '';
      setChatBusy(true);
      vscode.postMessage({ kind: 'chat.send', message: text });
    });

    window.addEventListener('message', function (e) {
      const msg = e.data || {};
      if (msg.kind === 'event' && typeof msg.raw === 'string') {
        try {
          const ev = JSON.parse(msg.raw);
          if (ev && typeof ev.type === 'string') {
            prependFeed(eventToFeedItem(ev));
          }
        } catch (_e) { /* skip garbage lines */ }
      } else if (msg.kind === 'overview' && Array.isArray(msg.cards)) {
        renderOverview(msg.cards);
      } else if (msg.kind === 'chat.reply' && typeof msg.text === 'string') {
        setChatBusy(false);
        appendReplyTurn(msg.text, Array.isArray(msg.proposals) ? msg.proposals : []);
      } else if (msg.kind === 'chat.error' && typeof msg.message === 'string') {
        setChatBusy(false);
        showChatError(msg.message);
      }
    });

    // Announce readiness; the extension replies with the initial overview.
    vscode.postMessage({ kind: 'hello' });
  }());
</script>
</body>
</html>`;
}
