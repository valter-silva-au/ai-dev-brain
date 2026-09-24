---
name: stage-gate
description: >-
  Operate the adb Founder-Playbook stage lifecycle: create organizations and
  initiatives, gather the evidence a StageGate requires, and advance an initiative
  Idea->MVP->Launch behind its gate. Use when a user asks to move a business
  initiative between founder-playbook stages, to check why an advance is blocked,
  or to assemble the evidence a gate needs. Pairs with the devils-advocate agent,
  which supplies the adversarial verdict a gate's judgment items require.
---

# Stage gates

`adb` models a startup lifecycle **above** the dev-task lifecycle. Understand the
two axes before touching a gate:

- **TaskStatus** (`backlog -> in_progress -> review -> done -> archived`) tracks a
  single ticket.
- **Stage** (`Idea -> MVP -> Launch -> Scale`) lives on an **Initiative** and is
  orthogonal to TaskStatus. A gate guards each stage transition.

Hierarchy: **Workspace -> Organization (business) -> Initiative (carries Stage) ->
Ticket**. Organizations and initiatives are workspace metadata
(`orgs/index.yaml`, `initiatives/index.yaml`) — they are *not* part of the
`tickets/<platform>/<org>/<repo>` path layout.

## The commands

```bash
adb org create "<name>" [--git-host github.com]   # register a business
adb org list | adb org show <org-id>

adb initiative create "<name>" --org <org-id>      # defaults to the Idea stage
adb initiative list | adb initiative show <id> [--json]
adb initiative set-stage <id> <Idea|MVP|Launch|Scale>   # set directly (no gate)

adb stage advance <initiative-id>                  # advance behind the gate
adb stage advance <initiative-id> --override --reason "<why>"   # human-only bypass
```

IDs are the slug of the name (`"Widget Launcher"` -> `widget-launcher`).

## How a gate works

`adb stage advance` evaluates the gate for the initiative's **current** stage:

- **Deterministic file items** — a named evidence file must exist and be
  non-empty under `initiatives/<initiative-id>/evidence/`. Any unmet item
  **blocks** the advance (non-zero exit) and is listed by name.
- **Metric items** — a numeric threshold read from a recorded `adb pmf` metric
  node (NOT a file). The MVP->Launch gate is entirely metric-based: it requires
  `sean-ellis >= 40` and `retention >= 40`, recorded with `adb pmf record`
  (below). A missing or below-threshold metric **blocks** the advance — dropping
  a `sean-ellis-survey.md` file does NOT satisfy it.
- **Judgment items** — an adversarial verdict (see the devils-advocate agent).
  The verdict is supplied by **recording the agent's output** as
  `initiatives/<initiative-id>/evidence/<item-id>.verdict.md` (item ids:
  `problem-validation` for Idea->MVP, `launch-readiness` for MVP->Launch). The
  gate parses the agent's `VERDICT: pass|fail` line: `pass` satisfies the item,
  `fail` **blocks** the advance like an unmet deterministic item, and no recorded
  verdict degrades to `pending` (never blocks).

On a clean pass the initiative moves to the next stage and the gate result is
recorded durably on the initiative. A pass or an override emits a
`stage.advanced` event; an override additionally emits `stage.override` (readable
via `adb events query --type stage.advanced`). Only a human may `--override`, and
only with a `--reason`.

## The evidence each gate wants

Each gate requires either **files** (dropped into
`initiatives/<initiative-id>/evidence/`) or **metrics** (recorded with
`adb pmf record`), plus a devils-advocate verdict:

| Transition | Required evidence | Judgment (devils-advocate) |
|------------|-------------------|----------------------------|
| **Idea -> MVP** | files: `problem-statement.md`, `target-customer.md` | is the problem real and worth solving? |
| **MVP -> Launch** | metrics: `sean-ellis >= 40` and `retention >= 40` (via `adb pmf record`) | is product-market fit real, not a false positive? |

> **MVP -> Launch is metric-only — there are no evidence files for it.** Record
> the numbers instead:
>
> ```bash
> adb pmf record --initiative <id> --metric sean-ellis --value 48 --source survey
> adb pmf record --initiative <id> --metric retention  --value 45 --source cohort
> ```
>
> Dropping a `sean-ellis-survey.md` file will NOT advance the gate; the gate reads
> the recorded metric nodes, not the evidence dir, for this transition.

For the file-based Idea->MVP items, write evidence that would survive scrutiny:
past-tense behaviour over hypotheticals, a nameable customer segment over "SMBs",
a real activated cohort over warm intros. Scaffold the Idea->MVP worksheets with
`adb initiative scaffold-evidence <id>`.

## The loop

1. `adb initiative show <id>` — confirm the current stage.
2. Provide the transition's required evidence: for **Idea->MVP**, drop the
   evidence files into the evidence dir; for **MVP->Launch**, record the metrics
   with `adb pmf record` (there are no files for it).
3. Run the **devils-advocate** agent on the evidence, then save its output to
   `initiatives/<id>/evidence/<item-id>.verdict.md` so the gate can read the
   verdict. If it returns `fail`, fix the gaps it names before advancing — do not
   reach for `--override` to dodge a real weakness.
4. `adb stage advance <id>`. If blocked, it prints exactly what is missing (unmet
   evidence or a failed verdict); add/fix it and retry.
5. Reserve `--override --reason "..."` for cases a human has genuinely validated
   off-system; the reason is logged on the gate and in the event stream.
