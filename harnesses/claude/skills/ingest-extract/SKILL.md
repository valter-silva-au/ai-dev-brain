---
name: ingest-extract
description: >-
  Extract typed graph entities and edges from a landed raw source artifact and
  emit them as adb ingestion proposals. Use after `adb ingest land` has landed a
  source (Slack thread, email, meeting notes, article, PR discussion) and you
  want to propose the stakeholders, systems, decisions, and relationships it
  mentions into the workspace graph. Pairs with the staged-ingestion pipeline
  (decision D8): proposals are confidence-gated — the certain auto-land, the
  fuzzy queue for human review.
---

# Ingestion extraction

`adb` ingests knowledge in **stages** so nothing derived is trusted blindly
(decision D8):

1. **Land** — a connector writes immutable raw content under `raw/` with
   provenance (source, content hash, cursor). You do not do this; `adb ingest
   land` does.
2. **Extract** — *this skill*: read a raw artifact and PROPOSE the typed nodes
   and edges it implies, each with a **confidence** and a **provenance** link
   back to the raw artifact.
3. **Gate** — `adb ingest propose` routes proposals: confidence **≥ threshold**
   (default 0.8) auto-lands into the graph; the rest go to a **review queue**.
4. **Review** — a human runs `adb ingest review` / `accept` / `reject`.

Your job is step 2 only: turn one raw artifact into a `proposals.yaml`.

## The graph you are proposing into

Entities are typed nodes; relationships are typed edges (the closed edge
vocabulary is `relates_to`, `part_of`, `blocks`, `depends_on`, `duplicates`).
Existing entities include tasks (`TASK-00001`) and initiatives. You may also
propose **new typed nodes** — a `stakeholder`, `system`, `dataset`, `decision` —
which land into the ingested-node registry and join the graph.

## How to extract

1. Find the raw artifact and its id: `adb ingest raw` (the provenance ledger).
2. Read its content at the listed `CONTENT` path.
3. Identify the entities and relationships it states. For each, decide a
   **confidence** in `[0,1]`:
   - **≥ 0.8** — explicit and unambiguous (auto-lands). e.g. the text plainly
     says "TASK-00042 is blocked by TASK-00043".
   - **< 0.8** — inferred, fuzzy, or a possible duplicate (queues for review).
4. Set `raw_id` on **every** proposal to the artifact's id — this is the
   provenance the whole pipeline depends on.

## Emit `proposals.yaml`

```yaml
proposals:
  # An edge between two EXISTING entities (lands onto `from`'s frontmatter).
  - raw_id: <the raw artifact id>
    kind: edge
    confidence: 0.9
    from: TASK-00042
    edge: { type: depends_on, target: TASK-00043 }

  # A NEW typed node (lands into the ingested-node registry, joins the graph).
  - raw_id: <the raw artifact id>
    kind: node
    confidence: 0.6            # fuzzy → will queue for review
    node:
      id: STK-acme
      type: stakeholder
      title: Acme Corp (raised the retention concern)
      links:
        - { type: relates_to, target: INIT-onboarding }
```

Then submit:

```bash
adb ingest propose --file proposals.yaml            # gate at the default 0.8
adb ingest propose --file proposals.yaml --threshold 0.9   # stricter auto-land
```

## Rules

- **Never invent entity ids.** Reference real task/initiative ids you can see in
  the workspace; for new nodes, mint a stable, human-readable id.
- **Provenance is mandatory.** Every proposal carries `raw_id`; a proposal
  without it is rejected at submit.
- **Be honest about confidence.** A duplicate you are unsure about, or a
  relationship you inferred rather than read, belongs **below** the threshold so
  a human reviews it. The gate is the safety net — use it.
- **Do not accept your own proposals.** Extraction proposes; a human (or a
  high-confidence auto-land) decides. `adb ingest accept/reject` is the reviewer's
  step, not yours.
