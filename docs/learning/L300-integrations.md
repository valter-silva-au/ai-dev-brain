# L300 — Integrations: connecting adb to remotes and the cloud

> **Tier:** L300 (advanced) · **Prereqs:** [L100 — Fundamentals](./L100-fundamentals.md), [L200 — Daily Workflows](./L200-daily-workflows.md)
>
> This is the "I connect adb to the outside world" tier. By now you can create, start, and close tasks locally. Here you learn the three ways adb reaches beyond your workspace:
>
> 1. **[Issue sync](#1-issue-sync-adb-issues-sync)** — mirror adb tickets to/from GitHub & GitLab issues (`adb issues sync`).
> 2. **[Archive](#2-archive-adb-archive)** — back the allowlisted knowledge base up to S3, with a fail-closed secret scan (`adb archive`).
> 3. **[The MCP server](#3-the-mcp-server)** — expose adb's task lifecycle to any MCP client, including Claude Code itself (`adb mcp serve`), and audit the MCP servers this machine already has configured (`adb mcp check`).
>
> Each section is honest about its **gates** — the external things that must be true before it works. Nothing here auto-provisions cloud infrastructure or stores a credential; adb rides *your* existing logins (`gh`, `glab`, the AWS profile chain).

---

## 1. Issue sync (`adb issues sync`)

`adb issues sync` reconciles each adb ticket with a remote GitHub or GitLab issue. It is **bidirectional and last-writer-wins (LWW)** over a fixed set of fields, and it is the first place adb ever shells out to `gh`/`glab`.

### Command

```
adb issues sync [--repo <platform/org/repo>] [--dry-run] [--direction both|push|pull]
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
- never writes a token or any PII into `backlog.yaml`, `status.yaml`, or `.adb/events.jsonl`.

This is enforced by tests (`internal/integration/issuesync` — `TestSyncer_EventPayloadHasNoCredentials` and argv-boundary tests that forbid `--token`, `hosts.yml`, `GITHUB_TOKEN`, `GITLAB_TOKEN`, `PRIVATE-TOKEN`, …). The provider simply shells out to `gh issue view/create/edit/close/reopen` and `glab issue view/create/update/close/reopen`.

### What gets skipped (and why)

A ticket only syncs if its `repo:` is a real, platform-qualified GitHub/GitLab triple. The skip filter (`internal/integration/issuesync/select.go:ProviderFor`) silently skips — and logs `issue.skipped` for:

- repo-less `_local` tickets,
- absolute or relative local-path repos,
- anything that isn't a 3-part `host/org/repo`,
- **Amazon-internal hosts** (`code.aws.dev`, `code.amazon.com`).

### Examples

```bash
# See what a full bidirectional sync would do — writes nothing:
adb issues sync --dry-run

# Push local ticket state up to GitHub/GitLab only (never pull remote edits):
adb issues sync --direction push

# Reconcile just one repo, both directions:
adb issues sync --repo github.com/valter-silva-au/ai-dev-brain
```

### Honest limits

- **`gh`/`glab` must be installed *and* authenticated on this host** — auth is entirely external to adb.
- **`pull` is not loss-safe.** `remoteChanged` is a timestamp heuristic (`Remote.UpdatedAt.After(LocalUpdated)`). A maintainer's non-synced `updated_at` bump (edit-then-revert a body) can trigger a pull that discards a local edit — logged as `issue.conflict`. This is a documented LWW-by-timestamp limitation until a remote-hash baseline lands.
- **GitHub label writes are additive** (`--add-label` only, no `--remove-label`). Because `adb:`/`priority:` labels change *keys* rather than accumulate, this is fine in practice, but the remote label set is **not** fully reconciled.

---

## 2. Archive (`adb archive`)

`adb archive` ships the **allowlisted** parts of your knowledge base to a versioned S3 bucket and pulls them back. It exists so your KB survives a lost laptop — not as a sharing or collaboration channel.

> ### Gate: the bucket is a prerequisite, not something adb creates
>
> **adb does not ship a CloudFormation/CDK stack, and it does not provision a bucket, KMS key, or IAM policy.** PR #78 shipped **only the Go S3 archive engine.** You must supply a real AWS account and a **pre-existing S3 bucket** yourself.
>
> The client relies on the bucket's **default encryption** for at-rest protection — `s3client.go` deliberately does **not** set `ServerSideEncryption`/`SSEKMSKeyId` on `PutObject`, keeping the client IAM policy minimal (anchor: `internal/integration/cloudsync/s3client.go`). Provisioning that bucket with default SSE-KMS, versioning, and a scoped IAM policy is an **external / manual** step you own. Code comments referencing "set by CDK" and "run `cdk destroy` after" describe *your* out-of-band provisioning, not anything in this repo.

### Commands

```
adb archive push   [--bucket <name>] [--region <r>] [--dry-run]
adb archive pull    --dest <dir> [--bucket <name>] [--region <r>]
adb archive status  [--bucket <name>] [--region <r>]
adb archive destroy --confirm [--bucket <name>] [--region <r>]
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

**Include root files (individually eligible at the workspace root):** `CLAUDE.md`, `Taskfile.yaml`, `Taskfile.yml`, `WIKI.md`, `.markdownlint-cli2.yaml`, `.gitleaks.toml`, `.pre-commit-config.yaml`, `.gitignore`.

**Denied segments (a hit *anywhere* in the path is a hard NO):** `backlog.yaml`, **`.adb`**, `.taskrc`, `.taskconfig`, `.adb-workspace-README.md`, `work`, `repos`, `.omnictx`, `communications`, `sessions`, and `.env` / `.env.*` — plus a retained set of **legacy root-level state names** (below).

The single `.adb` entry is what covers adb's per-workspace state today. Since #186 all 12 state files live *inside* `.adb/` (`internal/statedir` — `events.jsonl`, `governance.jsonl`, `task_counter`, `session_counter`, `context_state.yaml`, the scheduler triad, `automation_cursor`, `session_changes`, `evidence_reads`, `memory.sqlite`), and because a hit on *any* path component is a hard NO, denying `.adb` denies every one of them — including files added later. **A new state file never needs a new allowlist entry.**

> `mcp_cache.json` used to be the 13th. It is **retired**, not implemented: `adb mcp check` deliberately caches nothing across runs, so the file was dead data and its `statedir` const an orphan nothing read or wrote. The const is gone and `internal/migration.go`'s `removeDefunctState` now *deletes* any copy left on disk (see [§3](#3-the-mcp-server)).

The legacy root-level names — `.task_counter`, `.session_counter`, `.events.jsonl`, `.governance.jsonl`, `.adb_memory.sqlite`(+`-shm`/`-wal`), `.adb_mcp_cache.json`, `.adb_session_changes` — are **still denied, and that is not redundant.** They are the pre-#186 spellings, kept so an **un-migrated** workspace stays safe: one whose one-shot migration (`internal/migration.go`) hasn't run yet, or couldn't move a file. That includes `.adb_mcp_cache.json`, whose *deletion* is likewise a first-`NewApp` event — until that has run, or if it was refused (a symlink, a directory), the file is still on disk and still needs denying. Read the comment above `deniedSegment` before "tidying" any of them away — deleting them would silently widen the boundary for exactly the workspaces that need it most.

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
adb archive push --dry-run

# Real push (bucket via env), which runs gitleaks fail-closed first:
export ADB_CLOUD_BUCKET=my-adb-archive        # region defaults to ap-southeast-2
adb archive push

# Compare remote vs local object counts:
adb archive status --bucket my-adb-archive

# Restore into a fresh directory:
adb archive pull --bucket my-adb-archive --dest ~/adb-restore

# Empty the bucket's objects (the bucket itself stays):
adb archive destroy --bucket my-adb-archive --confirm
```

### Honest gates (recap)

1. A real **AWS account + a pre-provisioned S3 bucket** (no auto-create). adb ships no infra.
2. **`gitleaks` on `PATH`** — missing binary fails *closed* and aborts a real push.
3. A green **`--dry-run` does NOT prove the push will pass** — dry-run runs neither gitleaks nor S3.
4. `push` uploads a `repos-manifest.tsv` (columns `path`, `origin`, `head`, `branch`) so you can reconstruct which repos existed, but it does **not** archive the repos themselves (`repos/` is denied).

### The manifest's `origin` column carries no credential

`origin` comes from `git remote get-url origin`, which returns whatever the clone was configured with — so a clone set up as `https://user:ghp_…@github.com/org/repo.git` would put that token, in plaintext, into an object that gets uploaded. **The credential is stripped before the manifest is built** (`cloudsync.StripOriginCredentials`, applied at both the capture and the render boundary, so no `RepoEntry` holds a token even in memory).

What is stripped is the **userinfo component only**, because the column has to stay *re-cloneable* — that is the whole reason the manifest exists, and it is why blanket redaction would be the wrong fix here: `***` in place of the credential produces a URL nothing can clone.

| Origin as configured | In the manifest |
|---|---|
| `https://user:TOKEN@host/o/r.git` | `https://host/o/r.git` |
| `https://TOKEN@host/o/r.git` (a PAT as the username) | `https://host/o/r.git` |
| `ssh://git:TOKEN@host/o/r.git` | `ssh://git@host/o/r.git` — the password goes, the **login username stays** |
| `ssh://git@host/o/r.git` | unchanged, byte-identical |
| `git@github.com:org/repo.git` (scp-style) | unchanged, byte-identical — not a URL, and that `git@` is a username |
| `https://github.com/org/repo.git` | unchanged, byte-identical |

Two asymmetries worth knowing. Over **http(s)** the whole userinfo goes, because the username is credential material too (embedding a PAT as the username with no password is the common shape) and an https clone needs no username — your credential helper supplies both parts. Over **ssh** the username survives, because it is a login identity rather than a credential: `ssh://github.com/o/r.git` does not clone, it offers your local account name and is refused.

> Before this, the only thing between an embedded token and the bucket was the fail-closed gitleaks scan noticing it in the staging copy. That is real mitigation but it was *incidental* — it depended on the manifest being staged before the scan, and it turned a routine push into a hard abort rather than doing the right thing with the URL. A token-bearing origin now pushes successfully, with the token removed.

> **Events note:** cloud sync emits `cloud.sync_pushed`/`pulled`/`status`/`destroyed`, but those four are declared *locally* and are **not** in the canonical observability schema (`KnownEventTypes`). So `adb events query` sees them, but tooling that treats `KnownEventTypes` as the complete allowlist will handle them differently from the registered `issue.*` events. See [L400 — Architecture & Extending](./L400-architecture-and-extending.md).

---

## 3. The MCP server

`adb mcp serve` runs adb as a **Model Context Protocol server over stdio**, so an MCP client (Claude Code, another agent) can drive adb's task lifecycle as tools.

> **This is the *only* `serve` verb.** There is no `adb serve` web dashboard, no editor integration, and no TUI — all three are gone (`adb dashboard` was removed in TASK-00039).

### Command

```
adb mcp serve                                    # MCP server over stdio (no flags)
adb mcp check [--json] [--no-cache] [--exit-code]  # report the MCP servers THIS MACHINE has configured
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
      "env": { "ADB_HOME": "/Users/you/Code/myproject" }
    }
  }
}
```

That block works in any of the config files below — a project `.mcp.json`, `~/.claude.json`, or a Claude Desktop config — and `adb mcp check` reads all of them, so a registration like this one shows up in its report (as a `stdio` server whose `env_keys` lists `ADB_HOME`, never its value).

### `adb mcp check` — what is configured, and whether it can be launched

```
adb mcp check [--json] [--no-cache] [--exit-code]
```

**Anchors:** `internal/cli/mcp_check.go` — `newMCPCheckCmd` (the command + rendering), `execCommandResolver.Resolve` (the stdio verdict), `planMCPProbes`/`probeMCPServers`/`classifyMCPProbe` (the http verdict) · `internal/integration/mcpconfig.go:DiscoverMCPServers` (discovery + redaction) · `internal/integration/mcpclient.go:DefaultMCPClient.CheckHealth` (the HTTP probe).

This answers one question — *"which MCP servers does this machine have configured, and would each one start?"* — and is deliberately narrow about the second half. It reports **launchability, not liveness**; see the gate below.

#### Discovery: four files, most specific first

| # | File | Label in the output | What it is |
|---|------|--------------------|------------|
| 1 | `<workspace>/.mcp.json` | `project .mcp.json` | Project scope — and the file `adb harness build` emits ([L500 §9](./L500-founder-playbook-os.md#9-the-claude-code-plugin)). |
| 2 | `~/.claude.json` | `Claude Code` | Claude Code's own config: the root `mcpServers` block **plus** its per-project `projects[<workspace path>].mcpServers` block, keyed by the workspace path (`App.BasePath`, and its `EvalSymlinks`'d form too — Claude Code keys these blocks by *resolved* path, so a workspace reached through `/tmp` or a symlinked `~/Code` would otherwise match nothing and lose its whole project block silently). The project block is the more specific of the two, so **a per-project entry overrides a root entry of the same name** — and the winner's `scope` in `--json` is the project path rather than `global`. |
| 3 | `~/.config/Claude/claude_desktop_config.json` | `Claude Desktop` | Claude Desktop, XDG location. |
| 4 | `~/Library/Application Support/Claude/claude_desktop_config.json` | `Claude Desktop` | Claude Desktop, macOS location. |

That order **is** the precedence order: a server name configured in more than one file is reported **once**, from the most specific file that carries it — the same "most specific wins" shape as adb's own Repo > Org > Global config tiers. So a project `.mcp.json` entry shadows a same-named `~/.claude.json` one, and the report names the file that won.

Two properties of discovery worth relying on:

- **A malformed config reports its parse error and does not suppress the others.** Reading is best-effort **per file, and within a file** — a single unparseable `projects[<path>]` entry in `~/.claude.json` records its error without costing that file its valid root servers, so you see both the servers and the breakage. (A leading UTF-8 BOM is tolerated rather than reported as an invalid character.) A **broken** file is *always* surfaced in the human view — after the summary, as its own block — so a `✓`-only report can never quietly rest on a config that failed to parse:

  ```console
  1 launchable
  note: verifies each command resolves to an executable, not that the server responds

  ⚠ 1 config file(s) could not be read — servers in them are NOT listed above:
    ✗ ~/.config/Claude/claude_desktop_config.json  (Claude Desktop) — parse: invalid character 'o' looking for beginning of object key string
  ```

- **Every file consulted is reported, including the absent ones** — "I looked here and found nothing" is usually the answer you need when servers you expected are missing. The *complete* list appears in the human view only in the nothing-found case (`No MCP servers configured.` followed by `Looked in:`, one line per file, marked `not found` / `no servers` / its parse error); it is **always** in `--json`'s `sources[]`, each with `present` / `servers` / `error`.

#### The verdict is per-server, and depends on the transport

| Transport | Detected by | Check | Verdict |
|---|---|---|---|
| `stdio` | a `command` (or `type: stdio`) — the historical shape, and the default when nothing says otherwise | resolve the command to something **this user can execute**: `exec.LookPath` for a bare name; for a path, an `os.Lstat` to diagnose clearly and then `exec.LookPath` for the executability decision itself | `launchable` (with the resolved absolute path) / `unresolved` — `command not found: "…"`, `no such file: …`, `not executable: …`, `not a file: …`, `broken symlink: … → …` |
| `http` | a `url`, or `type: sse` / `http` / `streamable-http` — all three report as `http` | one HTTP `GET` on the url via `integration.MCPClient`, **not following redirects** | `reachable` — **any** HTTP response, annotated `status N` / `unreachable` — only when *nothing answered* (refused, DNS, TLS, timeout, or the whole-phase budget) |
| — | an entry with **neither** a command nor a url | none possible | `unknown` — `entry declares neither a command nor a url`, or `entry declares a blank command ("   ")` for a whitespace-only one; surfaced rather than hidden |

Two details of the stdio check are worth knowing, because both were once wrong in a way that produced a false `✓`:

- **Executability is delegated, not hand-rolled.** Testing `Perm()&0o111 != 0` asks whether *any* execute bit is set, so a `chmod 001` file, a root-owned `0700` tool, and a group-restricted `0750` one all passed. `exec.LookPath` performs the `access(2)`-style check with the **effective uid/gid** — the same question the OS answers when a client actually spawns the server.
- **A relative `command` resolves against the config file's directory**, not adb's cwd. Otherwise the same config reports `launchable` from the workspace root and `no such file` from a subdirectory of it. The message names both spellings — `not executable: "tools/no-exec" (resolved to /…/ws/tools/no-exec)` — so a relative verdict is diagnosable.

On the http side, `reachable` deliberately means **"the url answered"** and nothing more. A correct Streamable-HTTP MCP endpoint answers a bare `GET` (no `Accept: text/event-stream`) with **405**, so treating a non-2xx as "no server" sent people looking for a server that was working; and following a redirect turned "this url sends you to an SSO login page" into a `✓` earned by the login page. Redirects are therefore reported as themselves, **named with their destination** — the difference between "it redirected to your identity provider" and "it redirected to a path you mistyped" is the whole diagnosis, and the probe already holds it in the response headers:

```console
  ✓ http-405            http   http://127.0.0.1:8931/mcp: status 405
  ✓ unauthorized        http   http://127.0.0.1:8931/401: status 401
  ✓ sso-redirect        http   http://127.0.0.1:18931/sso: status 302 (redirect → https://login.example.invalid/oauth2/authorize?client_id=adb&state=***&code=***&redirect_uri=http%3A%2F%2F127.0.0.1%3A18931%2Fcb — the response came from the redirector, not from an MCP server)
  ✓ relative-redirect   http   http://127.0.0.1:18931/relative: status 303 (redirect → /login — the response came from the redirector, not from an MCP server)
  ✓ bare-redirect       http   http://127.0.0.1:18931/bare: status 301 (redirect — the response came from the redirector, typically an SSO login page, not from an MCP server)
  ✗ refused             http   http://127.0.0.1:59999/mcp: connection refused
```

Four properties of that destination, each deliberate:

- **A relative `Location` is reported verbatim, never resolved** against the probed url (`/login` above). Resolving would print a url the origin never sent, which is the one thing a diagnostic must not do — and it reads correctly anyway, because the destination sits on the same line as the url it came from. The interesting case, a redirect *off-host* to an IdP, is absolute by necessity and passes through unchanged.
- **A 3xx with no `Location` keeps the destination-less wording** (`bare-redirect`), rather than claiming a redirect to nowhere. Symmetrically, a stray `Location` on a **200** is not a redirect and is not reported as one — the field is populated only for a 3xx.
- **It is redacted on the way in** (`integration.RedactLocation`), like every other url this command handles. Note in the example that `state`, `code`, and `nonce` are `***` while `client_id`, `redirect_uri`, and a `country_code=AU` survive untouched — see the security section below for why that rule is exact-name rather than substring.
- **`--json` gained no new key.** The destination rides in the row's existing `detail` string, so a consumer parsing the report needs no schema change.

Probes run **concurrently** (a bounded pool of 8) with a per-request timeout of 5s (`integration.DefaultMCPProbeTimeout`) and a 30s budget over the whole probe phase as a backstop against a pathological config; a row the budget outruns reports `no response within 30s`. Three unroutable remotes take ~5s, not ~30s. The HTTP client is only constructed when a discovered server actually has a url, so an all-stdio workspace makes **no network calls at all**.

#### Real output

```console
$ adb mcp check
~/.claude.json (5)
  ✓ adb              stdio  /Users/valterh/.local/bin/adb
  ✓ builder-mcp      stdio  builder-mcp → /Users/valterh/.toolbox/bin/builder-mcp
  ✓ chrome-devtools  stdio  npx → /opt/homebrew/bin/npx
  ✓ omnictx          stdio  /Users/valterh/.local/bin/omnictx-mcp
  ✓ serena           stdio  uvx → /opt/homebrew/bin/uvx

5 launchable
note: verifies each command resolves to an executable, not that the server responds
```

Rows are grouped under the config file that owns them, and a `name → path` detail marks a PATH lookup (`npx → /opt/homebrew/bin/npx`) as distinct from a command that was already absolute. A failure mix:

```console
  ✗ gone         stdio  command not found: "definitely-not-installed-xyz"
  ? malformed    stdio  entry declares neither a command nor a url
  ✗ not-exec     stdio  not executable: "/etc/hosts"
  ✗ remote-down  http   http://127.0.0.1:59999/mcp: connection refused
  ✗ stale-abs    stdio  no such file: "/nope/missing-binary"

3 unresolved · 1 unreachable · 1 unknown
note: verifies each command resolves to an executable, not that the server responds; each url answered, not that an MCP server is behind it
```

Commands are **quoted** in a message, because a configured `"/bin/ls "` with a trailing space would otherwise produce `no such file: /bin/ls` — a sentence that reads as an outright lie about a file that is right there.

The footer note is **transport-aware**, and the run above shows both halves. A stdio-only report ends `verifies each command resolves to an executable, not that the server responds`; an http-only report ends `verifies each url answered, not that an MCP server is behind it`. The halves key off the verdicts actually reached, so an `unknown` row — where nothing was checked — contributes no claim.

#### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--json` | `false` | One object — `sources`, `servers`, `summary` — with **snake_case** keys throughout, the same convention `adb program`'s six subcommands use. `servers[]` carries `name`, `source` (the file it won from), `scope` (`global` for a user-level entry, or the project path — for a `projects[<path>]` block **and** for `<workspace>/.mcp.json`, which *is* the project config), `transport`, `command`, `args`, `url`, `env_keys`, `state`, plus `resolved` / `detail` where they apply; `summary` counts `total` / `launchable` / `unresolved` / `reachable` / `unreachable` / `unknown`. `url` and `args` are the **redacted** spellings — see below. |
| `--no-cache` | `false` | Probe every http/sse entry independently, instead of coalescing entries that name the **same** url into one probe. No effect on stdio servers. See the note below. |
| `--exit-code` | `false` | Exit non-zero when any server is `unresolved`, `unreachable`, or `unknown` (`N of M mcp server(s) unusable`) — for a CI gate, the same opt-in shape as `adb audit security` and `adb conformance check`. |

#### What `--no-cache` actually controls

**Nothing is ever cached between runs**, so every `adb mcp check` reports what is true now — which is the right default for a diagnostic, and means you never need a flag to defeat a stale verdict. There is no on-disk health cache at all: the `mcp_cache.json` state file was **retired** rather than implemented, and a stale copy left by an old build is deleted on first `NewApp` (§2's note, and `internal/migration.go`'s `removeDefunctState`).

The only reuse a single run can have is **two entries naming the same url**, which are coalesced into one probe whose result is fanned out to both rows. `--no-cache` splits them back into independent probes. Measured at the binary level against a request-counting local server, three entries over two distinct urls:

```text
default   : 2 HTTP request(s)   # the two entries sharing a url probe once
--no-cache: 3 HTTP request(s)   # each entry probes independently
```

That coalescing is a **probe-planning** decision, not a caching side-effect, and the reason is worth knowing if you touch this code: it used to fall out of `MCPClient`'s in-memory TTL cache, and once probes became concurrent it could not — two entries naming one url both dial before either has a result to cache, so "the second one is free" became a race the cache always lost. Making it explicit in the planner restores it deterministically. `--no-cache` installs a decorator whose *presence* tells the planner to schedule one probe per entry, and which also clears the TTL cache before every probe (needed because with more entries than workers a repeated url can come round again after the first probe has already populated it).

Reach for it when you want a per-entry answer for duplicated urls — not to defeat a stale result, because there isn't one to defeat.

#### One security property, stated because it is easy to lose

**Credentials are dropped on the way in, not filtered on the way out.** Three kinds of secret ride in an MCP entry, and all three are neutralised at the discovery boundary so no consumer — human view, `--json`, a future caller — has to remember to:

| Where a secret hides | What the report carries |
|---|---|
| `env` values (an API token is routine) | **key names only** — `MCPServer.EnvKeys` |
| a `url`'s userinfo credential — a **password**, or a **bare token as the username** (`https://ghp_…@host/mcp`) — or its query values | the redacted spelling — `MCPServer.URLDisplay` (`integration.RedactURL`). `MCPServer.URL` keeps the raw url for the probe to dial and is `json:"-"`, so it is **never** marshalled |
| a stdio entry's `args` (`--api-key sk-live-…`, `--header "Authorization: Bearer …"`, an embedded url) | stored pre-redacted — `integration.RedactArgs` |
| a **redirect destination**'s OAuth exchange parameters | redacted before it is stored on `MCPHealthCheck.Location` — `integration.RedactLocation`, and there is deliberately no raw spelling of that field to reach for instead |

```console
  ✓ secret-url   http   http://svc:***@127.0.0.1:58931/mcp?api_key=***&x=1: status 405
  ✓ pat-url      http   http://***@127.0.0.1:58931/mcp: status 405
```

Doing it at the boundary rather than in the renderer closes a real leak: the old code prefixed the **raw** url onto an error line where `net/http` had already redacted the password, un-redacting exactly what the stdlib had been careful about.

Which part of the userinfo is the credential depends on its **shape**, and the two rows above are the two cases. Where `net/http` has a rule — a password exists — adb matches it and keeps the username, because there the username is a login identity and it tells you which service account is configured. With **no** password the whole userinfo goes, because the username is then the only slot a credential can be in, and a PAT-as-username (`https://ghp_…@host/mcp`) is the dominant real-world spelling. That second half was a genuine leak until TASK-00039: `redactUserinfo` returned early when the userinfo had no `:`, so a bare-token url printed byte-identically in the human view *and* in `--json`'s `url`, at the boundary this section says drops credentials on the way in. An **empty** userinfo (`https://@host/mcp`) is still left byte-identical — there is nothing to redact, and `***@` would *invent* a credential that was never configured, which misleads a reader the same way echoing a real one does.

The sibling redactor in `cloudsync` reaches the same verdict on that shape by a different route: `StripOriginCredentials` **removes** the whole userinfo over http(s) rather than replacing it, because `repos-manifest.tsv`'s `origin` column has to stay re-cloneable and `https://***@host/o/r.git` clones nothing. The two agree on which bytes are the credential and differ only in what they leave behind.

`RedactLocation` needs one rule `RedactURL` does not have, and the split is the point. `RedactURL` matches its hints (`token`, `key`, `secret`, `auth`, …) as **substrings**, which is right for a general redactor. An OAuth login destination's credentials are `?code=`, `?state=` and `?nonce=` — none of which contains any of those hints, so `RedactURL` alone passes a whole authorization code through byte-identically. `RedactLocation` therefore adds an **exact-name** pass over exactly those three. Exact, not substring, because `code` as a substring would redact `?country_code=` in every url adb prints — degrading the general redactor to buy a rule that is only unambiguous in this one position.

> ### Gate: this verifies launchability, not liveness
>
> `adb mcp check` does **not** start a server, does not speak MCP to it, and has no handshake mode — there is no `--probe` flag. For a stdio server, a `✓` means *its command resolves to something this user can execute*; the footer says exactly that. A server whose binary exists but crashes on startup, whose args are wrong, or whose token has expired still reports `launchable`.
>
> For an http/sse server, `reachable` means **the url answered** — that is all, and the footer's http half says so (`verifies each url answered, not that an MCP server is behind it`). It is a deliberately weak claim in *both* directions: a `405` or `401` is `✓` because a correct MCP endpoint answers a bare `GET` that way and only a handshake could tell it from a broken one, and equally, any unrelated process listening on that url earns the same `✓`. Read `status N` alongside the tick — and read a `302` as "a redirector answered", not as your server.

#### Honest limits

- **A `reachable` http server may be nothing of the kind, and `--exit-code` will not flag it.** Since any answer passes, a config full of `405`s exits `0` — which is correct (that is what a healthy Streamable-HTTP endpoint returns) and is also what an unrelated web server on the same port returns. Only a handshake could separate them, and there is no handshake.
- **A redirect chain is reported one hop deep.** Redirects are not followed, so you get the first `Location` and no further: if that destination itself redirects, the report cannot say where the chain ends. One hop is normally the answer you need (it names the IdP, or the mistyped path), and following the chain is what produced the false `✓` in the first place.
- **`--exit-code` gates on server *states*, not on the health of the run.** Two things it lets through: an **empty** report (no servers configured means nothing is unusable → exit `0`) and an **unreadable config file** (it contributes no servers, so it cannot make the count non-zero — exit `0`, even though the `⚠` block is printed right above). A CI job that wants "at least one server, and every config parsed" has to read `summary.total` and `sources[].error` from `--json`.
- **Only the full list of consulted files is conditional.** A parse failure always shows in the human view; the exhaustive "here are all four files, present or not" list appears there only when nothing was found. `--json` has no such asymmetry.
- **Discovery knows these four files and no others.** A server registered with some other MCP client — or a Claude Desktop config in a non-standard location — is invisible to it, and will look identical to "not configured".

---

## Where to go next

- **[L100 — Fundamentals](./L100-fundamentals.md)** — install, `adb init workspace`, your first task.
- **[L200 — Daily Workflows](./L200-daily-workflows.md)** — the create → start/resume → close loop, running many tickets in parallel, durable sessions.
- **[L400 — Architecture & Extending](./L400-architecture-and-extending.md)** — the observability event pipeline (`adb events`, `adb metrics`, `adb alerts`), and why `cloud.*` events sit outside the canonical schema.

### One-line honesty checklist for this tier

- Issue sync needs an authenticated `gh`/`glab`; adb never holds a token.
- Cloud sync needs *your* AWS bucket (adb ships no infra) **and** `gitleaks` on `PATH`; a real push aborts on any secret finding.
- The uploaded `repos-manifest.tsv` strips the credential from each `origin` and keeps the URL re-cloneable — it does not rely on the gitleaks scan to catch a token-bearing remote.
- The only `serve` is `adb mcp serve` (stdio MCP) — there is no web dashboard.
- `adb mcp check` tells you a stdio server's command **resolves to something you can execute**, and that an http server's url **answered** — never that an MCP server is behind either. It starts nothing, speaks no MCP, and has no handshake/`--probe` mode.
