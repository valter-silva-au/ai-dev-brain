# L300 — Integrations: connecting adb to remotes and the cloud

> **Tier:** L300 (advanced) · **Prereqs:** [L100 — Fundamentals](./L100-fundamentals.md), [L200 — Daily Workflows](./L200-daily-workflows.md)
>
> This is the "I connect adb to the outside world" tier. By now you can create, start, and close tasks locally. Here you learn the three ways adb reaches beyond your workspace:
>
> 1. **[Issue sync](#1-issue-sync)** — mirror adb tickets to/from GitHub & GitLab issues (`adb sync issues`).
> 2. **[Cloud sync](#2-cloud-sync)** — back the allowlisted knowledge base up to S3, with a fail-closed secret scan (`adb sync cloud`).
> 3. **[The MCP server](#3-the-mcp-server)** — expose adb's task lifecycle to any MCP client, including Claude Code itself (`adb mcp serve`).
>
> Each section is honest about its **gates** — the external things that must be true before it works. Nothing here auto-provisions cloud infrastructure or stores a credential; adb rides *your* existing logins (`gh`, `glab`, the AWS profile chain).

---

## 1. Issue sync

`adb sync issues` reconciles each adb ticket with a remote GitHub or GitLab issue. It is **bidirectional and last-writer-wins (LWW)** over a fixed set of fields, and it is the first place adb ever shells out to `gh`/`glab`.

### Command

```
adb sync issues [--repo <platform/org/repo>] [--dry-run] [--direction both|push|pull]
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--repo <platform/org/repo>` | *(all tickets)* | Limit the sync to one platform-qualified repo, e.g. `github.com/valter-silva-au/ai-dev-brain`. Matched exactly against each ticket's `repo:`. |
| `--dry-run` | `false` | Print the reconcile plan without writing anything (locally or remotely). |
| `--direction both\|push\|pull` | `both` | Which sides may be written. `push` = local → remote only; `pull` = remote → local only; `both` = full LWW. An invalid value errors. |

**Anchor:** `internal/cli/sync_issues.go:newSyncIssuesCmd` · engine `internal/integration/issuesync/reconcile.go:Reconcile`.

### What actually gets synced

The synced-field allowlist is **exactly**: `title`, `body`, `labels` (derived from status + priority), `status`, `priority`. Everything else — owner, tags outside labels, timestamps, file paths, and the remote-linkage fields themselves — is **never** overwritten by a sync.

- **Status ↔ issue state:** adb `done`/`archived` → issue **CLOSED**; `backlog`/`in_progress`/`blocked`/`review` → issue **OPEN** plus an `adb:<status>` label. That label lets a closed→open toggle restore the exact fine-grained status on the next pull. On pull, a CLOSED issue always maps back to `done`; an open issue with no `adb:` label defaults to `in_progress`.
- **Priority:** exported as an informational `priority:<P>` label only. Pull **never** rewrites your local priority from it.
- **Change detection** uses a stored per-sync baseline hash (`Task.SyncHash`, a sha256 of the synced-field snapshot), **not** the `Updated` timestamp — because a plain `save` bumps `Updated` even when no synced field changed. Anchor: `internal/integration/issuesync/reconcile.go:SyncHash`.

### Auth — nothing lives in adb

Authentication is **per-host and owned by your `gh` / `glab` CLI login.** This path:

- never reads `~/.config/gh/hosts.yml`,
- never accepts a `--token` flag,
- never writes a token or any PII into `backlog.yaml`, `status.yaml`, or `.events.jsonl`.

This is enforced by tests (`internal/integration/issuesync` — `TestSyncer_EventPayloadHasNoCredentials` and argv-boundary tests that forbid `--token`, `hosts.yml`, `GITHUB_TOKEN`, `GITLAB_TOKEN`, `PRIVATE-TOKEN`, …). The provider simply shells out to `gh issue view/create/edit/close/reopen` and `glab issue view/create/update/close/reopen`.

### What gets skipped (and why)

A ticket only syncs if its `repo:` is a real, platform-qualified GitHub/GitLab triple. The skip filter (`internal/integration/issuesync/select.go:ProviderFor`) silently skips — and logs `issue.skipped` for:

- repo-less `_local` tickets,
- absolute or relative local-path repos,
- anything that isn't a 3-part `host/org/repo`,
- **Enterprise-internal hosts** (any remote that isn't `github.com` or a `gitlab` host).

### Examples

```bash
# See what a full bidirectional sync would do — writes nothing:
adb sync issues --dry-run

# Push local ticket state up to GitHub/GitLab only (never pull remote edits):
adb sync issues --direction push

# Reconcile just one repo, both directions:
adb sync issues --repo github.com/valter-silva-au/ai-dev-brain
```

### Honest limits

- **`gh`/`glab` must be installed *and* authenticated on this host** — auth is entirely external to adb.
- **`pull` is not loss-safe.** `remoteChanged` is a timestamp heuristic (`Remote.UpdatedAt.After(LocalUpdated)`). A maintainer's non-synced `updated_at` bump (edit-then-revert a body) can trigger a pull that discards a local edit — logged as `issue.conflict`. This is a documented LWW-by-timestamp limitation until a remote-hash baseline lands.
- **GitHub label writes are additive** (`--add-label` only, no `--remove-label`). Because `adb:`/`priority:` labels change *keys* rather than accumulate, this is fine in practice, but the remote label set is **not** fully reconciled.

---

## 2. Cloud sync

`adb sync cloud` ships the **allowlisted** parts of your knowledge base to a versioned S3 bucket and pulls them back. It exists so your KB survives a lost laptop — not as a sharing or collaboration channel.

> ### Gate: the bucket is a prerequisite, not something adb creates
>
> **adb does not ship a CloudFormation/CDK stack, and it does not provision a bucket, KMS key, or IAM policy.** PR #78 shipped **only the Go S3 archive engine.** You must supply a real AWS account and a **pre-existing S3 bucket** yourself.
>
> The client relies on the bucket's **default encryption** for at-rest protection — `s3client.go` deliberately does **not** set `ServerSideEncryption`/`SSEKMSKeyId` on `PutObject`, keeping the client IAM policy minimal (anchor: `internal/integration/cloudsync/s3client.go`). Provisioning that bucket with default SSE-KMS, versioning, and a scoped IAM policy is an **external / manual** step you own. Code comments referencing "set by CDK" and "run `cdk destroy` after" describe *your* out-of-band provisioning, not anything in this repo.

### Commands

```
adb sync cloud push   [--bucket <name>] [--region <r>] [--dry-run]
adb sync cloud pull    --dest <dir> [--bucket <name>] [--region <r>]
adb sync cloud status  [--bucket <name>] [--region <r>]
adb sync cloud destroy --confirm [--bucket <name>] [--region <r>]
```

**Anchor:** `internal/cli/sync_cloud.go:newSyncCloudCmd`.

| Subcommand | Required flag | Notes |
|------------|---------------|-------|
| `push` | *(bucket, unless `--dry-run`)* | Stages the upload set, runs gitleaks fail-closed, then uploads. `--dry-run` prints the plan and runs **neither** the scanner **nor** any S3 call. |
| `pull` | `--dest` | `--dest` must be a **fresh directory**; restores the full key hierarchy into it. |
| `status` | *(bucket)* | Reports remote object count vs. local upload-set count. |
| `destroy` | `--confirm` | **Empties the bucket's objects only** (double-gated: CLI `--confirm` *and* the engine's `confirm=true`). It does **not** tear down the bucket — that's your external `cdk destroy` / console action. |

**Bucket + region resolution order:** `--bucket`/`--region` flags → `ADB_CLOUD_BUCKET`/`ADB_CLOUD_REGION` env → region default **`ap-southeast-2`** (`const defaultCloudRegion`, `internal/cli/sync_cloud.go`). A bucket name is required for everything **except** `push --dry-run`. AWS credentials come from the standard profile chain (env / shared config / IMDS) — nothing is embedded, logged, or persisted.

### What gets archived — the deny-first allowlist

The allowlist is the **security boundary**, and it is **fail-closed**: deny-first, then a strict include-root allowlist, so a brand-new top-level directory is *never* uploaded by accident. It is a deliberate, code-reviewed subset — **not** parsed from `.gitignore` at runtime (a parse bug there could silently *widen* what ships). Anchor: `internal/integration/cloudsync/allowlist.go:ShouldUpload`.

**Include roots (only these trees are eligible):** `raw/`, `scripts/`, `skills/`, `wiki/`, `tickets/`.

**Include root files (individually eligible at the workspace root):** `CLAUDE.md`, `Taskfile.yaml`, `WIKI.md`, `.markdownlint-cli2.yaml`, `.gitleaks.toml`, `.pre-commit-config.yaml`, `.gitignore`.

**Denied segments (a hit *anywhere* in the path is a hard NO):** `backlog.yaml`, `.task_counter`, `.session_counter`, `.adb`, `.adb_memory.sqlite`(+`-shm`/`-wal`), `.adb_mcp_cache.json`, `.adb_session_changes`, `.taskrc`, `.taskconfig`, `.adb-workspace-README.md`, `.events.jsonl`, `work`, `repos`, `.omnictx`, `communications`, `sessions`, and `.env` / `.env.*`.

> **Important:** even though `tickets/` is an include root, `tickets/**/communications/` (Slack/PR correspondence) and any `sessions/` transcripts are **denied** — and that denial is hard-coded here, *not* carried by `.gitignore`. If you document "what gets archived", state plainly that per-ticket correspondence and session transcripts are excluded.

### The fail-closed gitleaks gate

A real (non-dry-run) `push` runs a **fail-closed** secret scan over the *staging copy* before any object is uploaded:

```
gitleaks detect --no-git --source <staging> --redact --no-banner
```

Semantics (`internal/integration/cloudsync/gitleaks.go:ScanForSecretsWith`):

- **exit 0** → clean, proceed.
- **exit 1** → a finding → **abort**, no upload.
- **anything else** (exit 2+, non-nil start error, **or a missing gitleaks binary**) → treated as a **security failure** and aborts.

So a missing `gitleaks` on `PATH` does not "skip" the scan — it **blocks the push**. This is a second gate on top of the AWS account/bucket requirement.

Defence-in-depth beyond the scan: the walker prunes denied dirs and skips symlinks; `stageFile` `os.Lstat`s each entry and refuses anything that isn't a regular file; `pull` rejects absolute S3 keys and any key whose cleaned target would escape `--dest`. Anchors: `internal/integration/cloudsync/walk.go`, `internal/integration/cloudsync/sync.go`.

### Examples

```bash
# Dry run — see the exact upload set with no AWS call and no scan:
adb sync cloud push --dry-run

# Real push (bucket via env), which runs gitleaks fail-closed first:
export ADB_CLOUD_BUCKET=my-adb-archive        # region defaults to ap-southeast-2
adb sync cloud push

# Compare remote vs local object counts:
adb sync cloud status --bucket my-adb-archive

# Restore into a fresh directory:
adb sync cloud pull --bucket my-adb-archive --dest ~/adb-restore

# Empty the bucket's objects (the bucket itself stays):
adb sync cloud destroy --bucket my-adb-archive --confirm
```

### Honest gates (recap)

1. A real **AWS account + a pre-provisioned S3 bucket** (no auto-create). adb ships no infra.
2. **`gitleaks` on `PATH`** — missing binary fails *closed* and aborts a real push.
3. A green **`--dry-run` does NOT prove the push will pass** — dry-run runs neither gitleaks nor S3.
4. `push` uploads a `repos-manifest.tsv` (columns `path`, `origin`, `head`, `branch`) so you can reconstruct which repos existed, but it does **not** archive the repos themselves (`repos/` is denied).

> **Events note:** cloud sync emits `cloud.sync_pushed`/`pulled`/`status`/`destroyed`, but those four are declared *locally* and are **not** in the canonical observability schema (`KnownEventTypes`). So `adb events query` sees them, but tooling that treats `KnownEventTypes` as the complete allowlist will handle them differently from the registered `issue.*` events. See [L400 — Architecture & Extending](./L400-architecture-and-extending.md).

---

## 3. The MCP server

`adb mcp serve` runs adb as a **Model Context Protocol server over stdio**, so an MCP client (Claude Code, another agent) can drive adb's task lifecycle as tools.

> **This is the *only* `serve` verb.** There is no `adb serve` web dashboard — that command and its web UI are gone. The terminal dashboard is `adb dashboard` (a Bubbletea TUI); the in-editor dashboard is the VS Code extension's webview. `adb mcp serve` is unrelated to both.

### Command

```
adb mcp serve            # MCP server over stdio (no flags)
adb mcp check [--no-cache]   # validate configured MCP servers' health
```

**Anchors:** `internal/cli/mcp_serve.go:newMCPServeCmd` (wired under `adb mcp` via `internal/cli/team.go:NewMCPCmd`) → `internal/mcpserver/server.go:Serve`.

### The tools it exposes

`registerTaskTools` exposes seven task-lifecycle tools and `registerGraphTools` adds four graph/knowledge tools (`internal/mcpserver/server.go` + `graph_tools.go`), each delegating to the same `App` subsystems the CLI uses:

| Tool | Maps to |
|------|---------|
| `adb_task_list` | list/filter tasks |
| `adb_task_create` | create a task |
| `adb_task_start` | promote backlog → in_progress |
| `adb_task_close` | mark done |
| `adb_task_update` | change status/priority/owner |
| `adb_task_start_all` | promote every backlog task |
| `adb_task_close_all` | mark every active task done |
| `graph_neighbors` | edges incident to an entity (`--type` filter) — `App.GraphManager` |
| `related_tickets` | backlog tickets linked to a ticket (type + direction) — GraphManager + BacklogManager |
| `get_initiative` | an initiative's stage + gate state — `App.StageManager` |
| `search_knowledge` | semantic search over vector memory — `App.OpenMemoryStore`; degrades to a clear notice when memory is unconfigured |

`adb_task_create`'s `type` goes through `parseTaskType`, which enforces the full `ValidTaskTypes` set (the 8 Conventional code types + the non-code `work`/`prototype`) and **rejects the retired `bug` alias** with `task type "bug" is retired; use \`fix\` instead`. The server exposes **no** issue-sync or cloud-sync tools — those stay CLI-only.

### Registering it with a client

Because the server's launch cwd is unpredictable, set `ADB_HOME` in the registration so adb resolves the right workspace:

```json
{
  "mcpServers": {
    "adb": {
      "command": "adb",
      "args": ["mcp", "serve"],
      "env": { "ADB_HOME": "/Users/you/Code/workspace" }
    }
  }
}
```

`adb mcp check` validates the MCP servers configured in your `claude_desktop_config.json` (use `--no-cache` to bypass the health cache).

---

## Where to go next

- **[L100 — Fundamentals](./L100-fundamentals.md)** — install, `adb init workspace`, your first task.
- **[L200 — Daily Workflows](./L200-daily-workflows.md)** — the create → resume → close loop, start-all/close-all, the VS Code extension.
- **[L400 — Architecture & Extending](./L400-architecture-and-extending.md)** — the observability event pipeline (`adb events`, `adb metrics`, `adb alerts`), and why `cloud.*` events sit outside the canonical schema.

### One-line honesty checklist for this tier

- Issue sync needs an authenticated `gh`/`glab`; adb never holds a token.
- Cloud sync needs *your* AWS bucket (adb ships no infra) **and** `gitleaks` on `PATH`; a real push aborts on any secret finding.
- The only `serve` is `adb mcp serve` (stdio MCP) — there is no web dashboard.
