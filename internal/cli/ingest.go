package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// NewIngestCmd creates the `adb ingest` command group — the staged ingestion
// pipeline (decision D8). Connectors LAND immutable raw content (provenance +
// hash/cursor dedup); an extraction skill PROPOSES typed nodes/edges; proposals
// are confidence-gated (auto-land the certain, queue the fuzzy for REVIEW).
func NewIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Staged ingestion: land raw sources, propose graph entities, review them",
		Long: `Staged ingestion pipeline (decision D8):

  land     immutable raw/ landing with provenance (source, hash, cursor) + dedup
  raw      list landed raw artifacts (the provenance ledger)
  propose  submit an extraction skill's proposed nodes/edges (confidence-gated)
  review   list proposals awaiting a decision (the review queue)
  accept   apply a queued proposal to the graph
  reject   drop a queued proposal

High-confidence proposals auto-land; fuzzy ones queue for review. Every derived
node/edge traces back to its raw artifact (provenance). The review queue is CLI
today; a VS Code webview review surface is the documented future path (it would
shell the same accept/reject commands, per the extension's thin-front-end model).`,
	}
	cmd.AddCommand(
		newIngestLandCmd(),
		newIngestRawCmd(),
		newIngestProposeCmd(),
		newIngestReviewCmd(),
		newIngestAcceptCmd(),
		newIngestRejectCmd(),
	)
	return cmd
}

func newIngestLandCmd() *cobra.Command {
	var (
		source string
		cursor string
		file   string
	)
	cmd := &cobra.Command{
		Use:   "land --source <source> [--cursor <c>] [--file <path>]",
		Short: "Land raw source content immutably (reads --file or stdin)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			if source == "" {
				return fmt.Errorf("--source is required, e.g. --source slack:C123 or --source file:notes.md")
			}
			var (
				content []byte
				err     error
			)
			if file != "" {
				content, err = os.ReadFile(file)
			} else {
				content, err = io.ReadAll(cmd.InOrStdin())
			}
			if err != nil {
				return fmt.Errorf("read content: %w", err)
			}
			art, landed, err := App.IngestManager.Land(source, cursor, content)
			if err != nil {
				return fmt.Errorf("land: %w", err)
			}
			if !landed {
				fmt.Fprintf(cmd.OutOrStdout(), "• Already landed (dedup): %s from %s\n", art.ID, art.Source)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Landed %s (%d bytes) at %s\n", art.ID, len(content), art.ContentPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "connector/source id, e.g. slack:C123, file:notes.md (required)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "per-source dedup marker (message id, timestamp, offset)")
	cmd.Flags().StringVar(&file, "file", "", "read content from this file instead of stdin")
	return cmd
}

func newIngestRawCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "raw",
		Short: "List landed raw artifacts (the provenance ledger)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			arts, err := App.IngestManager.RawArtifacts()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(arts)
			}
			if len(arts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No raw artifacts landed yet.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSOURCE\tCURSOR\tCONTENT")
			for _, a := range arts {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.ID, a.Source, a.Cursor, a.ContentPath)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newIngestProposeCmd() *cobra.Command {
	var (
		file      string
		threshold float64
	)
	cmd := &cobra.Command{
		Use:   "propose --file <proposals.yaml>",
		Short: "Submit an extraction skill's proposed nodes/edges (confidence-gated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			var (
				data []byte
				err  error
			)
			if file != "" {
				data, err = os.ReadFile(file)
			} else {
				data, err = io.ReadAll(cmd.InOrStdin())
			}
			if err != nil {
				return fmt.Errorf("read proposals: %w", err)
			}
			var doc models.ProposalQueue
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("parse proposals (want a `proposals:` list): %w", err)
			}
			if len(doc.Proposals) == 0 {
				return fmt.Errorf("no proposals found (expected a top-level `proposals:` list)")
			}
			res, err := App.IngestManager.Submit(doc.Proposals, threshold)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "✓ Submitted %d proposal(s): %d auto-landed (≥%.2f), %d queued for review.\n",
				len(doc.Proposals), len(res.Landed), threshold, len(res.Queued))
			// A high-confidence proposal that could not apply (e.g. an edge whose
			// from-entity doesn't exist yet) falls back to the review queue instead
			// of aborting the batch (#173) — surface it so it isn't a silent gap.
			if len(res.Requeued) > 0 {
				fmt.Fprintf(out, "⚠ %d high-confidence proposal(s) could not auto-land and were queued for review instead (fix the cause, then `adb ingest accept`):\n", len(res.Requeued))
				for _, p := range res.Requeued {
					fmt.Fprintf(out, "  · %s (%s)\n", p.ID, p.Kind)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read proposals YAML from this file instead of stdin")
	cmd.Flags().Float64Var(&threshold, "threshold", 0.8, "confidence at/above which a proposal auto-lands")
	return cmd
}

func newIngestReviewCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "List proposals awaiting a decision (the review queue)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			pending, err := App.IngestManager.Pending()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(pending)
			}
			if len(pending) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Review queue is empty.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tCONF\tKIND\tPROPOSAL\tPROVENANCE")
			for _, p := range pending {
				fmt.Fprintf(w, "%s\t%.2f\t%s\t%s\t%s\n", p.ID, p.Confidence, p.Kind, proposalSummary(p), p.RawID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func proposalSummary(p models.EntityProposal) string {
	switch p.Kind {
	case models.ProposalEdge:
		if p.Edge != nil {
			return fmt.Sprintf("%s --%s--> %s", p.From, p.Edge.Type, p.Edge.Target)
		}
	case models.ProposalNode:
		if p.Node != nil {
			return fmt.Sprintf("%s (%s) %s", p.Node.ID, p.Node.Type, p.Node.Title)
		}
	}
	return "-"
}

func newIngestAcceptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accept <id>",
		Short: "Apply a queued proposal to the graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			p, err := App.IngestManager.Accept(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Accepted %s: %s\n", p.ID, proposalSummary(p))
			return nil
		},
	}
	return cmd
}

func newIngestRejectCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "Drop a queued proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.IngestManager == nil {
				return fmt.Errorf("app not initialized")
			}
			p, err := App.IngestManager.Reject(args[0], reason)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Rejected %s%s\n", p.ID, reasonSuffix(reason))
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the proposal was rejected (recorded in the ledger)")
	return cmd
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}
