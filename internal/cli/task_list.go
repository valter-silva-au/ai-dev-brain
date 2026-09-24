package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// taskRowJSON is the machine-readable shape of one task, emitted by
// `adb task list --json` (as an array element) and `adb task show --json` (as the
// whole document).
//
// The field set is the one the MCP server's task view carries — kept in step
// deliberately, so an agent gets the same data whichever entry point it drives.
type taskRowJSON struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Owner    string   `json:"owner,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Repo     string   `json:"repo,omitempty"`
	// WorktreePath and TicketPath let a machine consumer decide launchability
	// and a working directory without re-reading backlog.yaml. Branch is
	// included for completeness.
	WorktreePath string `json:"worktree_path,omitempty"`
	TicketPath   string `json:"ticket_path,omitempty"`
	Branch       string `json:"branch,omitempty"`
	// Initiative is the associated founder-playbook initiative id (empty when
	// the ticket has no association). Metadata only — see pkg/models/task.go.
	Initiative string `json:"initiative,omitempty"`
}

// taskGitJSON is the live git state of one task's worktree.
//
// It is a type of its own, embedded by both taskGitRowJSON (`task list --git`)
// and statusRow (`task worktree list`), so the two commands cannot come to spell
// the same five facts differently. None of the fields is `omitempty`: `false`
// and `0` are answers here, and a missing `dirty` key would read as "unknown"
// when it means "clean".
type taskGitJSON struct {
	WorktreeExists bool `json:"worktree_exists"`
	Dirty          bool `json:"dirty"`
	Ahead          int  `json:"ahead"`
	Behind         int  `json:"behind"`
	// WorktreeMissing means a worktree_path is recorded but absent on disk —
	// distinct from a task that never had one.
	WorktreeMissing bool `json:"worktree_missing"`
}

// taskGitRowJSON is a task row enriched with live git state. The embedded struct
// is anonymous and by value, so encoding/json promotes both field sets into one
// flat object: `--git` adds keys to each element rather than changing the
// document's shape.
type taskGitRowJSON struct {
	taskRowJSON
	taskGitJSON
}

// taskRow projects a task onto its JSON row.
func taskRow(t models.Task) taskRowJSON {
	return taskRowJSON{
		ID:           t.ID,
		Title:        t.Title,
		Type:         string(t.Type),
		Status:       string(t.Status),
		Priority:     string(t.Priority),
		Owner:        t.Owner,
		Tags:         t.Tags,
		Repo:         t.Repo,
		WorktreePath: t.WorktreePath,
		TicketPath:   t.TicketPath,
		Branch:       t.Branch,
		Initiative:   t.Initiative,
	}
}

// gitStateFor reads the live git state of one task's worktree, returning the
// state plus the branch to display (the live branch when git could report one,
// else whatever the backlog recorded).
//
// A task with no recorded worktree yields the zero state — not an error and not
// an omission. That is what lets `task list --git` enrich every row instead of
// filtering to worktree-bearing tasks the way buildStatusRows does.
func gitStateFor(
	task models.Task,
	gitStatus func(worktreePath string) (integration.WorktreeStatus, error),
) (taskGitJSON, string) {
	branch := task.Branch
	if task.WorktreePath == "" || gitStatus == nil {
		return taskGitJSON{}, branch
	}
	state, err := gitStatus(task.WorktreePath)
	switch {
	case err != nil:
		// The directory may exist but git couldn't report — don't claim
		// missing; keep the backlog-recorded branch.
		return taskGitJSON{WorktreeExists: true}, branch
	case !state.Exists:
		return taskGitJSON{WorktreeMissing: true}, branch
	default:
		if state.Branch != "" {
			branch = state.Branch
		}
		return taskGitJSON{
			WorktreeExists: true,
			Dirty:          state.Dirty,
			Ahead:          state.Ahead,
			Behind:         state.Behind,
		}, branch
	}
}

// filterTasksByStatus narrows a task set to one status, rejecting an unknown
// value rather than filtering to nothing.
//
// "No tasks found" is a WRONG ANSWER for a typo: `--filter in-progress` (hyphen)
// or `--filter Done` (case) both read as "you have no such work" when they
// actually mean "that is not a status".
func filterTasksByStatus(tasks []models.Task, filterStatus string) ([]models.Task, error) {
	if filterStatus == "" {
		return tasks, nil
	}
	if !models.IsValidTaskStatus(models.TaskStatus(filterStatus)) {
		return nil, fmt.Errorf(
			"invalid --filter %q (want one of: %s)",
			filterStatus, strings.Join(models.ValidTaskStatusNames(), ", "),
		)
	}
	var filtered []models.Task
	for _, task := range tasks {
		if string(task.Status) == filterStatus {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

// newTaskListCmd creates `adb task list` — the task listing, formerly
// `adb task status`.
func newTaskListCmd() *cobra.Command {
	return newTaskListCmdWithGitDefault(false)
}

// newTaskListCmdWithGitDefault builds the listing with a chosen `--git` default.
//
// The retired top-level `adb status` was exactly this command with the git join
// always on, so it is built from here with the default flipped rather than
// reimplemented — the same lockstep guarantee deprecatedAlias gives a rename.
func newTaskListCmdWithGitDefault(gitDefault bool) *cobra.Command {
	var (
		filterStatus string
		jsonOut      bool
		gitState     bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks, optionally filtered by status",
		Long: `List every task in the backlog, grouped by status.

--filter narrows to one status. --git joins each task with the live state of its
worktree (branch, dirty, ahead/behind, missing). --git never changes WHICH tasks
are listed: a task with no worktree keeps its row with the git fields at their
zero values.

--json always emits an array, with or without --git; --git adds keys to each
element. Worktrees that no ticket owns are reported by 'adb task worktree list',
which is the command that owns them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			tasks, err := filterTasksByStatus(backlog.Tasks, filterStatus)
			if err != nil {
				return err
			}

			if !gitState {
				if jsonOut {
					rows := make([]taskRowJSON, 0, len(tasks))
					for _, t := range tasks {
						rows = append(rows, taskRow(t))
					}
					return writeIndentedJSON(cmd.OutOrStdout(), rows)
				}
				printTaskGroups(cmd.OutOrStdout(), tasks)
				return nil
			}

			rows := make([]taskGitRowJSON, 0, len(tasks))
			for _, t := range tasks {
				state, branch := gitStateFor(t, App.GitWorktreeManager.WorktreeStatus)
				row := taskRow(t)
				row.Branch = branch
				rows = append(rows, taskGitRowJSON{taskRowJSON: row, taskGitJSON: state})
			}
			if jsonOut {
				return writeIndentedJSON(cmd.OutOrStdout(), rows)
			}
			printTaskGitTable(cmd.OutOrStdout(), rows)
			return nil
		},
	}

	cmd.Flags().StringVar(&filterStatus, "filter", "", "Filter by status (backlog, in_progress, blocked, review, done, archived)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON (always an array, with or without --git)")
	cmd.Flags().BoolVar(&gitState, "git", gitDefault, "Join with live per-worktree git state (branch, dirty, ahead/behind, missing)")

	return cmd
}

// writeIndentedJSON encodes v as indented JSON with a trailing newline. Used by
// every task listing so the shape decisions live in one place.
func writeIndentedJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTaskGroups renders the human listing: tasks grouped by status, in
// lifecycle order rather than alphabetically.
func printTaskGroups(w io.Writer, tasks []models.Task) {
	if len(tasks) == 0 {
		fmt.Fprintln(w, "No tasks found")
		return
	}

	byStatus := make(map[models.TaskStatus][]models.Task)
	for _, task := range tasks {
		byStatus[task.Status] = append(byStatus[task.Status], task)
	}

	for _, status := range []models.TaskStatus{
		models.TaskStatusInProgress,
		models.TaskStatusReview,
		models.TaskStatusBlocked,
		models.TaskStatusBacklog,
		models.TaskStatusDone,
		models.TaskStatusArchived,
	} {
		group := byStatus[status]
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s (%d):\n", strings.ToUpper(string(status)), len(group))
		for _, task := range group {
			fmt.Fprintf(w, "  %s: [%s] %s [%s] (owner: %s)\n",
				task.ID, task.Type, task.Title, task.Priority, task.Owner)
		}
	}
}

// printTaskGitTable renders the --git listing: one row per task, with a git
// column. A task with no worktree shows "—" rather than a misleading "clean".
func printTaskGitTable(w io.Writer, rows []taskGitRowJSON) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No tasks found")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "TASK\tSTATUS\tBRANCH\tGIT\tREPO")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Status, r.Branch, gitColumn(r.taskGitJSON, r.WorktreePath), r.Repo)
	}
	_ = tw.Flush()
}

// gitColumn renders one task's git state as a single table cell.
func gitColumn(state taskGitJSON, worktreePath string) string {
	if worktreePath == "" {
		return "—"
	}
	cell := "clean"
	switch {
	case state.WorktreeMissing:
		cell = "MISSING"
	case state.Dirty:
		cell = "dirty"
	}
	if state.Ahead > 0 || state.Behind > 0 {
		cell = fmt.Sprintf("%s ↑%d↓%d", cell, state.Ahead, state.Behind)
	}
	return cell
}

// newTaskShowCmd creates `adb task show <task-id>` — the per-task read that
// `task status` never was (it took no positional argument and only ever listed).
func newTaskShowCmd() *cobra.Command {
	var (
		jsonOut  bool
		gitState bool
	)

	cmd := &cobra.Command{
		Use:   "show <task-id>",
		Short: "Show one task in detail",
		Long: `Show a single task: its identity, lifecycle state, paths, and associations.

--json emits ONE OBJECT (not a one-element array), with the same field set as a
'adb task list --json' row, so a consumer can reuse the same decoder. --git adds
the live worktree state.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			task, err := App.BacklogManager.GetTask(args[0])
			if err != nil {
				return fmt.Errorf("failed to load task: %w", err)
			}
			if task == nil {
				return fmt.Errorf("task %s not found", args[0])
			}

			row := taskRow(*task)
			var state taskGitJSON
			if gitState {
				state, row.Branch = gitStateFor(*task, App.GitWorktreeManager.WorktreeStatus)
			}

			if jsonOut {
				if gitState {
					return writeIndentedJSON(cmd.OutOrStdout(), taskGitRowJSON{taskRowJSON: row, taskGitJSON: state})
				}
				return writeIndentedJSON(cmd.OutOrStdout(), row)
			}
			printTaskDetail(cmd.OutOrStdout(), *task, row, state, gitState)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as a single JSON object")
	cmd.Flags().BoolVar(&gitState, "git", false, "Include the live worktree git state")

	return cmd
}

// printTaskDetail renders the human `task show` view.
func printTaskDetail(w io.Writer, task models.Task, row taskRowJSON, state taskGitJSON, withGit bool) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\n", row.ID, row.Title)
	fmt.Fprintf(tw, "Type\t%s\n", row.Type)
	fmt.Fprintf(tw, "Status\t%s\n", row.Status)
	fmt.Fprintf(tw, "Priority\t%s\n", row.Priority)
	if row.Owner != "" {
		fmt.Fprintf(tw, "Owner\t%s\n", row.Owner)
	}
	if len(row.Tags) > 0 {
		fmt.Fprintf(tw, "Tags\t%s\n", strings.Join(row.Tags, ", "))
	}
	if row.Initiative != "" {
		fmt.Fprintf(tw, "Initiative\t%s\n", row.Initiative)
	}
	if row.Repo != "" {
		fmt.Fprintf(tw, "Repo\t%s\n", row.Repo)
	}
	if row.Branch != "" {
		fmt.Fprintf(tw, "Branch\t%s\n", row.Branch)
	}
	if row.WorktreePath != "" {
		fmt.Fprintf(tw, "Worktree\t%s\n", row.WorktreePath)
	}
	if row.TicketPath != "" {
		fmt.Fprintf(tw, "Ticket\t%s\n", row.TicketPath)
	}
	if withGit {
		fmt.Fprintf(tw, "Git\t%s\n", gitColumn(state, row.WorktreePath))
	}
	fmt.Fprintf(tw, "Created\t%s\n", task.Created.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated\t%s\n", task.Updated.Format(time.RFC3339))
	_ = tw.Flush()
}

// newTaskCloseCmd creates `adb task close <task-id>` — mark a task done.
//
// It delegates to core.TaskManager.Close, the same method the MCP server's
// adb_task_close uses, so "closing a task" means one thing across every entry
// point rather than two hand-rolled UpdateStatus calls that drift.
func newTaskCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <task-id>",
		Short: "Close a task (mark it done)",
		Long: `Mark a task done.

Closing only changes status: the ticket directory, worktree, and branch are all
left alone, and 'adb task update <id> --status in_progress' reverses it. Use
'adb task archive' to retire the ticket, or 'adb task worktree remove' to reclaim
just the worktree.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			taskID := args[0]
			if err := App.TaskManager.Close(taskID); err != nil {
				return fmt.Errorf("failed to close task: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Task %s closed (done)\n", taskID)
			return nil
		},
	}
	return cmd
}

// newTaskTimelineCmd creates `adb task timeline <task-id>` — one task's history,
// read out of the event log.
//
// No new storage and no new event type: every task-scoped event already carries
// data.task_id, so the timeline is that filter with a rendering. That is also
// why its --json is byte-compatible with `adb events query --json`.
func newTaskTimelineCmd() *cobra.Command {
	var (
		since   string
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "timeline <task-id>",
		Short: "Show one task's event history, oldest first",
		Long: `Replay everything the event log recorded for a task: creation, status
changes, priority changes, worktree create/remove, agent sessions.

This is a view over '.adb/events.jsonl' — there is no separate timeline store, so
a task created before the event log existed has an empty timeline rather than a
reconstructed one. --json emits the same array shape as 'adb events query --json'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.EventLog == nil {
				return errNoApp
			}
			taskID := args[0]

			events, err := App.EventLog.ReadAll()
			if err != nil {
				return fmt.Errorf("read event log: %w", err)
			}

			var cutoff time.Time
			if since != "" {
				d, err := parseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				cutoff = time.Now().UTC().Add(-d)
			}

			filtered := filterEvents(events, "", taskID, cutoff)

			if jsonOut {
				return writeJSONArray(cmd.OutOrStdout(), filtered)
			}
			if len(filtered) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No events recorded for %s.\n", taskID)
				return nil
			}
			return writeHumanEvents(cmd.OutOrStdout(), filtered)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Only events within this window (e.g. 24h, 7d)")
	// No backticks in a flag usage string: cobra's UnquoteUsage reads a
	// backquoted word as the flag's VALUE PLACEHOLDER, so a bool flag renders as
	// `--json adb events query --json` and looks like it takes an argument.
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as a JSON array (same shape as adb events query --json)")

	return cmd
}
