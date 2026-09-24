package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// deprecatedAlias builds a hidden command that keeps an old spelling working
// after a rename, delegating to the same behaviour under the new name.
//
// It takes the target's own *constructor* rather than a built command, and that
// is the whole point: calling the constructor reproduces the flag set, defaults,
// usage strings, and positional-args contract exactly. Hand-writing a second
// command is how a rename quietly drops a flag or changes a default, and the
// resulting breakage looks like the old spelling "still working".
//
// A command may have only one parent in cobra, so a cross-namespace move
// (`adb sync context` → `adb context build`) genuinely needs two command
// objects; this is how both stay in lockstep.
//
// The notice is written to stderr by hand rather than through cobra's
// `Deprecated` field. Cobra prints that one via `Printf` → `OutOrStderr()`,
// which resolves to **stdout** as soon as any ancestor has called `SetOut` — and
// a helpful sentence prepended to `--json` output is a silently broken pipeline,
// not a visible failure. Hidden keeps it out of help; the wrapper owns the
// message.
func deprecatedAlias(
	build func() *cobra.Command,
	oldName string,
	replacement string,
) *cobra.Command {
	command := build()

	// Keep everything after the verb — `<target>`, `[flags]`, and friends
	// document the positional contract and must survive the rename.
	if _, rest, found := strings.Cut(command.Use, " "); found {
		command.Use = oldName + " " + rest
	} else {
		command.Use = oldName
	}

	command.Hidden = true
	command.Short = fmt.Sprintf("Deprecated: use `%s`", replacement)
	command.Long = ""
	// An alias inherits no aliases of its own: the old name is the alias.
	command.Aliases = nil

	notice := func(cmd *cobra.Command) {
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"warning: `adb %s` is deprecated and will be removed; use `%s`\n",
			cmd.CommandPath()[len("adb "):],
			replacement,
		)
	}

	// Wrap whichever runner the target actually declared. Commands in this
	// package use RunE almost everywhere, but `version` uses Run, and an alias
	// that silently stopped running would be worse than no alias.
	switch {
	case command.RunE != nil:
		inner := command.RunE
		command.RunE = func(cmd *cobra.Command, args []string) error {
			notice(cmd)
			return inner(cmd, args)
		}
	case command.Run != nil:
		inner := command.Run
		command.Run = func(cmd *cobra.Command, args []string) {
			notice(cmd)
			inner(cmd, args)
		}
	default:
		// A parent-only command (no runner of its own). Its children carry the
		// behaviour, so the notice belongs on each of them; nothing to wrap here.
	}

	return command
}

// deprecatedAliasTree builds a hidden parent whose children are deprecated
// aliases, for retiring a whole namespace (`adb sync …`). Each child names its
// own replacement, because a retired namespace does not move to one place —
// `adb sync` scattered into `context`, `issues`, `archive`, `wiki`, `harness`.
func deprecatedAliasTree(
	oldName string,
	short string,
	children ...*cobra.Command,
) *cobra.Command {
	parent := &cobra.Command{
		Use:    oldName,
		Short:  short,
		Hidden: true,
	}
	parent.AddCommand(children...)
	return parent
}
