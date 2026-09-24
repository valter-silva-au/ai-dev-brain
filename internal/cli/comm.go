package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewCommCmd creates the `adb comm` command group for stakeholder
// communications (issue #121). The CommunicationManager was built-but-unwired;
// this is its CLI surface: log inbound/outbound correspondence against a ticket
// and list it back. Communications live under the ticket's communications/ dir
// (dated markdown with YAML frontmatter).
func NewCommCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comm",
		Short: "Log and list stakeholder communications on a ticket",
		Long: `Record stakeholder correspondence (email, Slack, meetings) against a ticket,
each tagged with a direction (inbound = received, outbound = sent).

  adb comm log --task TASK-00001 --direction inbound --from pm@acme.com \
      --subject "Retention concern" --channel email --message "They want ..."
  adb comm list --task TASK-00001`,
	}
	cmd.AddCommand(newCommLogCmd(), newCommListCmd())
	return cmd
}

func newCommLogCmd() *cobra.Command {
	var (
		taskID    string
		direction string
		from      string
		to        []string
		subject   string
		channel   string
		tags      []string
		message   string
	)
	cmd := &cobra.Command{
		Use:   "log --task <id> --direction <inbound|outbound>",
		Short: "Log a communication against a ticket (content via --message or stdin)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.CommunicationManager == nil {
				return fmt.Errorf("app not initialized")
			}
			if strings.TrimSpace(taskID) == "" {
				return fmt.Errorf("--task is required")
			}
			dir := models.CommunicationDirection(strings.TrimSpace(direction))
			if !dir.IsValid() {
				return fmt.Errorf("--direction must be inbound or outbound")
			}
			content := message
			if content == "" {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read content: %w", err)
				}
				content = strings.TrimRight(string(b), "\r\n")
			}
			if strings.TrimSpace(content) == "" {
				return fmt.Errorf("no content: pass --message or pipe via stdin")
			}

			comm := models.NewCommunication(newCommID(), taskID, content)
			comm.Direction = dir
			comm.From = from
			comm.To = to
			comm.Subject = subject
			comm.Channel = channel
			for _, tg := range tags {
				if tg = strings.TrimSpace(tg); tg != "" {
					comm.AddTag(models.CommunicationTag(tg))
				}
			}
			if err := App.CommunicationManager.SaveCommunication(comm); err != nil {
				return fmt.Errorf("save communication: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Logged %s communication on %s%s\n", dir, taskID, subjectSuffix(subject))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&taskID, "task", "", "ticket id to attach the communication to (required)")
	f.StringVar(&direction, "direction", "", "inbound (received) or outbound (sent) (required)")
	f.StringVar(&from, "from", "", "who the communication is from")
	f.StringSliceVar(&to, "to", nil, "recipients (comma-separated)")
	f.StringVar(&subject, "subject", "", "subject line")
	f.StringVar(&channel, "channel", "", "channel: email, slack, teams, meeting, …")
	f.StringSliceVar(&tags, "tags", nil, "tags (comma-separated), e.g. question,blocker")
	f.StringVar(&message, "message", "", "the communication content (else read from stdin)")
	return cmd
}

func newCommListCmd() *cobra.Command {
	var (
		taskID     string
		direction  string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "list --task <id>",
		Short: "List a ticket's communications (newest first), optionally by direction",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.CommunicationManager == nil {
				return fmt.Errorf("app not initialized")
			}
			if strings.TrimSpace(taskID) == "" {
				return fmt.Errorf("--task is required")
			}
			var wantDir models.CommunicationDirection
			if d := strings.TrimSpace(direction); d != "" {
				wantDir = models.CommunicationDirection(d)
				if !wantDir.IsValid() {
					return fmt.Errorf("--direction must be inbound or outbound")
				}
			}
			comms, err := App.CommunicationManager.GetAllCommunications(taskID)
			if err != nil {
				return fmt.Errorf("list communications: %w", err)
			}
			if wantDir != "" {
				filtered := comms[:0:0]
				for _, c := range comms {
					if c.Direction == wantDir {
						filtered = append(filtered, c)
					}
				}
				comms = filtered
			}
			if jsonOutput {
				return printJSON(comms)
			}
			if len(comms) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No communications on %s.\n", taskID)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DATE\tDIRECTION\tCHANNEL\tFROM\tSUBJECT")
			for _, c := range comms {
				dir := string(c.Direction)
				if dir == "" {
					dir = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					c.Date.Local().Format("2006-01-02"), dir, dashIfEmpty(c.Channel), dashIfEmpty(c.From), dashIfEmpty(c.Subject))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&taskID, "task", "", "ticket id to list communications for (required)")
	cmd.Flags().StringVar(&direction, "direction", "", "filter to inbound or outbound only")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// newCommID mints a timestamp-based communication id. The stored filename is
// primarily date+subject; the id is a stable fallback identifier.
func newCommID() string {
	return "comm-" + time.Now().UTC().Format("20060102T150405.000000000")
}

func subjectSuffix(subject string) string {
	if subject == "" {
		return ""
	}
	return ": " + subject
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
