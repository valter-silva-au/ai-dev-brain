package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage session summaries for tasks",
	Long:  `Commands for saving and managing session summaries that capture work progress between sessions.`,
}

var sessionSaveCmd = &cobra.Command{
	Use:   "save [task-id]",
	Short: "Save a session summary for the current task",
	Long: `Save a structured session summary to the task's sessions/ directory.

If no task-id is provided, the ADB_TASK_ID environment variable is used.
The session file is timestamped and contains sections for accomplished work,
decisions, blockers, and next steps.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := ""
		if len(args) > 0 {
			taskID = args[0]
		} else {
			taskID = os.Getenv("ADB_TASK_ID")
		}

		if taskID == "" {
			return fmt.Errorf("task ID required: provide as argument or set ADB_TASK_ID")
		}

		if BasePath == "" {
			return fmt.Errorf("base path not initialized")
		}

		ticketPath := core.ResolveTicketDir(BasePath, taskID)
		if _, err := os.Stat(ticketPath); err != nil {
			return fmt.Errorf("task %s not found: %w", taskID, err)
		}

		sessionsDir := filepath.Join(ticketPath, "sessions")
		if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
			return fmt.Errorf("creating sessions directory: %w", err)
		}

		// Read context.md to include current state in the session summary.
		contextPath := filepath.Join(ticketPath, "context.md")
		contextSummary := ""
		if data, err := os.ReadFile(contextPath); err == nil {
			contextSummary = string(data)
		}

		now := time.Now().UTC()
		filename := now.Format("2006-01-02T15-04-05Z") + ".md"
		sessionPath := filepath.Join(sessionsDir, filename)

		content := fmt.Sprintf(`# Session: %s

**Task:** %s
**Date:** %s

## Accomplished

- (describe what was completed this session)

## Decisions

- (record any decisions made)

## Blockers

- (note any blockers encountered)

## Next Steps

- (list what should happen next)
`, now.Format(time.RFC3339), taskID, now.Format("2006-01-02"))

		if contextSummary != "" {
			content += fmt.Sprintf(`
## Context Snapshot

%s
`, contextSummary)
		}

		if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing session file: %w", err)
		}

		fmt.Printf("Session saved: %s\n", sessionPath)
		fmt.Printf("Edit the file to fill in session details.\n")

		return nil
	},
}

var sessionIngestCmd = &cobra.Command{
	Use:   "ingest [task-id]",
	Short: "Ingest knowledge from the latest session file",
	Long: `Read the latest session file for a task and ingest any decisions and
learnings into the knowledge store.

If no task-id is provided, the ADB_TASK_ID environment variable is used.
Placeholder items (from the initial template) are ignored.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := ""
		if len(args) > 0 {
			taskID = args[0]
		} else {
			taskID = os.Getenv("ADB_TASK_ID")
		}

		if taskID == "" {
			return fmt.Errorf("task ID required: provide as argument or set ADB_TASK_ID")
		}

		if BasePath == "" {
			return fmt.Errorf("base path not initialized")
		}

		if KnowledgeMgr == nil {
			return fmt.Errorf("knowledge manager not initialized")
		}

		ticketPath := core.ResolveTicketDir(BasePath, taskID)
		if _, err := os.Stat(ticketPath); err != nil {
			return fmt.Errorf("task %s not found: %w", taskID, err)
		}

		sessionsDir := filepath.Join(ticketPath, "sessions")
		sessionPath, err := findLatestSessionFile(sessionsDir)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(sessionPath)
		if err != nil {
			return fmt.Errorf("reading session file: %w", err)
		}

		decisions, learnings := parseSessionSections(string(data))

		if len(decisions) == 0 && len(learnings) == 0 {
			fmt.Println("No knowledge entries found in session file.")
			fmt.Printf("Edit %s and add items under ## Decisions or ## Accomplished, then run ingest again.\n", sessionPath)
			return nil
		}

		ingested := 0
		for _, d := range decisions {
			_, err := KnowledgeMgr.AddKnowledge(
				models.KnowledgeTypeDecision,
				"",
				d,
				"",
				taskID,
				models.SourceSession,
				nil,
				nil,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to ingest decision: %v\n", err)
				continue
			}
			ingested++
		}

		for _, l := range learnings {
			_, err := KnowledgeMgr.AddKnowledge(
				models.KnowledgeTypeLearning,
				"",
				l,
				"",
				taskID,
				models.SourceSession,
				nil,
				nil,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to ingest learning: %v\n", err)
				continue
			}
			ingested++
		}

		fmt.Printf("Ingested %d knowledge entry(s) from %s (%d decision(s), %d learning(s)).\n",
			ingested, filepath.Base(sessionPath), len(decisions), len(learnings))
		return nil
	},
}

// findLatestSessionFile returns the path to the most recent .md file in the
// sessions directory, determined by lexicographic sorting of filenames.
func findLatestSessionFile(sessionsDir string) (string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", fmt.Errorf("reading sessions directory: %w", err)
	}

	var mdFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			mdFiles = append(mdFiles, entry.Name())
		}
	}

	if len(mdFiles) == 0 {
		return "", fmt.Errorf("no session files found in %s", sessionsDir)
	}

	sort.Strings(mdFiles)
	return filepath.Join(sessionsDir, mdFiles[len(mdFiles)-1]), nil
}

// parseSessionSections extracts non-placeholder list items from the
// "## Decisions" and "## Accomplished" sections of a session file.
func parseSessionSections(content string) (decisions, learnings []string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := ""

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimPrefix(line, "## ")
			heading = strings.TrimSpace(heading)
			switch {
			case strings.EqualFold(heading, "Decisions"):
				currentSection = "decisions"
			case strings.EqualFold(heading, "Accomplished"):
				currentSection = "accomplished"
			default:
				currentSection = ""
			}
			continue
		}

		if currentSection == "" {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}

		item := strings.TrimPrefix(trimmed, "- ")
		item = strings.TrimSpace(item)

		if item == "" || isPlaceholder(item) {
			continue
		}

		switch currentSection {
		case "decisions":
			decisions = append(decisions, item)
		case "accomplished":
			learnings = append(learnings, item)
		}
	}

	return decisions, learnings
}

// isPlaceholder returns true if the item text looks like a template placeholder.
func isPlaceholder(item string) bool {
	lower := strings.ToLower(item)
	placeholderPrefixes := []string{
		"(describe",
		"(record",
		"(note",
		"(list",
	}
	for _, prefix := range placeholderPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func init() {
	sessionCmd.AddCommand(sessionSaveCmd)
	sessionCmd.AddCommand(sessionIngestCmd)
	rootCmd.AddCommand(sessionCmd)
}
