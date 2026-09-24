package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildAliasFixture is a stand-in for a real command constructor: it has flags,
// an args spec, and a body that records that it ran.
func buildAliasFixture(ran *bool) func() *cobra.Command {
	return func() *cobra.Command {
		var (
			format string
			force  bool
			count  int
		)
		cmd := &cobra.Command{
			Use:   "build <target>",
			Short: "Build the thing",
			Args:  cobra.ExactArgs(1),
			RunE: func(command *cobra.Command, args []string) error {
				*ran = true
				return nil
			},
		}
		cmd.Flags().StringVar(&format, "format", "human", "output format")
		cmd.Flags().BoolVar(&force, "force", false, "overwrite")
		cmd.Flags().IntVar(&count, "count", 3, "how many")
		return cmd
	}
}

// TestDeprecatedAliasPreservesTheEntireFlagSet is the point of building an alias
// from the target's own constructor rather than hand-writing a second command:
// a rename must not quietly drop or re-default a flag, and hand-maintaining two
// flag sets is exactly how that happens.
func TestDeprecatedAliasPreservesTheEntireFlagSet(t *testing.T) {
	ran := false
	build := buildAliasFixture(&ran)
	target := build()
	alias := deprecatedAlias(build, "old-build", "adb new build")

	target.Flags().VisitAll(func(want *pflag.Flag) {
		got := alias.Flags().Lookup(want.Name)
		if got == nil {
			t.Fatalf("alias dropped flag --%s", want.Name)
		}
		if got.DefValue != want.DefValue {
			t.Fatalf(
				"alias --%s default = %q, want %q",
				want.Name,
				got.DefValue,
				want.DefValue,
			)
		}
		if got.Usage != want.Usage {
			t.Fatalf("alias --%s usage = %q, want %q", want.Name, got.Usage, want.Usage)
		}
	})
}

// TestDeprecatedAliasKeepsTheArgsSpecAndRenamesOnlyTheVerb pins that the alias
// is the same command under a different name — the positional-args contract has
// to survive, or the old spelling starts rejecting invocations it used to take.
func TestDeprecatedAliasKeepsTheArgsSpecAndRenamesOnlyTheVerb(t *testing.T) {
	ran := false
	alias := deprecatedAlias(buildAliasFixture(&ran), "old-build", "adb new build")

	if alias.Name() != "old-build" {
		t.Fatalf("alias name = %q, want old-build", alias.Name())
	}
	// The `<target>` suffix documents a required positional and must carry over.
	if alias.Use != "old-build <target>" {
		t.Fatalf("alias Use = %q, want %q", alias.Use, "old-build <target>")
	}
	if err := alias.Args(alias, []string{}); err == nil {
		t.Fatal("alias accepted zero args; the ExactArgs(1) spec was lost")
	}
	if err := alias.Args(alias, []string{"x"}); err != nil {
		t.Fatalf("alias rejected its one valid arg: %v", err)
	}
}

// TestDeprecatedAliasIsHiddenButStillRuns is the whole contract of a staged
// rename: existing scripts keep working, and nobody discovers the old spelling
// from help.
func TestDeprecatedAliasIsHiddenButStillRuns(t *testing.T) {
	ran := false
	alias := deprecatedAlias(buildAliasFixture(&ran), "old-build", "adb new build")

	if alias.IsAvailableCommand() {
		t.Fatal("alias must not appear in help")
	}
	if !alias.Hidden {
		t.Fatal("alias must be Hidden")
	}

	root := &cobra.Command{Use: "adb", SilenceUsage: true}
	root.AddCommand(alias)
	root.SetArgs([]string{"old-build", "target"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("alias failed to run: %v\n%s", err, out.String())
	}
	if !ran {
		t.Fatal("alias did not run the target's behaviour")
	}
	if !strings.Contains(out.String(), "adb new build") {
		t.Fatalf("alias did not name its replacement:\n%s", out.String())
	}
}

// TestDeprecatedAliasNoticeGoesToStderr is the load-bearing one. The deprecation
// notice must never land on stdout: `adb sync context --json | jq` has to keep
// parsing after the rename, and a helpful sentence prepended to the JSON is a
// silently broken pipeline rather than a visible failure.
//
// Cobra prints the notice via c.Printf → OutOrStderr(), which resolves to stderr
// only while nothing has called SetOut. Production never does; this test is what
// notices if that changes.
func TestDeprecatedAliasNoticeGoesToStderr(t *testing.T) {
	ran := false
	build := func() *cobra.Command {
		return &cobra.Command{
			Use:  "old",
			Args: cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				ran = true
				// Stand in for a command that writes machine-readable output.
				command.OutOrStdout().Write([]byte(`{"ok":true}` + "\n"))
				return nil
			},
		}
	}
	alias := deprecatedAlias(build, "old", "adb new")

	root := &cobra.Command{Use: "adb", SilenceUsage: true}
	root.AddCommand(alias)
	root.SetArgs([]string{"old"})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := root.Execute(); err != nil {
		t.Fatalf("alias failed to run: %v", err)
	}
	if !ran {
		t.Fatal("alias did not run")
	}
	if strings.Contains(stdout.String(), "deprecated") {
		t.Fatalf(
			"deprecation notice polluted stdout, breaking machine-readable output:\n%s",
			stdout.String(),
		)
	}
	if !strings.Contains(stderr.String(), "deprecated") {
		t.Fatalf("deprecation notice did not reach stderr:\n%s", stderr.String())
	}
	if stdout.String() != `{"ok":true}`+"\n" {
		t.Fatalf("stdout is not exactly the command's own output:\n%q", stdout.String())
	}
}
