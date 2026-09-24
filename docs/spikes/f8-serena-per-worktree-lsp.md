# Spike (F8, #212): per-worktree Serena/LSP resource cost + activation strategy

> **Status:** complete — gates the F8 code (below). **Decision:** adb emits the
> worktree path + an activation hint only; it does **not** manage, pre-warm, or
> fan out Serena. Instance-per-project bounds cost to *active sessions*, not to
> the total worktree count.

## The question (the one the #198 evidence set did not answer)

adb creates a git worktree per task. If code-nav (Serena → a language server per
project) activates *per worktree*, what does that cost when many worktrees exist,
and what activation strategy keeps it bounded? Measure/reason before committing
adb to any Serena-management behaviour.

## What Serena's model actually is

Serena is **instance-per-project**: the MCP *client* (Claude Code) spawns one
Serena stdio process per session, and Serena activates the **nearest**
`.serena/project.yml` walking up from the launch cwd. Because #202 provisions a
`.serena/project.yml` inside every code worktree, a session launched with
`cwd = <worktree>` (which is exactly what `adb task resume` / Start-All do)
activates *that worktree* as its own project — isolated symbols, no cross-worktree
duplication. adb does not (and should not) spawn Serena itself.

Each activated project starts the language server(s) for its detected languages
(#201): `gopls` for Go, `typescript-language-server` for TS, etc.

## The cost, reasoned

The load-bearing cost is the **language server**, not Serena's thin adapter:

- `gopls` on a moderate Go module resident-sets in the ~hundreds of MB range and
  climbs with module + dependency size; first activation also pays an indexing
  cost (seconds).
- `typescript-language-server` is comparable on a non-trivial TS project.

Crucially, cost scales with **concurrently-active projects**, i.e. **open
sessions**, not with how many worktrees exist on disk. Ten worktrees that are not
open cost **zero** LSP memory — no session, no Serena, no server. A developer
working interactively is almost always in **one** worktree at a time, so the
steady-state cost is ~one project's language servers.

### The real risk: Start-All fan-out

The one place worktree count and *active* sessions converge is **Start-All**
(L200): it opens one terminal per launchable ticket, each a session with
`cwd = <worktree>`. If Serena is attached in every one, N tickets ⇒ N Serena
instances ⇒ N × (gopls + tsserver). That is the scenario to bound.

Mitigations, in order of preference:

1. **Don't auto-attach Serena in every Start-All terminal** — attach it in the
   worktree you actually drive. (Serena/MCP registration is the *client's* choice;
   adb does not force it on.)
2. **Cap Start-All** — the extension already exposes `adb.startAll.cap`
   (L200), which directly bounds concurrent sessions and therefore LSP instances.
3. **Lazy indexing** — gopls/tsserver index on first symbol request, so an open
   session that never asks for a symbol pays little beyond process startup.

## Decision (gates the code)

- adb **emits the active worktree path + an activation hint** on launch/dispatch
  and otherwise stays out of Serena's way. It does **not** manage, pre-warm, or
  fan Serena out across worktrees — doing so would multiply LSP cost by the
  worktree count for no benefit, since only active sessions need code-nav.
- Isolation + activation are already delivered by #202's per-worktree
  `.serena/project.yml` (instance-per-project). F8's code contribution is
  therefore intentionally small: surface the path + hint so a human/agent knows
  code-nav follows the ticket.
- **Document the Start-All caveat** (above) so an operator running a wide
  fan-out knows to lean on `adb.startAll.cap` or selective attachment rather than
  attaching Serena to every terminal.

## What F8 ships alongside this report

`adb task resume` (and the launch/dispatch path) prints a one-line Serena
activation hint naming the worktree — "this worktree is its own project
(instance-per-project); code-nav will activate `<path>` via its
`.serena/project.yml`." No new event type, no Serena process management — path +
hint only, per the decision above.
