package v3cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// requireFlag marks a flag required, reporting a wiring failure instead of
// discarding it.
//
// cobra's MarkFlagRequired returns an error in exactly one situation: the named
// flag was never registered. That is a typo in the constructor the call sits in,
// and its consequence is invisible — the command keeps working, with a flag that
// silently stopped being required. `--org`, `--workspace` and `--path` all carry
// real safety weight here (see the repository/organization scope rules), so the
// failure is named on stderr — never stdout, so `--format json` stays parseable.
func requireFlag(command *cobra.Command, name string) {
	if err := command.MarkFlagRequired(name); err != nil {
		fmt.Fprintf(command.ErrOrStderr(), "adb: internal flag-wiring error: %v\n", err)
	}
}
