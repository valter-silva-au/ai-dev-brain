package cli

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
// silently stopped being required. There is nothing to recover at runtime, so
// the honest handling is to name it on stderr (never stdout, so a `--json`
// pipeline stays parseable) where the next invocation surfaces it.
func requireFlag(command *cobra.Command, name string) {
	reportFlagWiringError(command, command.MarkFlagRequired(name))
}

// hideFlag hides a flag, with the same contract as requireFlag: the only failure
// is an unregistered name, and a silently-unhidden flag would advertise a
// deprecated spelling in --help.
func hideFlag(command *cobra.Command, name string) {
	reportFlagWiringError(command, command.Flags().MarkHidden(name))
}

// reportFlagWiringError writes a construction-time flag-wiring failure to the
// command's error writer. Routed through cobra's ErrOrStderr (rather than
// os.Stderr directly) so it lands wherever the caller pointed the command's
// stderr, which is also what makes it testable.
func reportFlagWiringError(command *cobra.Command, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(command.ErrOrStderr(), "adb: internal flag-wiring error: %v\n", err)
}
