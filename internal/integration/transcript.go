package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// TranscriptTurn represents a single turn in the transcript
type TranscriptTurn struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolUsageStats tracks tool usage statistics
type ToolUsageStats struct {
	ToolName  string    `json:"tool_name"`
	Count     int       `json:"count"`
	FirstUsed time.Time `json:"first_used"`
	LastUsed  time.Time `json:"last_used"`
}

// TranscriptResult represents the parsed transcript data
type TranscriptResult struct {
	Turns         []TranscriptTurn           `json:"turns"`
	Summary       string                     `json:"summary"`
	StartTime     time.Time                  `json:"start_time"`
	EndTime       time.Time                  `json:"end_time"`
	Duration      time.Duration              `json:"duration"`
	ToolStats     map[string]*ToolUsageStats `json:"tool_stats"`
	SchemaVersion string                     `json:"schema_version"`
	TotalTurns    int                        `json:"total_turns"`
}

// TranscriptParser parses Claude Code JSONL session transcripts
type TranscriptParser interface {
	// Parse parses a JSONL transcript from a reader
	Parse(reader io.Reader) (*TranscriptResult, error)
}

// DefaultTranscriptParser implements TranscriptParser
type DefaultTranscriptParser struct {
	initialBufferSize int
	maxBufferSize     int
}

// NewTranscriptParser creates a new transcript parser with specified buffer sizes
func NewTranscriptParser() TranscriptParser {
	return &DefaultTranscriptParser{
		initialBufferSize: 64 * 1024,        // 64KB initial
		maxBufferSize:     10 * 1024 * 1024, // 10MB max
	}
}

// Parse parses a JSONL transcript from a reader
func (p *DefaultTranscriptParser) Parse(reader io.Reader) (*TranscriptResult, error) {
	result := &TranscriptResult{
		Turns:     []TranscriptTurn{},
		ToolStats: make(map[string]*ToolUsageStats),
	}

	// Create scanner with large buffer
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, p.initialBufferSize)
	scanner.Buffer(buffer, p.maxBufferSize)

	lineNum := 0
	schemaDetected := false
	var firstLines []string

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue // Skip empty lines
		}

		// Collect first 5 non-empty lines for schema detection
		if len(firstLines) < 5 {
			firstLines = append(firstLines, line)
		}

		// Detect schema version after collecting 5 lines
		if !schemaDetected && len(firstLines) == 5 {
			result.SchemaVersion = p.detectSchemaVersion(firstLines)
			schemaDetected = true
		}

		// Parse the JSON line
		var rawData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rawData); err != nil {
			// Skip malformed lines but continue parsing
			continue
		}

		// Extract turn information
		turn := p.extractTurn(rawData)
		if turn != nil {
			result.Turns = append(result.Turns, *turn)

			// Update time range
			if result.StartTime.IsZero() || turn.Timestamp.Before(result.StartTime) {
				result.StartTime = turn.Timestamp
			}
			if turn.Timestamp.After(result.EndTime) {
				result.EndTime = turn.Timestamp
			}

			// Track tool usage
			p.updateToolStats(rawData, turn.Timestamp, result.ToolStats)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading transcript: %w", err)
	}

	// Detect schema if not yet detected (for transcripts with < 5 lines)
	if !schemaDetected && len(firstLines) > 0 {
		result.SchemaVersion = p.detectSchemaVersion(firstLines)
		schemaDetected = true
	}

	// Calculate duration
	if !result.StartTime.IsZero() && !result.EndTime.IsZero() {
		result.Duration = result.EndTime.Sub(result.StartTime)
	}

	result.TotalTurns = len(result.Turns)

	// Generate structural summary
	result.Summary = p.generateSummary(result)

	// Default schema version if not detected
	if result.SchemaVersion == "" {
		result.SchemaVersion = "unknown"
	}

	return result, nil
}

// detectSchemaVersion detects the schema version from the first 5 lines
func (p *DefaultTranscriptParser) detectSchemaVersion(lines []string) string {
	// Look for version indicators in the first few lines
	for _, line := range lines {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		// Check for explicit version field
		if version, ok := data["version"].(string); ok {
			return version
		}
		if version, ok := data["schema_version"].(string); ok {
			return version
		}

		// Infer version from structure
		if _, hasType := data["type"]; hasType {
			if _, hasContent := data["content"]; hasContent {
				return "v1.0"
			}
		}
	}

	return "v1.0" // Default version
}

// extractTurn extracts a TranscriptTurn from raw JSON data
func (p *DefaultTranscriptParser) extractTurn(data map[string]interface{}) *TranscriptTurn {
	turn := &TranscriptTurn{
		Metadata: make(map[string]interface{}),
	}

	// Extract role
	if role, ok := data["role"].(string); ok {
		turn.Role = role
	} else if role, ok := data["type"].(string); ok {
		turn.Role = role
	}

	// Extract content
	if content, ok := data["content"].(string); ok {
		turn.Content = content
	} else if message, ok := data["message"].(string); ok {
		turn.Content = message
	} else if text, ok := data["text"].(string); ok {
		turn.Content = text
	}

	// Extract timestamp
	if ts, ok := data["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			turn.Timestamp = parsed
		}
	} else if ts, ok := data["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			turn.Timestamp = parsed
		}
	}

	// If no timestamp, use current time
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now()
	}

	// Store other fields in metadata
	for key, value := range data {
		if key != "role" && key != "type" && key != "content" && key != "message" && key != "text" && key != "timestamp" && key != "time" {
			turn.Metadata[key] = value
		}
	}

	// Only return valid turns with content
	if turn.Role != "" || turn.Content != "" {
		return turn
	}

	return nil
}

// updateToolStats updates tool usage statistics
func (p *DefaultTranscriptParser) updateToolStats(data map[string]interface{}, timestamp time.Time, stats map[string]*ToolUsageStats) {
	// Look for tool usage indicators
	var toolName string

	if tool, ok := data["tool"].(string); ok {
		toolName = tool
	} else if toolType, ok := data["tool_type"].(string); ok {
		toolName = toolType
	} else if function, ok := data["function"].(string); ok {
		toolName = function
	} else if name, ok := data["name"].(string); ok {
		if role, ok := data["role"].(string); ok && role == "tool" {
			toolName = name
		}
	}

	if toolName == "" {
		return
	}

	// Update or create tool stats
	if stat, exists := stats[toolName]; exists {
		stat.Count++
		if timestamp.After(stat.LastUsed) {
			stat.LastUsed = timestamp
		}
		if timestamp.Before(stat.FirstUsed) {
			stat.FirstUsed = timestamp
		}
	} else {
		stats[toolName] = &ToolUsageStats{
			ToolName:  toolName,
			Count:     1,
			FirstUsed: timestamp,
			LastUsed:  timestamp,
		}
	}
}

// StructuralSummarizer generates non-LLM summaries from transcript data
type StructuralSummarizer interface {
	Summarize(result *TranscriptResult) string
}

// generateSummary generates a structural (non-LLM) summary of the transcript
func (p *DefaultTranscriptParser) generateSummary(result *TranscriptResult) string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Session with %d turns", result.TotalTurns))

	if !result.StartTime.IsZero() {
		summary.WriteString(fmt.Sprintf(", started at %s", result.StartTime.Format("15:04:05")))
	}

	if result.Duration > 0 {
		summary.WriteString(fmt.Sprintf(", duration: %s", p.formatDuration(result.Duration)))
	}

	if len(result.ToolStats) > 0 {
		summary.WriteString(fmt.Sprintf(". Tools used: %d", len(result.ToolStats)))

		// Find most used tool
		var mostUsedTool string
		maxCount := 0
		for _, stat := range result.ToolStats {
			if stat.Count > maxCount {
				maxCount = stat.Count
				mostUsedTool = stat.ToolName
			}
		}
		if mostUsedTool != "" {
			summary.WriteString(fmt.Sprintf(", most used: %s (%d times)", mostUsedTool, maxCount))
		}
	}

	summary.WriteString(".")

	return summary.String()
}

// formatDuration formats a duration in a human-readable way
func (p *DefaultTranscriptParser) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
