// Package claude embeds the CLAUDE CODE HARNESS: the adversarial
// devils-advocate agent and the founder-playbook skills. This is the
// harness-adapter tree (TASK-00031): agent-specific glue lives here, separate
// from the agent-agnostic document templates under templates/. The tree is
// enumerated data-driven via core.HarnessManifest and installed by
// `adb harness install` (and packaged by `adb harness build`), so adding a
// skill or agent is a matter of dropping a file into this tree — never a Go
// change.
package claude

import "embed"

//go:embed all:skills all:agents
var FS embed.FS
