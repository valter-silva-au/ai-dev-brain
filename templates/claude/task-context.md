# Task Context (Tier 0)

You are working on **{{.TaskID}}: {{.Title}}**. This file is your always-loaded core:
identity + how-to-work + pointers. Pull deeper tiers on demand (see the end).

## (a) Identity & State

- **Task:** {{.TaskID}} — {{.Title}}
- **Ticket:** `{{.TicketPath}}`
- **Worktree:** {{if .WorktreePath}}`{{.WorktreePath}}`{{else}}(this directory){{end}}
- **Branch:** {{if .Branch}}`{{.Branch}}`{{else}}(unset){{end}}
- **Status:** {{.Status}}{{if .Phase}} · **Phase:** {{.Phase}}{{end}}{{if .ProgressPct}} · **Progress:** {{.ProgressPct}}%{{end}}
- **Created:** {{.CreatedAt}}{{if .UpdatedAt}} · **Updated:** {{.UpdatedAt}}{{end}}
{{- if .Stage}}
- **Initiative:** {{if .Initiative}}{{.Initiative}} · {{end}}**Stage:** {{.Stage}} (founder-playbook Idea→MVP→Launch→Scale)
{{- end}}
{{- if .Gate}}
- **Gate:** {{.Gate}}
{{- end}}

**Acceptance criteria:**
{{range .AcceptanceCriteria}}
- [ ] {{.}}
{{- else}}
- [ ] Not yet defined — see `{{.TicketPath}}/context.md`
{{- end}}

{{if .SteerDirectives}}**Active steer:** {{.SteerDirectives}}{{else}}**Active steer directives:** re-read `{{.TicketPath}}/steer.md` before each major action.{{end}}

## (b) The Loop, Definition of Done & Standing DOs/DONTs

**Loop:** research -> develop -> verify -> repeat. Understand before you change; make the
smallest change that works; prove it with a test or a run; loop until the DoD holds.

**Definition of Done:** acceptance criteria met · builds clean · tests pass (new + existing) ·
lint/format/vet clean on touched files · docs/notes updated · nothing destructive done without
confirmation. Do not report done until every box is checked.

**Standing DOs/DONTs (binding, inlined — a worktree shadows the KB root):**

{{.StandingRules}}

## (c) Related tickets (pointers only)

{{range .Siblings}}
- {{.}}
{{- else}}
- No siblings resolved yet. Siblings live under `tickets/{{if .RepoSubPath}}{{.RepoSubPath}}{{else}}<platform>/<org>/<repo>{{end}}/` in the KB.
{{- end}}
- Repo tickets dir: `tickets/{{if .RepoSubPath}}{{.RepoSubPath}}{{else}}<platform>/<org>/<repo>{{end}}/`
- Workspace decisions index: `$ADB_HOME/wiki/decisions/`

## (d) Other live sessions

{{if .LiveSessions}}{{.LiveSessions}}{{else}}No other active sessions reported (live digest wired in a later ticket).{{end}}

## Go deeper (Tier 1/2 — pull on demand)

- Tier 1 (this ticket): `{{.TicketPath}}/context.md` · `design.md` · `notes.md` ·
  `knowledge/decisions.yaml` · `sessions/`
- Tier 2 (workspace): `$ADB_HOME/CLAUDE.md` · cross-machine digest
  `$ADB_HOME/tickets/_active-digest.<host>.md`

<!-- adb:tier0 state={{.StateHash}} generated={{.CreatedAt}} -->
