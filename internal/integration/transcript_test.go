package integration

import (
	"strings"
	"testing"
	"time"
)

func TestNewTranscriptParser(t *testing.T) {
	parser := NewTranscriptParser()
	if parser == nil {
		t.Error("expected non-nil parser")
	}
}

func TestParseEmptyTranscript(t *testing.T) {
	parser := NewTranscriptParser()
	reader := strings.NewReader("")

	result, err := parser.Parse(reader)
	if err != nil {
		t.Errorf("unexpected error for empty transcript: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
	if len(result.Turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(result.Turns))
	}
}

func TestParseBasicTranscript(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z"}
{"role":"assistant","content":"Hi there!","timestamp":"2024-01-01T10:00:05Z"}
{"role":"user","content":"How are you?","timestamp":"2024-01-01T10:00:10Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Turns) != 3 {
		t.Errorf("expected 3 turns, got %d", len(result.Turns))
	}

	if result.TotalTurns != 3 {
		t.Errorf("expected TotalTurns=3, got %d", result.TotalTurns)
	}

	// Check first turn
	if result.Turns[0].Role != "user" {
		t.Errorf("expected role 'user', got '%s'", result.Turns[0].Role)
	}
	if result.Turns[0].Content != "Hello" {
		t.Errorf("expected content 'Hello', got '%s'", result.Turns[0].Content)
	}

	// Check time range
	if result.StartTime.IsZero() {
		t.Error("expected non-zero start time")
	}
	if result.EndTime.IsZero() {
		t.Error("expected non-zero end time")
	}
	if result.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", result.Duration)
	}
}

func TestParseTranscriptWithTools(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Read file","timestamp":"2024-01-01T10:00:00Z"}
{"role":"tool","content":"File contents","tool":"ReadFile","timestamp":"2024-01-01T10:00:01Z"}
{"role":"tool","content":"Done","tool":"ReadFile","timestamp":"2024-01-01T10:00:02Z"}
{"role":"tool","content":"Written","tool":"WriteFile","timestamp":"2024-01-01T10:00:03Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check tool stats
	if len(result.ToolStats) != 2 {
		t.Errorf("expected 2 tool stats, got %d", len(result.ToolStats))
	}

	// Check ReadFile stats
	if stat, exists := result.ToolStats["ReadFile"]; exists {
		if stat.Count != 2 {
			t.Errorf("expected ReadFile count=2, got %d", stat.Count)
		}
		if stat.ToolName != "ReadFile" {
			t.Errorf("expected tool name 'ReadFile', got '%s'", stat.ToolName)
		}
	} else {
		t.Error("expected ReadFile in tool stats")
	}

	// Check WriteFile stats
	if stat, exists := result.ToolStats["WriteFile"]; exists {
		if stat.Count != 1 {
			t.Errorf("expected WriteFile count=1, got %d", stat.Count)
		}
	} else {
		t.Error("expected WriteFile in tool stats")
	}
}

func TestParseTranscriptWithMalformedLines(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z"}
invalid json line
{"role":"assistant","content":"Hi","timestamp":"2024-01-01T10:00:05Z"}
{malformed}
{"role":"user","content":"Bye","timestamp":"2024-01-01T10:00:10Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have 3 valid turns, malformed lines should be skipped
	if len(result.Turns) != 3 {
		t.Errorf("expected 3 valid turns, got %d", len(result.Turns))
	}
}

func TestParseTranscriptSchemaDetection(t *testing.T) {
	parser := NewTranscriptParser()

	// Test with explicit version
	transcript := `{"version":"v2.0","role":"system","content":"Start"}
{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z"}
{"role":"assistant","content":"Hi","timestamp":"2024-01-01T10:00:05Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.SchemaVersion != "v2.0" {
		t.Errorf("expected schema version 'v2.0', got '%s'", result.SchemaVersion)
	}
}

func TestParseTranscriptAlternativeFields(t *testing.T) {
	parser := NewTranscriptParser()

	// Test with alternative field names
	transcript := `{"type":"user","message":"Hello","time":"2024-01-01T10:00:00Z"}
{"type":"assistant","text":"Hi there","time":"2024-01-01T10:00:05Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Turns) != 2 {
		t.Errorf("expected 2 turns, got %d", len(result.Turns))
	}

	// Check that alternative fields were parsed
	if result.Turns[0].Role != "user" {
		t.Errorf("expected role 'user', got '%s'", result.Turns[0].Role)
	}
	if result.Turns[0].Content != "Hello" {
		t.Errorf("expected content 'Hello', got '%s'", result.Turns[0].Content)
	}
}

func TestParseTranscriptSummary(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z"}
{"role":"tool","content":"Done","tool":"TestTool","timestamp":"2024-01-01T10:00:01Z"}
{"role":"tool","content":"Done","tool":"TestTool","timestamp":"2024-01-01T10:00:02Z"}
{"role":"assistant","content":"Complete","timestamp":"2024-01-01T10:00:05Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check that summary is generated
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}

	// Summary should mention number of turns
	if !strings.Contains(result.Summary, "4 turns") {
		t.Errorf("expected summary to mention turns, got: %s", result.Summary)
	}

	// Summary should mention tools
	if !strings.Contains(result.Summary, "Tools used") {
		t.Errorf("expected summary to mention tools, got: %s", result.Summary)
	}

	// Summary should mention most used tool
	if !strings.Contains(result.Summary, "TestTool") {
		t.Errorf("expected summary to mention TestTool, got: %s", result.Summary)
	}
}

func TestParseTranscriptWithEmptyLines(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z"}

{"role":"assistant","content":"Hi","timestamp":"2024-01-01T10:00:05Z"}

`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Empty lines should be skipped
	if len(result.Turns) != 2 {
		t.Errorf("expected 2 turns, got %d", len(result.Turns))
	}
}

func TestParseTranscriptMetadata(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:00Z","custom_field":"value","number":42}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Turns) != 1 {
		t.Errorf("expected 1 turn, got %d", len(result.Turns))
	}

	// Check metadata
	turn := result.Turns[0]
	if len(turn.Metadata) == 0 {
		t.Error("expected metadata to be populated")
	}

	if val, ok := turn.Metadata["custom_field"].(string); !ok || val != "value" {
		t.Errorf("expected custom_field='value' in metadata, got %v", turn.Metadata["custom_field"])
	}
}

func TestParseTranscriptLargeBuffer(t *testing.T) {
	parser := NewTranscriptParser()

	// Create a very long content string (larger than default buffer)
	longContent := strings.Repeat("a", 100000)
	transcript := `{"role":"user","content":"` + longContent + `","timestamp":"2024-01-01T10:00:00Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error for large content: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Turns) != 1 {
		t.Errorf("expected 1 turn, got %d", len(result.Turns))
	}

	if len(result.Turns[0].Content) != len(longContent) {
		t.Errorf("expected content length %d, got %d", len(longContent), len(result.Turns[0].Content))
	}
}

func TestToolStatsTimestamps(t *testing.T) {
	parser := NewTranscriptParser()

	transcript := `{"role":"tool","tool":"TestTool","content":"First","timestamp":"2024-01-01T10:00:00Z"}
{"role":"tool","tool":"TestTool","content":"Second","timestamp":"2024-01-01T10:00:05Z"}
{"role":"tool","tool":"TestTool","content":"Third","timestamp":"2024-01-01T10:00:03Z"}`

	reader := strings.NewReader(transcript)
	result, err := parser.Parse(reader)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	stat, exists := result.ToolStats["TestTool"]
	if !exists {
		t.Fatal("expected TestTool in tool stats")
	}

	if stat.Count != 3 {
		t.Errorf("expected count=3, got %d", stat.Count)
	}

	// Check that FirstUsed is the earliest timestamp
	expectedFirst, _ := time.Parse(time.RFC3339, "2024-01-01T10:00:00Z")
	if !stat.FirstUsed.Equal(expectedFirst) {
		t.Errorf("expected FirstUsed=%v, got %v", expectedFirst, stat.FirstUsed)
	}

	// Check that LastUsed is the latest timestamp
	expectedLast, _ := time.Parse(time.RFC3339, "2024-01-01T10:00:05Z")
	if !stat.LastUsed.Equal(expectedLast) {
		t.Errorf("expected LastUsed=%v, got %v", expectedLast, stat.LastUsed)
	}
}
