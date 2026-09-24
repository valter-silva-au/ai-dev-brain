package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// This file owns the command groups that replaced `adb sync` (TASK-00039 Q4).
//
// `sync` was a namespace organised around a *verb*, so it collected jobs with
// nothing in common — regenerating instruction files, reconciling GitHub issues,
// pushing an S3 archive, installing an agent harness — while leaving each one's
// actual noun unnamed. Learning the CLI meant learning that "sync" was where
// four unrelated things lived.
//
// Each capability now sits under the thing it acts on. The behaviour is
// untouched: every command below is the same constructor that `sync` registered,
// re-parented, and every retired spelling survives as a hidden deprecated alias
// built from that same constructor (see alias.go for why that matters).

// NewContextCmd creates `adb context` — everything that generates or indexes the
// written-down context adb exists to maintain.
func NewContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Generate and index the written-down context",
		Long: `Generate and index the context adb keeps for you and your agents.

  build    regenerate the workspace instruction file (AGENTS.md + harness pointers)
  task     regenerate one worktree's Tier-0 task context
  repos    regenerate the multi-repo overview
  memory   index ticket knowledge into vector memory, and search it

Was ` + "`adb sync context`" + ` / ` + "`task-context`" + ` / ` + "`repos`" + ` / ` + "`adb memory`" + `.`,
	}
	cmd.AddCommand(
		newContextBuildCmd(),
		newContextTaskCmd(),
		newContextReposCmd(),
		newContextMemoryCmd(),
	)
	return cmd
}

// newContextBuildCmd is `sync context` under its noun, with `sync all` folded in
// as `--all`.
//
// The two were separate commands doing overlapping work: `sync context` wrote the
// instruction files, `sync all` wrote those *plus* the repo context and the user
// context. That is one job with a breadth flag, not two verbs — and keeping them
// apart is what let `sync all` drift into using a different generator and
// silently ignoring --force (see the comment it carried about not calling
// GenerateAll).
func newContextBuildCmd() *cobra.Command {
	build := newSyncContextCmd()
	build.Use = "build [--all] [--thin]"
	build.Short = "Regenerate the workspace instruction files"

	inner := build.RunE
	all := false
	build.RunE = func(cmd *cobra.Command, args []string) error {
		if err := inner(cmd, args); err != nil {
			return err
		}
		if !all {
			return nil
		}
		// --all extends the same run to the two sibling artifacts `sync all` also
		// wrote. Order matters: the instruction files come from the multi-section
		// generator above, so this must NOT call GenerateAll(), whose
		// GenerateContext step is the legacy backlog dump and would overwrite them.
		if err := generateSiblingContext(); err != nil {
			return err
		}
		// Say so. The inner run reports each instruction file it touched, so
		// silence here would read as "--all did nothing".
		fmt.Fprintln(cmd.OutOrStdout(), "✓ repo context + agent user context regenerated")
		return nil
	}
	// No backticks in this usage string: cobra's UnquoteUsage treats a
	// backquoted word as the flag's VALUE PLACEHOLDER, so "(was `adb sync all`)"
	// rendered as `--all adb sync all` in help — making a bool flag look like it
	// takes an argument.
	build.Flags().BoolVar(
		&all,
		"all",
		false,
		"Also regenerate the repo context and the agent user context (was 'adb sync all')",
	)
	return build
}

// newContextTaskCmd is `sync task-context` under its noun.
func newContextTaskCmd() *cobra.Command {
	cmd := newSyncTaskContextCmd()
	cmd.Use = "task <task-id>"
	cmd.Short = "Regenerate one worktree's task context"
	return cmd
}

// newContextReposCmd is `sync repos` under its noun.
func newContextReposCmd() *cobra.Command {
	cmd := newSyncReposCmd()
	cmd.Use = "repos"
	cmd.Short = "Regenerate the multi-repo context overview"
	return cmd
}

// newContextMemoryCmd is `adb memory`, moved under `context` and pruned to the
// two verbs that serve the knowledge loop.
//
// `store`/`delete`/`list` were manual pokes at a *derived* index — the store is
// rebuildable from ticket knowledge by `index`, so hand-editing it could only
// create drift. `export`/`import` were stubs that returned an error on every
// invocation. The underlying store methods survive; only the CLI verbs go
// (`search_knowledge` over MCP still reads the same store).
func newContextMemoryCmd() *cobra.Command {
	cmd := NewMemoryCmd()
	cmd.Short = "Index ticket knowledge into vector memory, and search it"
	return cmd
}

// NewWikiCmd creates `adb wiki` — publishing ticket knowledge as a browsable,
// LLM-consumable corpus.
func NewWikiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wiki",
		Short: "Publish ticket knowledge as a navigable corpus",
	}
	publish := newSyncWikiCmd()
	publish.Use = "publish [--out <dir>]"
	publish.Short = "Publish ticket knowledge to a wiki tree"
	cmd.AddCommand(publish)
	return cmd
}

// NewIssuesCmd creates `adb issues`. `sync` survives as a verb here and only
// here, because reconciling two authorities really is the operation — unlike the
// other former `sync` children, which generate or publish in one direction.
func NewIssuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issues",
		Short: "Reconcile tickets with GitHub/GitLab issues",
	}
	sync := newSyncIssuesCmd()
	sync.Use = "sync [--repo <platform/org/repo>] [--dry-run] [--direction both|push|pull]"
	cmd.AddCommand(sync)
	return cmd
}

// NewArchiveCmd creates `adb archive` — the S3 archive plane, formerly
// `adb sync cloud`. "cloud" named the transport; "archive" names the job.
func NewArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Back the knowledge base up to S3, and restore it",
		Long: `Back the allowlisted knowledge base up to a versioned S3 bucket.

This exists so the KB survives a lost laptop — it is not a sharing or
collaboration channel. adb ships no infrastructure: you supply the bucket, and a
real push runs a fail-closed gitleaks scan before any object is uploaded.

Was ` + "`adb sync cloud`" + `.`,
	}
	cmd.AddCommand(
		newSyncCloudPushCmd(),
		newSyncCloudPullCmd(),
		newSyncCloudStatusCmd(),
		newSyncCloudDestroyCmd(),
	)
	return cmd
}

// NewHarnessCmd creates `adb harness` — installing and packaging the agent
// harness adb ships (agents, skills, and the MCP registration).
//
// `install` is the former `sync claude-user`. `adb init claude` is deliberately
// NOT folded in here despite naming the same agent: it does a different job
// (writes a stub CLAUDE.md at a target *path*, rather than regenerating from live
// data into the agent's *config dir*) and takes a different argument. Nothing
// replaces it, so it stays where it is — the same replacement test the `adb init`
// child visibility rule uses.
func NewHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "harness",
		Short: "Install and package the agent harness",
		Long: `Install and package the agent harness adb ships.

  install    regenerate live context and install the harness into the agent's
             config dir (was ` + "`adb sync claude-user`" + `)
  build      emit an installable Claude Code plugin + marketplace
  manifest   print the plugin manifest

Was ` + "`adb sync claude-user`" + ` and ` + "`adb plugin`" + `.`,
	}
	install := newSyncClaudeUserCmd()
	install.Use = "install [name]"
	install.Short = "Install the harness into the agent's config dir"
	install.Args = cobra.MaximumNArgs(1)

	cmd.AddCommand(install, newPluginBuildCmd(), newPluginManifestCmd())
	return cmd
}

// newRetiredSyncCmd is the hidden `adb sync` tree. Every child names its own
// replacement, because a retired namespace does not move to one place: `sync`
// scattered into `context`, `wiki`, `issues`, `archive`, and `harness`.
func newRetiredSyncCmd() *cobra.Command {
	cloud := deprecatedAliasTree(
		"cloud",
		"Deprecated: use `adb archive`",
		deprecatedAlias(newSyncCloudPushCmd, "push", "adb archive push"),
		deprecatedAlias(newSyncCloudPullCmd, "pull", "adb archive pull"),
		deprecatedAlias(newSyncCloudStatusCmd, "status", "adb archive status"),
		deprecatedAlias(newSyncCloudDestroyCmd, "destroy", "adb archive destroy"),
	)
	return deprecatedAliasTree(
		"sync",
		"Deprecated: redistributed into `context`, `wiki`, `issues`, `archive`, `harness`",
		deprecatedAlias(newSyncContextCmd, "context", "adb context build"),
		deprecatedAlias(newSyncTaskContextCmd, "task-context", "adb context task"),
		deprecatedAlias(newSyncReposCmd, "repos", "adb context repos"),
		deprecatedAlias(newSyncAllCmd, "all", "adb context build --all"),
		deprecatedAlias(newSyncWikiCmd, "wiki", "adb wiki publish"),
		deprecatedAlias(newSyncIssuesCmd, "issues", "adb issues sync"),
		cloud,
		deprecatedAlias(newSyncClaudeUserCmd, "claude-user", "adb harness install"),
	)
}

// newRetiredMemoryCmd is the hidden `adb memory` tree. Only the two surviving
// verbs get aliases: the dropped ones have nothing to point at.
func newRetiredMemoryCmd() *cobra.Command {
	cmd := deprecatedAliasTree(
		"memory",
		"Deprecated: use `adb context memory`",
		deprecatedAlias(newMemoryIndexCmd, "index", "adb context memory index"),
		deprecatedAlias(newMemorySearchCmd, "search", "adb context memory search"),
	)
	// The connection flags are persistent on the real parent, so the alias tree
	// needs them too or `adb memory search --provider ollama` stops parsing.
	addMemoryConnectionFlags(cmd)
	return cmd
}

// newRetiredPluginCmd is the hidden `adb plugin` tree.
func newRetiredPluginCmd() *cobra.Command {
	return deprecatedAliasTree(
		"plugin",
		"Deprecated: use `adb harness`",
		deprecatedAlias(newPluginBuildCmd, "build", "adb harness build"),
		deprecatedAlias(newPluginManifestCmd, "manifest", "adb harness manifest"),
	)
}

// newRetiredReposCmd is the hidden `adb repos` tree. Its children moved onto the
// v3 `adb repo` noun; `list` had to become `inventory` there because v3 already
// owns `repo list` for a different thing (registered repositories, not clones).
func newRetiredReposCmd() *cobra.Command {
	return deprecatedAliasTree(
		"repos",
		"Deprecated: use `adb repo pull` / `adb repo inventory`",
		deprecatedAlias(newReposPullCmd, "pull", "adb repo pull"),
		deprecatedAlias(newReposListCmd, "list", "adb repo inventory"),
	)
}

// newRetiredStatusCmd is the hidden top-level `adb status`.
//
// It is built from the listing's own constructor with `--git` DEFAULTED ON,
// because that is what `adb status` was: the task list joined with live git
// state. Building it any other way would have made `adb status` and
// `adb task list --git` two implementations of one report, which is the drift
// deprecatedAlias exists to prevent — the flag default is the only difference.
func newRetiredStatusCmd() *cobra.Command {
	return deprecatedAlias(
		func() *cobra.Command { return newTaskListCmdWithGitDefault(true) },
		"status",
		"adb task list --git",
	)
}

// newRetiredWorkCmd is the hidden `adb work` tree. Its children all moved onto
// `adb task worktree`, so unlike `sync` this namespace really did go to one
// place — the old name was just vague about which noun it operated on.
//
// `adb work prune` and `adb work reconcile` now PREVIEW by default (see
// newTaskWorktreeCmd's safety rule). That is a behaviour change on a deprecated
// spelling, in the fail-safe direction, and `--dry-run` still parses as the
// no-op it has become.
func newRetiredWorkCmd() *cobra.Command {
	return deprecatedAliasTree(
		"work",
		"Deprecated: use `adb task worktree`",
		deprecatedAlias(newTaskWorktreeListCmd, "list", "adb task worktree list"),
		deprecatedAlias(newTaskWorktreeSwitchCmd, "switch", "adb task worktree switch"),
		deprecatedAlias(newTaskWorktreePruneCmd, "prune", "adb task worktree prune"),
		deprecatedAlias(newTaskWorktreeReconcileCmd, "reconcile", "adb task worktree reconcile"),
	)
}

// NewRepoPullCmd and NewRepoInventoryCmd are exported so the v3-composed root can
// mount them onto its `repo` parent. They read the legacy App, so like
// `NewOrgCreateCmd` they must carry no v3 annotation.
func NewRepoPullCmd() *cobra.Command {
	return newReposPullCmd()
}

func NewRepoInventoryCmd() *cobra.Command {
	cmd := newReposListCmd()
	cmd.Use = "inventory"
	cmd.Short = "List cloned repos and the tickets spanning each"
	return cmd
}

// generateSiblingContext regenerates the repo context and the agent user context
// — the two artifacts `adb sync all` produced beyond the instruction files.
// Extracted so `context build --all` and the retired `sync all` alias run exactly
// the same code.
func generateSiblingContext() error {
	if App == nil {
		return errNoApp
	}
	contextGen := core.NewContextGenerator(
		App.BasePath+"/backlog.yaml",
		App.BasePath+"/tickets",
		App.BasePath,
		App.TemplateManager,
	)
	if err := contextGen.GenerateRepoContext(); err != nil {
		return fmt.Errorf("failed to generate repo context: %w", err)
	}
	if err := contextGen.GenerateClaudeUserContext(false, false); err != nil {
		return fmt.Errorf("failed to generate agent user context: %w", err)
	}
	return nil
}

// errNoApp is the standard uninitialised-App error these handlers return.
var errNoApp = errors.New("app not initialized")
