package claude

import "embed"

// FS contains embedded template files.
//
// The top-level globs cover the task-artifact templates and rules; the
// `all:projectinit` pattern recursively embeds the project-scaffolding tree
// (including dotfiles like .taskrc / .gitignore and the .claude/ subtree that a
// bare glob would skip), so `adb init project` can source every scaffolded file
// from here rather than from inline string literals.
//
// The `all:skills` / `all:agents` trees are the Claude Code harness (the
// devils-advocate agent + the founder-playbook skills). They are enumerated
// data-driven via core.HarnessManifest and installed by `adb sync claude-user`,
// so adding a skill or agent is a matter of dropping a file into the tree.
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
//go:embed *.md *.yaml *.sh rules/*.md
//go:embed all:projectinit
//go:embed all:skills all:agents
//go:embed all:validation
//go:embed all:compliance
//go:embed all:gtm
var FS embed.FS
