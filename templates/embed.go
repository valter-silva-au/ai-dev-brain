// Package templates embeds the AGENT-AGNOSTIC template surface (TASK-00031):
// ticket-artifact templates (context/notes/design/handoff/status), the Tier-0
// worktree task-context pair, the shell prompt script, and the document packs
// (projectinit / programs / validation / compliance / gtm). Nothing in this
// tree is named after, or renders differently for, any particular agent — the
// Claude Code harness (skills + agents) lives separately under
// harnesses/claude/ and is enumerated by core.HarnessManifest.
//
// The top-level globs cover the task-artifact templates, the prompt script and
// the standing rules; the `all:projectinit` pattern recursively embeds the
// project-scaffolding tree (including dotfiles like .taskrc / .gitignore),
// so `adb init project` can source every scaffolded file from here rather
// than from inline string literals.
//
// The `all:validation` tree is the Idea/MVP validation template pack (each
// worksheet paired with a `.adversarial.md` companion prompt). It is enumerated
// data-driven via core.ValidationTemplates and scaffolded into an initiative's
// evidence dir by `adb initiative scaffold-evidence`.
//
// The `all:compliance` tree is the SOC2/GDPR/HIPAA control-checklist pack
// (#131 step 17), enumerated by core.ComplianceFrameworks and scaffolded into a
// workspace by `adb compliance scaffold <framework>`.
//
// The `all:gtm` tree is the go-to-market pack (#135 step 18): a positioning/
// messaging canvas and a moat-narrative (7 Powers / NFX / a16z), enumerated by
// core.GTMPacks and scaffolded by `adb gtm scaffold <pack>`.
//
// The `all:programs` tree holds the DOCUMENT PROGRAMS (TASK-00023): one
// subdirectory per pack, each a `program.yaml` manifest at its root plus its
// artifact templates under `templates/<phase>/`. The shipped packs are
// product-discovery, technical-design, delivery-readiness, operations, and
// change-adoption. They are enumerated data-driven via core.ListPrograms and
// surfaced by `adb program` / provisioned by `adb init project`, so adding a
// program is a pack you drop into this tree — never a Go change. `all:` is
// required: the tree is nested, and a bare glob would miss the per-phase
// template subdirectories.
//

package templates

import "embed"

//go:embed *.md *.yaml *.sh rules/*.md
//go:embed all:projectinit
//go:embed all:validation
//go:embed all:compliance
//go:embed all:gtm
//go:embed all:programs
var FS embed.FS
