package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// TranscriptResult holds the parsed output of a Claude Code JSONL transcript.
type TranscriptResult struct {
	Turns     []models.SessionTurn
	Summary   string
	StartedAt time.Time
	EndedAt   time.Time
	TurnCount int
	ToolsUsed map[string]int
}

// TranscriptParser parses Claude Code JSONL session transcripts into structured turn data.
type TranscriptParser interface {
	ParseTranscript(filePath string) (*TranscriptResult, error)
}

// transcriptParser implements TranscriptParser.
type transcriptParser struct{}

// NewTranscriptParser creates a new TranscriptParser.
func NewTranscriptParser() TranscriptParser {
	return &transcriptParser{}
}

// jsonlLine represents a single line in the Claude Code JSONL transcript.
type jsonlLine struct {
	Type      string          `json:"type"`
	IsMeta    bool            `json:"isMeta,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	Timestamp string          `json:"timestamp,omitempty"`
	UUID      string          `json:"uuid,omitempty"`
}

// jsonlMessage represents the message field in a JSONL line.
type jsonlMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBlock represents a typed content block within a message.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

// ParseTranscript reads a Claude Code JSONL transcript file and returns
// structured turn data, a summary, timestamps, and tool usage statistics.
func (p *transcriptParser) ParseTranscript(filePath string) (*TranscriptResult, error) {
	f, err := os.Open(filePath) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("opening transcript %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	result := &TranscriptResult{
		ToolsUsed: make(map[string]int),
	}

	turnIndex := 0
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large JSONL lines (e.g., assistant responses with tool output).
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlLine
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines.
			continue
		}

		switch entry.Type {
		case "summary":
			if entry.Summary != "" {
				result.Summary = entry.Summary
			}

		case "user":
			if entry.IsMeta {
				continue
			}
			text := extractMessageText(entry.Message)
			if text == "" {
				continue
			}

			ts := parseTimestamp(entry.Timestamp)
			turn := models.SessionTurn{
				Index:     turnIndex,
				Role:      "user",
				Timestamp: ts,
				Content:   text,
				Digest:    generateDigest(text),
			}
			result.Turns = append(result.Turns, turn)
			turnIndex++

		case "assistant":
			text, tools := extractAssistantContent(entry.Message)
			if text == "" && len(tools) == 0 {
				continue
			}

			ts := parseTimestamp(entry.Timestamp)
			turn := models.SessionTurn{
				Index:     turnIndex,
				Role:      "assistant",
				Timestamp: ts,
				Content:   text,
				Digest:    generateDigest(text),
				ToolsUsed: tools,
			}
			result.Turns = append(result.Turns, turn)
			turnIndex++

			for _, tool := range tools {
				result.ToolsUsed[tool]++
			}

		default:
			// Skip unknown types (e.g., "file-history-snapshot").
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading transcript %s: %w", filePath, err)
	}

	result.TurnCount = len(result.Turns)

	// Determine time boundaries.
	if len(result.Turns) > 0 {
		result.StartedAt = result.Turns[0].Timestamp
		result.EndedAt = result.Turns[len(result.Turns)-1].Timestamp
	}

	return result, nil
}

// extractMessageText extracts the text content from a raw message JSON.
// message.content can be either a string or an array of content blocks.
func extractMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var msg jsonlMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	return extractTextFromContent(msg.Content)
}

// extractTextFromContent handles content that is either a plain string
// or an array of content blocks, returning concatenated text.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as a plain string first.
	var plainStr string
	if err := json.Unmarshal(raw, &plainStr); err == nil {
		return plainStr
	}

	// Try as an array of content blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// extractAssistantContent extracts text and tool names from an assistant message.
// It skips "thinking" blocks and collects tool names from "tool_use" blocks.
func extractAssistantContent(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}

	var msg jsonlMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", nil
	}

	if len(msg.Content) == 0 {
		return "", nil
	}

	// Content for assistants is typically an array of blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		// Fallback: try as plain string.
		var plainStr string
		if err := json.Unmarshal(msg.Content, &plainStr); err == nil {
			return plainStr, nil
		}
		return "", nil
	}

	var textParts []string
	var tools []string

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case "tool_use":
			if b.Name != "" {
				tools = append(tools, b.Name)
			}
		case "thinking":
			// Skip thinking blocks.
		}
	}

	return strings.Join(textParts, "\n"), tools
}

// parseTimestamp parses an ISO 8601 timestamp string, falling back to zero time.
func parseTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// generateDigest produces a short digest from content: first 200 characters, trimmed.
func generateDigest(content string) string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) <= 200 {
		return trimmed
	}
	return trimmed[:200]
}

// StructuralSummarizer generates a summary from session turns without LLM.
type StructuralSummarizer struct{}

// Summarize produces a structural summary from session turns.
// Format: "{first_user_message_truncated_to_100_chars} ({N} turns, tools: Read({X}), Edit({Y}), ...)"
func (s *StructuralSummarizer) Summarize(turns []models.SessionTurn) string {
	if len(turns) == 0 {
		return "(empty session)"
	}

	// Find the first user message as the topic.
	var topic string
	for _, t := range turns {
		if t.Role == "user" && strings.TrimSpace(t.Content) != "" {
			topic = strings.TrimSpace(t.Content)
			break
		}
	}

	if topic == "" {
		topic = "(no user message)"
	}

	// Truncate topic to 100 chars.
	if len(topic) > 100 {
		topic = topic[:100]
	}

	// Count tools across all assistant turns.
	toolCounts := make(map[string]int)
	for _, t := range turns {
		if t.Role == "assistant" {
			for _, tool := range t.ToolsUsed {
				toolCounts[tool]++
			}
		}
	}

	// Build tool summary sorted by count descending, then by name.
	type toolEntry struct {
		name  string
		count int
	}
	var entries []toolEntry
	for name, count := range toolCounts {
		entries = append(entries, toolEntry{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].name < entries[j].name
	})

	var toolParts []string
	for _, e := range entries {
		toolParts = append(toolParts, fmt.Sprintf("%s(%d)", e.name, e.count))
	}

	if len(toolParts) > 0 {
		return fmt.Sprintf("%s (%d turns, tools: %s)", topic, len(turns), strings.Join(toolParts, ", "))
	}
	return fmt.Sprintf("%s (%d turns)", topic, len(turns))
}
