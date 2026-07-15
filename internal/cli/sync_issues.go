package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration/issuesync"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// newSyncIssuesCmd creates `adb sync issues` — bidirectional GitHub/GitLab
// issue sync (WS-E). Unlike the other `sync` subcommands (which regenerate
// LOCAL context files), this one talks to a remote: it links each ticket
// whose `repo:` names a real github.com / gitlab remote to an issue and
// reconciles them last-writer-wins over title/body/labels/status/priority.
//
// Auth is per-host and owned by the user's `gh` / `glab` login. This code
// path never reads ~/.config/gh/hosts.yml, never accepts a --token flag, and
// never writes a token to backlog.yaml, status.yaml, or .events.jsonl —
// audited by internal/integration/issuesync's TestSyncer_EventPayloadHasNoCredentials.
func newSyncIssuesCmd() *cobra.Command {
	var (
		repo      string
		dryRun    bool
		direction string
	)
	cmd := &cobra.Command{
		Use:   "issues [--repo <platform/org/repo>] [--dry-run] [--direction both|push|pull]",
		Short: "Reconcile adb tickets with GitHub/GitLab issues",
		Long: `Reconcile each ticket whose repo names a real github.com or gitlab remote
with its remote issue. Direction defaults to 'both' (last-writer-wins over
title/body/labels/status/priority); --dry-run shows the plan without writing.

_local tickets, absolute or relative local paths, and enterprise-internal hosts
(anything that isn't a github.com/gitlab remote) are skipped.

Auth is per-host: this uses the host's gh / glab login. No token is ever
read or stored by adb.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.BacklogManager == nil {
				return fmt.Errorf("app not initialized")
			}

			dir := issuesync.Direction(direction)
			switch dir {
			case issuesync.DirectionBoth, issuesync.DirectionPush, issuesync.DirectionPull:
			default:
				return fmt.Errorf("invalid --direction %q (must be both, push, or pull)", direction)
			}

			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("load backlog: %w", err)
			}

			ticketsDir := filepath.Join(App.BasePath, "tickets")
			s := &issuesync.Syncer{
				Body: func(t models.Task) string {
					// Body is the ticket's context.md; best-effort (empty on
					// miss). ResolveTicketDir handles any nesting depth so
					// this works for legacy flat tickets and the WS-A
					// nested correlation layout.
					dir, rerr := core.ResolveTicketDir(ticketsDir, t.ID)
					if rerr != nil {
						return ""
					}
					b, _ := os.ReadFile(filepath.Join(dir, "context.md"))
					return string(b)
				},
				WriteBody: func(t models.Task, remoteBody string) error {
					// On a remote-wins pull, persist the remote body to the
					// ticket's context.md (body is a bidirectional LWW field);
					// otherwise the pulled body is silently dropped (#176).
					dir, rerr := core.ResolveTicketDir(ticketsDir, t.ID)
					if rerr != nil {
						return rerr
					}
					return os.WriteFile(filepath.Join(dir, "context.md"), []byte(remoteBody), 0o644)
				},
				Write: func(t models.Task) error { return App.BacklogManager.UpdateTask(t) },
				Log: func(evt string, data map[string]interface{}) {
					App.EventLog.Log(observability.EventType(evt), data)
				},
			}

			synced := 0
			for _, t := range backlog.Tasks {
				if repo != "" && t.Repo != repo {
					continue
				}
				res := s.SyncTask(t, dir, dryRun)
				if res.Action != issuesync.ActionNoop {
					synced++
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s)\n", res.TaskID, res.Action, res.Reason)
				}
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %d synced%s\n", synced, mode)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Limit to one platform-qualified repo (e.g. github.com/org/repo)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show the reconcile plan without writing")
	cmd.Flags().StringVar(&direction, "direction", "both", "Sync direction: both, push, or pull")
	return cmd
}
