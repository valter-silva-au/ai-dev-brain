package v3cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// allCommands flattens a command tree, hidden commands included.
func allCommands(root *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{root}
	for _, child := range root.Commands() {
		out = append(out, allCommands(child)...)
	}
	return out
}

// TestRemovedCommandsAreGone pins the TASK-00039 removals against the *composed*
// root — the command tree a user actually gets.
//
// These are removed rather than aliased, deliberately. A `deprecatedAlias` keeps
// a *renamed* capability reachable; there is nothing for these to point at, and
// a hidden alias that still worked would just be the command with worse
// discoverability. So `Find` must fail for each.
//
// Cobra's `Find` returns the nearest matching parent with the unmatched tokens
// when a name does not resolve, rather than erroring — so the assertion has to
// check what was actually resolved, not just that err is nil.
func TestRemovedCommandsAreGone(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		Repository:   &fakeRepository{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	for _, path := range [][]string{
		// Top-level removals (TASK-00039 Q6 + the command-surface review):
		// adb is terminal-native and MCP-first, so a TUI, an LLM chat adapter,
		// a generic process runner, and two non-functional multi-agent
		// launchers are all outside the product.
		{"dashboard"},
		{"chat"},
		{"exec"},
		{"run"},
		{"team"},
		{"agents"},
		// Bulk task operations (Q8) — an explicit per-ticket loop is scriptable
		// and does not hide which tickets moved.
		{"task", "start-all"},
		{"task", "close-all"},
		// One-shot migrations and normalizers: these existed to fix workspaces
		// that predate a schema change, not to be part of the product surface.
		{"task", "migrate-types"},
		{"task", "normalize-titles"},
		{"task", "normalize"},
		// A ruflo-specific launcher, and the session store's CLI surface.
		{"task", "run-with-ruflo"},
		{"session"},
		// Tool-effectiveness telemetry (#203). Removed in TASK-00039: it had no
		// store, exactly one emitter and one consumer, and no non-human reader,
		// so it was a note-taking command wearing a third-party vendor's name.
		// Serena PROVISIONING is a different feature and stays — it lives on the
		// worktree-bootstrap seam and never had a CLI surface.
		{"serena"},
	} {
		name := strings.Join(path, " ")
		t.Run(name, func(t *testing.T) {
			found, remaining, err := root.Find(path)
			if err != nil {
				return // resolved to nothing at all — removed.
			}
			// `Find` degrades to the deepest matching ancestor. The command is
			// only still present if nothing was left unmatched.
			if len(remaining) == 0 && found != nil &&
				found.Name() == path[len(path)-1] {
				t.Fatalf("`adb %s` is still registered", name)
			}
		})
	}
}

// TestRemovedCommandsAreNotAdvertisedAnywhere catches the subtler regression: a
// command deleted from the tree but still named in help text, so users keep
// being told to run it. The generated-artifact equivalents (workspace CLAUDE.md,
// project_context.md) are covered at the binary level in test/e2e.
func TestRemovedCommandsAreNotAdvertisedAnywhere(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		Repository:   &fakeRepository{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	// Walk every command's help surface, including hidden ones: a deprecated
	// alias pointing at a removed command would be actively misleading.
	var offenders []string
	stale := []string{
		"adb dashboard",
		"adb chat",
		"adb exec ",
		"adb run ",
		"adb team",
		"adb agents",
		"task start-all",
		"task close-all",
		"task migrate-types",
		"task normalize-titles",
		"run-with-ruflo",
		"adb serena",
	}
	for _, cmd := range allCommands(root) {
		haystack := cmd.Short + "\n" + cmd.Long + "\n" + cmd.Example
		for _, needle := range stale {
			if strings.Contains(haystack, needle) {
				offenders = append(
					offenders,
					cmd.CommandPath()+" mentions "+needle,
				)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf(
			"help text still advertises removed commands:\n  %s",
			strings.Join(offenders, "\n  "),
		)
	}
}
