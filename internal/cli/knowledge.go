package cli

import (
	"fmt"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// KnowledgeMgr is set during app initialization in app.go.
var KnowledgeMgr core.KnowledgeManager

// KnowledgeX is the knowledge extractor, set during app initialization in app.go.
var KnowledgeX core.KnowledgeExtractor

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Query and manage accumulated project knowledge",
	Long: `Query, add, and explore knowledge accumulated across task lifecycles.

Knowledge entries are decisions, learnings, patterns, and gotchas extracted
from completed tasks and stored in docs/knowledge/.`,
}

var knowledgeQueryCmd = &cobra.Command{
	Use:   "query <search-term>",
	Short: "Search accumulated knowledge",
	Long:  `Search across all knowledge entries by keyword. Matches against summary, detail, topic, tags, and entities.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if KnowledgeMgr == nil {
			return fmt.Errorf("knowledge manager not initialized")
		}

		queryType, _ := cmd.Flags().GetString("type")
		entries, err := runQuery(args[0], queryType)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Printf("No knowledge found for %q.\n", args[0])
			return nil
		}

		fmt.Printf("%d result(s) for %q:\n\n", len(entries), args[0])
		printEntries(entries)
		return nil
	},
}

func runQuery(term, queryType string) ([]models.KnowledgeEntry, error) {
	switch queryType {
	case "topic":
		return KnowledgeMgr.QueryByTopic(term)
	case "entity":
		return KnowledgeMgr.QueryByEntity(term)
	case "tag":
		return KnowledgeMgr.QueryByTags([]string{term})
	default:
		return KnowledgeMgr.Search(term)
	}
}

var knowledgeAddCmd = &cobra.Command{
	Use:   "add <summary>",
	Short: "Manually add a knowledge entry",
	Long:  `Add a knowledge entry manually. Use flags to set type, topic, tags, and entities.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if KnowledgeMgr == nil {
			return fmt.Errorf("knowledge manager not initialized")
		}

		entryType, _ := cmd.Flags().GetString("type")
		topic, _ := cmd.Flags().GetString("topic")
		detail, _ := cmd.Flags().GetString("detail")
		tagsStr, _ := cmd.Flags().GetString("tags")
		entitiesStr, _ := cmd.Flags().GetString("entities")
		taskID, _ := cmd.Flags().GetString("task")

		var tags []string
		if tagsStr != "" {
			tags = strings.Split(tagsStr, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
		}

		var entities []string
		if entitiesStr != "" {
			entities = strings.Split(entitiesStr, ",")
			for i := range entities {
				entities[i] = strings.TrimSpace(entities[i])
			}
		}

		kt := models.KnowledgeEntryType(entryType)
		id, err := KnowledgeMgr.AddKnowledge(
			kt,
			topic,
			args[0],
			detail,
			taskID,
			models.SourceManual,
			entities,
			tags,
		)
		if err != nil {
			return fmt.Errorf("adding knowledge: %w", err)
		}

		fmt.Printf("Knowledge entry %s created.\n", id)
		return nil
	},
}

var knowledgeTopicsCmd = &cobra.Command{
	Use:   "topics",
	Short: "List knowledge topics and their relationships",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if KnowledgeMgr == nil {
			return fmt.Errorf("knowledge manager not initialized")
		}

		graph, err := KnowledgeMgr.ListTopics()
		if err != nil {
			return fmt.Errorf("listing topics: %w", err)
		}

		if len(graph.Topics) == 0 {
			fmt.Println("No topics found. Knowledge topics are created when entries are added with a --topic flag.")
			return nil
		}

		fmt.Printf("%d topic(s):\n\n", len(graph.Topics))
		fmt.Printf("  %-25s %-40s %s  %s\n", "TOPIC", "DESCRIPTION", "ENTRIES", "TASKS")
		fmt.Printf("  %-25s %-40s %s  %s\n", "-----", "-----------", "-------", "-----")
		for _, topic := range graph.Topics {
			taskList := strings.Join(topic.Tasks, ", ")
			fmt.Printf("  %-25s %-40s %d       %s\n",
				topic.Name, truncate(topic.Description, 38), topic.EntryCount, taskList)
		}
		return nil
	},
}

var knowledgeTimelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show chronological knowledge trail",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if KnowledgeMgr == nil {
			return fmt.Errorf("knowledge manager not initialized")
		}

		sinceStr, _ := cmd.Flags().GetString("since")
		since, err := parseSinceDuration(sinceStr)
		if err != nil {
			return err
		}

		entries, err := KnowledgeMgr.GetTimeline(since)
		if err != nil {
			return fmt.Errorf("getting timeline: %w", err)
		}

		if len(entries) == 0 {
			fmt.Printf("No knowledge events since %s.\n", since.Format("2006-01-02"))
			return nil
		}

		fmt.Printf("Knowledge timeline (%d events since %s):\n\n", len(entries), since.Format("2006-01-02"))
		for _, entry := range entries {
			taskRef := ""
			if entry.Task != "" {
				taskRef = fmt.Sprintf(" [%s]", entry.Task)
			}
			fmt.Printf("  %s  %s%s\n", entry.Date, entry.Event, taskRef)
		}
		return nil
	},
}

func printEntries(entries []models.KnowledgeEntry) {
	for _, e := range entries {
		fmt.Printf("  %s [%s] %s\n", e.ID, e.Type, e.Summary)
		if e.Topic != "" {
			fmt.Printf("    topic: %s\n", e.Topic)
		}
		if e.SourceTask != "" {
			fmt.Printf("    source: %s (%s)\n", e.SourceTask, e.SourceType)
		}
		if len(e.Tags) > 0 {
			fmt.Printf("    tags: %s\n", strings.Join(e.Tags, ", "))
		}
		if len(e.Entities) > 0 {
			fmt.Printf("    entities: %s\n", strings.Join(e.Entities, ", "))
		}
		fmt.Println()
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	knowledgeQueryCmd.Flags().String("type", "", "Query type: topic, entity, tag, or empty for keyword search")
	knowledgeAddCmd.Flags().String("type", "learning", "Entry type: decision, learning, pattern, gotcha, relationship")
	knowledgeAddCmd.Flags().String("topic", "", "Topic/theme for this entry")
	knowledgeAddCmd.Flags().String("detail", "", "Detailed description")
	knowledgeAddCmd.Flags().String("tags", "", "Comma-separated tags")
	knowledgeAddCmd.Flags().String("entities", "", "Comma-separated entities (people, systems)")
	knowledgeAddCmd.Flags().String("task", "", "Source task ID")
	knowledgeTimelineCmd.Flags().String("since", "30d", "Time window: 7d, 30d, 24h")

	knowledgeCmd.AddCommand(knowledgeQueryCmd)
	knowledgeCmd.AddCommand(knowledgeAddCmd)
	knowledgeCmd.AddCommand(knowledgeTopicsCmd)
	knowledgeCmd.AddCommand(knowledgeTimelineCmd)
	rootCmd.AddCommand(knowledgeCmd)
}
