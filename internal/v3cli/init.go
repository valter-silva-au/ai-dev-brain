package v3cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
)

// newInitCommand builds `adb init` as a NAMESPACE over two distinct things that
// used to be told apart only by the presence of a positional argument:
//
//	adb init boundary <path>   the .aidb workspace boundary (v3 foundation)
//	adb init workspace <path>  the task workspace (backlog.yaml, tickets/, work/)
//
// Both are legitimate and neither replaces the other, which is exactly why the
// bare `adb init <path>` form was a trap: nothing in help said the positional
// form was a *different concept* from its named sibling, let alone which one it
// was. TASK-00039 Q2 resolved this by naming both.
//
// The bare form still works — scripts and muscle memory depend on it — but warns
// and is no longer advertised. See newInitBoundaryCommand for the shared body.
func newInitCommand(service Foundation) *cobra.Command {
	bare := newFoundationInitializer(service)

	command := &cobra.Command{
		Use:   "init",
		Short: "Initialize a workspace boundary, task workspace, or project",
		Long: `Initialize adb state.

  adb init boundary <path>    the .aidb workspace boundary (manifest, config,
                              state, journal) — the v3 foundation
  adb init workspace <path>   the task workspace (backlog.yaml, tickets/, work/)
                              that the task lifecycle reads
  adb init project <path>     a project, with document-program packs

` + "`boundary`" + ` and ` + "`workspace`" + ` are different things; neither replaces the
other. Passing a bare path (` + "`adb init <path>`" + `) still runs ` + "`boundary`" + ` for
compatibility, but is deprecated — name the subcommand instead.`,
		// MaximumNArgs, not ExactArgs: zero args must reach help rather than
		// erroring, now that this is a namespace.
		Args: cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			v3Annotation: "true",
		},
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 0 {
				return command.Help()
			}
			fmt.Fprintf(
				command.ErrOrStderr(),
				"warning: `adb init <path>` is deprecated and will be removed; "+
					"use `adb init boundary <path>` (or `adb init workspace <path>` "+
					"for the task workspace)\n",
			)
			return bare.RunE(command, args)
		},
	}

	// The bare form's flags live on the parent so `adb init <path> --apply`
	// parses exactly as it did before; `boundary` carries its own copy.
	bare.Flags().VisitAll(func(flag *pflag.Flag) {
		command.Flags().AddFlag(flag)
	})

	command.AddCommand(newInitBoundaryCommand(service))

	legacyInit := cli.NewInitCmd()
	for _, child := range legacyInit.Commands() {
		legacyInit.RemoveCommand(child)
		// Hide the children that the top-level `adb init <path>` supersedes, but
		// keep the ones with no replacement VISIBLE.
		//
		// `init workspace` is the only way to create the task workspace
		// (backlog.yaml, tickets/, work/) that the whole task lifecycle reads —
		// `adb init <path>` creates the .aidb/ boundary, which is a different
		// thing. Hiding it meant a new user following `adb init --help` could not
		// get started at all: the one command they needed was invisible.
		child.Hidden = !visibleInitChildren[child.Name()]
		command.AddCommand(child)
	}

	return command
}

// newInitBoundaryCommand is the advertised spelling for initializing the `.aidb`
// workspace boundary.
func newInitBoundaryCommand(service Foundation) *cobra.Command {
	command := newFoundationInitializer(service)
	command.Use = "boundary <path>"
	command.Short = foundation.InitializeDescriptor.Summary
	command.Long = `Initialize the .aidb workspace boundary.

Creates .aidb/manifest.yaml, .aidb/config.yaml, .aidb/state.sqlite,
.aidb/events/, .aidb/cache/, and organizations/. Dry-run is the default;
--apply is the only mutating path, and re-applying returns "unchanged".

This is NOT the task workspace — for backlog.yaml, tickets/, and work/, use
` + "`adb init workspace <path>`" + `.`
	return command
}

// newFoundationInitializer builds the `workspace.initialize/v1` command body.
// It is constructed twice — once as the `boundary` subcommand, once as the
// deprecated bare `adb init <path>` form — so the two cannot drift in flags,
// defaults, or validation. Building from one function is what keeps the
// compatibility path honest rather than an approximation of the real one.
func newFoundationInitializer(service Foundation) *cobra.Command {
	var (
		name   string
		apply  bool
		dryRun bool
		format string
	)

	command := &cobra.Command{
		Use:   foundation.InitializeDescriptor.Command + " <path>",
		Short: foundation.InitializeDescriptor.Summary,
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{
			v3Annotation: "true",
		},
		RunE: func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("foundation service is not configured")
			}
			if apply && dryRun {
				return errors.New("--apply and --dry-run are mutually exclusive")
			}

			root, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve workspace path: %w", err)
			}
			workspaceName := name
			if workspaceName == "" {
				workspaceName = filepath.Base(filepath.Clean(root))
			}

			result, err := service.Initialize(
				command.Context(),
				foundation.InitializeRequest{
					Root:  root,
					Name:  workspaceName,
					Apply: apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderInitializeHuman,
			)
		},
	}

	command.Flags().StringVar(&name, "name", "", "Workspace display name")
	command.Flags().BoolVar(
		&apply,
		"apply",
		false,
		"Apply the initialization plan",
	)
	command.Flags().BoolVar(
		&dryRun,
		"dry-run",
		false,
		"Preview initialization without writing (the default)",
	)
	command.Flags().StringVar(
		&format,
		"format",
		"human",
		"Output format: human or json",
	)
	return command
}

// visibleInitChildren are the `adb init` subcommands that remain in public help
// because nothing has replaced them yet. Everything else is a staged-compat
// spelling and stays hidden.
//
// The test of membership is replacement, not age: `adb init <path>` creates the
// .aidb boundary and supersedes neither of these.
//
//   - `workspace` scaffolds the task workspace (backlog.yaml, tickets/, work/)
//     that the whole task lifecycle reads.
//   - `project` provisions a document-program pack into a project, and is the
//     only way to do it — hiding it made the `adb program` surface look like it
//     had no entry point.
var visibleInitChildren = map[string]bool{
	"workspace": true,
	"project":   true,
}

func renderInitializeHuman(
	writer io.Writer,
	result capability.Result[foundation.InitializeData],
) error {
	if _, err := fmt.Fprintf(
		writer,
		"%s/%s: %s\n",
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		writer,
		"Workspace: %s\nRoot: %s\n",
		result.Data.WorkspaceID,
		result.Data.Root,
	); err != nil {
		return err
	}
	if err := renderEffects(writer, result.Effects); err != nil {
		return err
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}
