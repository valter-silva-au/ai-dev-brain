package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// findCmd resolves a command path, returning nil when the path does not fully
// resolve. Cobra's Find degrades to the deepest matching ancestor, so checking
// the error alone is not enough.
func findCmd(root *cobra.Command, path ...string) *cobra.Command {
	found, remaining, err := root.Find(path)
	if err != nil || len(remaining) > 0 || found == nil {
		return nil
	}
	if found.Name() != path[len(path)-1] {
		return nil
	}
	return found
}

// TestSyncNamespaceIsRedistributed pins TASK-00039 Q4. `adb sync` was a
// namespace organised around a *verb*, which put unrelated jobs together
// (regenerating context, reconciling GitHub issues, pushing an S3 archive,
// installing a harness) and left each one's real noun unnamed. Every capability
// moves to a command named after the thing it acts on.
func TestSyncNamespaceIsRedistributed(t *testing.T) {
	root := NewRootCmd()

	for _, want := range [][]string{
		{"context", "build"},
		{"context", "task"},
		{"context", "repos"},
		{"context", "memory", "index"},
		{"context", "memory", "search"},
		{"wiki", "publish"},
		{"issues", "sync"},
		{"archive", "push"},
		{"archive", "pull"},
		{"archive", "status"},
		{"archive", "destroy"},
		{"harness", "install"},
		{"harness", "build"},
		{"harness", "manifest"},
	} {
		name := strings.Join(want, " ")
		t.Run(name, func(t *testing.T) {
			cmd := findCmd(root, want...)
			if cmd == nil {
				t.Fatalf("`adb %s` is not registered", name)
			}
			if cmd.Hidden {
				t.Fatalf("`adb %s` is hidden; it is the blessed spelling", name)
			}
		})
	}
}

// TestRetiredNamespacesStayReachableButHidden is the other half of a rename: a
// removal has nothing to point at, but these all do, so every old spelling keeps
// working and says where it went. Scripts and muscle memory do not break on a
// vocabulary change.
func TestRetiredNamespacesStayReachableButHidden(t *testing.T) {
	root := NewRootCmd()

	for _, old := range [][]string{
		{"sync", "context"},
		{"sync", "task-context"},
		{"sync", "repos"},
		{"sync", "all"},
		{"sync", "wiki"},
		{"sync", "issues"},
		{"sync", "cloud", "push"},
		{"sync", "claude-user"},
		{"memory", "index"},
		{"memory", "search"},
		{"repos", "pull"},
		{"repos", "list"},
		{"plugin", "build"},
		{"plugin", "manifest"},
	} {
		name := strings.Join(old, " ")
		t.Run(name, func(t *testing.T) {
			cmd := findCmd(root, old...)
			if cmd == nil {
				t.Fatalf("`adb %s` stopped working; a rename must keep an alias", name)
			}
			if !cmd.Hidden {
				t.Fatalf("`adb %s` is a retired spelling and must be hidden", name)
			}
			if !strings.Contains(cmd.Short, "Deprecated") {
				t.Fatalf("`adb %s` does not announce itself deprecated: %q", name, cmd.Short)
			}
		})
	}
}

// TestAliasesPreserveTheirFlagSets is why the aliases are built from the target's
// own constructor. A rename that silently drops a flag turns "the old spelling
// still works" into a lie that only shows up in someone's script.
func TestAliasesPreserveTheirFlagSets(t *testing.T) {
	root := NewRootCmd()

	for _, pair := range []struct {
		old []string
		new []string
		// argsMayDifferBecause is set only where the rename deliberately changes
		// the positional contract. Stating the reason per pair keeps the default
		// strict: a silent args change is as breaking as a dropped flag.
		argsMayDifferBecause string
	}{
		{old: []string{"sync", "context"}, new: []string{"context", "build"}},
		{old: []string{"sync", "task-context"}, new: []string{"context", "task"}},
		{old: []string{"sync", "repos"}, new: []string{"context", "repos"}},
		{old: []string{"sync", "wiki"}, new: []string{"wiki", "publish"}},
		{old: []string{"sync", "issues"}, new: []string{"issues", "sync"}},
		{old: []string{"sync", "cloud", "push"}, new: []string{"archive", "push"}},
		{old: []string{"sync", "cloud", "pull"}, new: []string{"archive", "pull"}},
		{old: []string{"sync", "cloud", "destroy"}, new: []string{"archive", "destroy"}},
		{
			old: []string{"sync", "claude-user"}, new: []string{"harness", "install"},
			// `sync claude-user` declared no Args (so cobra accepted and ignored
			// any), while `harness install [name]` names an optional harness. The
			// new spelling is deliberately narrower; the alias keeps the old
			// permissive contract so nothing that passed junk starts failing.
			argsMayDifferBecause: "harness install names an optional [name]",
		},
		{old: []string{"plugin", "build"}, new: []string{"harness", "build"}},
		{old: []string{"memory", "search"}, new: []string{"context", "memory", "search"}},
	} {
		name := strings.Join(pair.old, " ") + " → " + strings.Join(pair.new, " ")
		t.Run(name, func(t *testing.T) {
			oldCmd := findCmd(root, pair.old...)
			newCmd := findCmd(root, pair.new...)
			if oldCmd == nil || newCmd == nil {
				t.Fatalf("could not resolve both spellings (%v / %v)", oldCmd, newCmd)
			}
			// Compare the full effective flag set, inherited flags included:
			// `memory`'s connection flags are persistent on the parent, so
			// checking local flags only would miss them entirely.
			oldCmd.Flags().VisitAll(func(want *pflag.Flag) {
				got := newCmd.Flags().Lookup(want.Name)
				if got == nil {
					got = newCmd.InheritedFlags().Lookup(want.Name)
				}
				if got == nil {
					t.Fatalf("new spelling lost --%s", want.Name)
				}
				if got.DefValue != want.DefValue {
					t.Fatalf(
						"--%s default drifted: old %q, new %q",
						want.Name, want.DefValue, got.DefValue,
					)
				}
			})
			if pair.argsMayDifferBecause == "" &&
				(oldCmd.Args == nil) != (newCmd.Args == nil) {
				t.Fatalf("positional-args contract differs between spellings")
			}
		})
	}
}

// TestMemoryIsPrunedToIndexAndSearch pins the Q4 merge: `adb memory` moves under
// `adb context` and keeps only the two verbs that serve the knowledge loop.
// `store`/`delete`/`list` were manual poking at a derived index, and
// `export`/`import` were stubs that only ever returned an error.
func TestMemoryIsPrunedToIndexAndSearch(t *testing.T) {
	root := NewRootCmd()

	memory := findCmd(root, "context", "memory")
	if memory == nil {
		t.Fatal("`adb context memory` is not registered")
	}
	kept := map[string]bool{"index": true, "search": true}
	for _, child := range memory.Commands() {
		if !kept[child.Name()] {
			t.Fatalf("`adb context memory %s` should have been dropped", child.Name())
		}
	}
	for name := range kept {
		if findCmd(root, "context", "memory", name) == nil {
			t.Fatalf("`adb context memory %s` is missing", name)
		}
	}
	for _, gone := range []string{"store", "delete", "list", "export", "import"} {
		if cmd := findCmd(root, "memory", gone); cmd != nil {
			t.Fatalf("retired `adb memory %s` is still reachable", gone)
		}
	}
}

// TestContextBuildAbsorbsSyncAll folds `sync all` into a flag rather than a
// fourth spelling of the same job.
func TestContextBuildAbsorbsSyncAll(t *testing.T) {
	root := NewRootCmd()
	build := findCmd(root, "context", "build")
	if build == nil {
		t.Fatal("`adb context build` is not registered")
	}
	if build.Flags().Lookup("all") == nil {
		t.Fatal("`adb context build` needs --all to absorb `sync all`")
	}
	if findCmd(root, "sync", "all") == nil {
		t.Fatal("`adb sync all` must remain as a hidden alias")
	}
}
