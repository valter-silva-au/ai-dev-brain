package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// This file is the CLI surface for TEMPLATE PROVISIONING (TASK-00031): ticket
// templates ship inside adb as defaults, are provisioned into the workspace
// ($ADB_HOME/templates/) and are thereafter user-owned — edit/add/remove
// without rebuilding adb. Rendering resolves the workspace copy first, falling
// back to the embedded defaults. Every handler here is thin — resolve, call
// into internal/core, print — mirroring `adb compliance` / `adb gtm` /
// `adb program`.

// templatesDir returns the workspace template layer directory ($ADB_HOME/templates).
func templatesDir() string {
	return filepath.Join(App.BasePath, "templates")
}

// NewTemplatesCmd creates the `adb templates` command group.
func NewTemplatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Provision and inspect the ticket templates",
		Long: `Provision and inspect the ticket templates (context/notes/design/handoff/status/task-context).

Ticket templates are provisioned defaults: ` + "`adb templates provision`" + ` installs the
embedded defaults into $ADB_HOME/templates/, where they are user-owned from then on.
Rendering (at ` + "`adb task create`" + `) resolves the workspace copy first and falls back
to the embedded default, so an edited template wins and a deleted one reverts to the
shipped version. Templates are referenced by id + version (stamped into ticket
frontmatter at render time), never a filesystem path.`,
	}
	cmd.AddCommand(
		newTemplatesProvisionCmd(),
		newTemplatesListCmd(),
		newTemplatesShowCmd(),
		newTemplatesValidateCmd(),
	)
	return cmd
}

// newTemplatesProvisionCmd implements `adb templates provision`.
func newTemplatesProvisionCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Install the embedded template defaults into the workspace (user-owned from then on)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			destDir := templatesDir()
			res, err := core.ProvisionTemplates(templates.FS, destDir, core.HarnessInstallOptions{DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to provision templates: %w", err)
			}
			verb := "Provisioned"
			if dryRun {
				verb = "Would provision"
			}
			fmt.Printf("%s templates into %s:\n", verb, res.ClaudeDir)
			for _, e := range res.Entries {
				note := ""
				if e.Action == core.HarnessSkipped {
					note = " (edited locally; pass --force to overwrite)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-10s %s%s\n", e.Action, filepath.Base(e.Dest), note)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite a provisioned template that was edited (differs from the embedded copy)")
	return cmd
}

// newTemplatesListCmd implements `adb templates list`: every ticket template,
// where it currently resolves from (workspace override or embedded) and its
// version stamp.
func newTemplatesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List ticket templates with their resolution source and version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			lm, err := core.NewLayeredTemplateManager(templatesDir(), templates.FS)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%-18s %-12s %s\n", "TEMPLATE", "SOURCE", "VERSION")
			for _, tt := range core.TicketTemplates {
				source, err := lm.Source(tt)
				if err != nil {
					source = fmt.Sprintf("(unreadable: %v)", err)
				}
				version, verr := lm.Version(tt)
				if verr != nil {
					version = "-"
				}
				fmt.Fprintf(out, "%-18s %-18s %s\n", string(tt), source, version)
			}
			return nil
		},
	}
}

// newTemplatesShowCmd implements `adb templates show <name>`: print the
// resolved template content (workspace override wins) with a provenance header.
func newTemplatesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <template>",
		Short: "Print a ticket template as currently resolved (workspace override wins)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			tt := core.TemplateType(args[0])
			lm, err := core.NewLayeredTemplateManager(templatesDir(), templates.FS)
			if err != nil {
				return err
			}
			source, err := lm.Source(tt)
			if err != nil {
				return fmt.Errorf("template %s not found: %w", args[0], err)
			}
			// Version resolves the same template Source just resolved, so this
			// cannot fail here — but reporting beats printing "version: " blank
			// if that ever stops being true.
			version, verr := lm.Version(tt)
			if verr != nil {
				return fmt.Errorf("template %s version: %w", args[0], verr)
			}
			content, rerr := readResolvedTemplate(templatesDir(), templates.FS, tt)
			if rerr != nil {
				return rerr
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "# template: %s · source: %s · version: %s\n\n", tt, source, version)
			_, err = out.Write(content)
			return err
		},
	}
}

// readResolvedTemplate fetches the raw bytes of the resolved template,
// workspace-first. (Show prints raw bytes; the manager parses for validation.)
func readResolvedTemplate(wsDir string, embedded fs.FS, tt core.TemplateType) ([]byte, error) {
	if wsDir != "" {
		if b, err := os.ReadFile(filepath.Join(wsDir, string(tt))); err == nil {
			return b, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading workspace template %s: %w", tt, err)
		}
	}
	return fs.ReadFile(embedded, string(tt))
}

// newTemplatesValidateCmd implements `adb templates validate`: every template
// must parse; workspace overrides are reported against the embedded default so
// a user can see exactly what they've diverged.
func newTemplatesValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check every ticket template parses (workspace override or embedded)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			lm, err := core.NewLayeredTemplateManager(templatesDir(), templates.FS)
			if err != nil {
				return fmt.Errorf("template validation failed: %w", err)
			}
			out := cmd.OutOrStdout()
			failed := false
			for _, tt := range core.TicketTemplates {
				source, err := lm.Source(tt)
				if err != nil {
					failed = true
					fmt.Fprintf(out, "FAIL %-18s %v\n", tt, err)
					continue
				}
				// Unreachable while Source above succeeded (both go through the
				// same resolve), but a template whose version cannot be read is
				// a validation FAIL like any other — not a blank column.
				version, verr := lm.Version(tt)
				if verr != nil {
					failed = true
					fmt.Fprintf(out, "FAIL %-18s %v\n", tt, verr)
					continue
				}
				note := ""
				if source != "embedded" {
					if embVer := embeddedVersion(tt); embVer != version {
						note = " (edited; embedded version " + embVer + ")"
					}
				}
				fmt.Fprintf(out, "ok   %-18s %-12s %s%s\n", tt, source, version, note)
			}
			if failed {
				return fmt.Errorf("one or more templates failed to parse")
			}
			return nil
		},
	}
}

// embeddedVersion returns the version stamp of the embedded copy of tt, or ""
// if unreadable.
func embeddedVersion(tt core.TemplateType) string {
	b, err := fs.ReadFile(templates.FS, string(tt))
	if err != nil {
		return "?"
	}
	return core.TemplateVersion(b)
}
