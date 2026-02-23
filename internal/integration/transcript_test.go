package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func writeTranscript(t *testing.T, dir string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, "transcript.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test transcript: %v", err)
	}
	return path
}

func TestTranscriptParser_ValidTranscript(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":"Hello, help me debug this"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will help you debug."},{"type":"tool_use","name":"Read"}]},"timestamp":"2025-01-15T10:01:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"Thanks, it works now"},"timestamp":"2025-01-15T10:02:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TurnCount != 3 {
		t.Errorf("expected 3 turns, got %d", result.TurnCount)
	}

	// First turn: user.
	if result.Turns[0].Role != "user" {
		t.Errorf("expected first turn role 'user', got %s", result.Turns[0].Role)
	}
	if result.Turns[0].Content != "Hello, help me debug this" {
		t.Errorf("unexpected first turn content: %s", result.Turns[0].Content)
	}

	// Second turn: assistant with tool.
	if result.Turns[1].Role != "assistant" {
		t.Errorf("expected second turn role 'assistant', got %s", result.Turns[1].Role)
	}
	if len(result.Turns[1].ToolsUsed) != 1 || result.Turns[1].ToolsUsed[0] != "Read" {
		t.Errorf("expected tools_used [Read], got %v", result.Turns[1].ToolsUsed)
	}

	// Tool usage stats.
	if result.ToolsUsed["Read"] != 1 {
		t.Errorf("expected Read tool count 1, got %d", result.ToolsUsed["Read"])
	}

	// Timestamps.
	if result.StartedAt.IsZero() {
		t.Error("expected non-zero start time")
	}
	if result.EndedAt.IsZero() {
		t.Error("expected non-zero end time")
	}
}

func TestTranscriptParser_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnCount != 0 {
		t.Errorf("expected 0 turns for empty file, got %d", result.TurnCount)
	}
}

func TestTranscriptParser_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{not valid json}`,
		`{"type":"user","message":{"role":"user","content":"valid message"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`another bad line`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip malformed lines and parse the valid one.
	if result.TurnCount != 1 {
		t.Errorf("expected 1 turn (skipping malformed), got %d", result.TurnCount)
	}
}

func TestTranscriptParser_SkipsMetaMessages(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"meta init message"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"real user message"},"timestamp":"2025-01-15T10:01:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnCount != 1 {
		t.Errorf("expected 1 turn (skipping meta), got %d", result.TurnCount)
	}
	if result.Turns[0].Content != "real user message" {
		t.Errorf("unexpected content: %s", result.Turns[0].Content)
	}
}

func TestTranscriptParser_SkipsThinkingBlocks(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","text":"internal reasoning"},{"type":"text","text":"visible response"}]},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnCount != 1 {
		t.Fatalf("expected 1 turn, got %d", result.TurnCount)
	}
	if result.Turns[0].Content != "visible response" {
		t.Errorf("expected only visible text, got: %s", result.Turns[0].Content)
	}
}

func TestTranscriptParser_SkipsUnknownTypes(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"file-history-snapshot","data":{}}`,
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnCount != 1 {
		t.Errorf("expected 1 turn (skipping unknown type), got %d", result.TurnCount)
	}
}

func TestTranscriptParser_ExtractsSummary(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"summary","summary":"This session was about debugging auth flow"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "This session was about debugging auth flow" {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
}

func TestTranscriptParser_ExtractsMultipleTools(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read"},{"type":"text","text":"reading file"},{"type":"tool_use","name":"Edit"},{"type":"tool_use","name":"Read"}]},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolsUsed["Read"] != 2 {
		t.Errorf("expected Read count 2, got %d", result.ToolsUsed["Read"])
	}
	if result.ToolsUsed["Edit"] != 1 {
		t.Errorf("expected Edit count 1, got %d", result.ToolsUsed["Edit"])
	}
	if len(result.Turns[0].ToolsUsed) != 3 {
		t.Errorf("expected 3 tools on turn, got %d", len(result.Turns[0].ToolsUsed))
	}
}

func TestTranscriptParser_StringContent(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":"plain string content"},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Turns[0].Content != "plain string content" {
		t.Errorf("unexpected content: %s", result.Turns[0].Content)
	}
}

func TestTranscriptParser_ArrayContent(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"part one"},{"type":"text","text":"part two"}]},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Turns[0].Content != "part one\npart two" {
		t.Errorf("expected joined content, got: %s", result.Turns[0].Content)
	}
}

func TestTranscriptParser_FileNotFound(t *testing.T) {
	parser := NewTranscriptParser()
	_, err := parser.ParseTranscript("/nonexistent/path/transcript.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestTranscriptParser_TurnIndexing(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response"}]},"timestamp":"2025-01-15T10:01:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"second"},"timestamp":"2025-01-15T10:02:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, turn := range result.Turns {
		if turn.Index != i {
			t.Errorf("turn %d has index %d", i, turn.Index)
		}
	}
}

func TestTranscriptParser_DigestTruncation(t *testing.T) {
	dir := t.TempDir()
	longContent := strings.Repeat("x", 300)
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":"`+longContent+`"},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Turns[0].Digest) != 200 {
		t.Errorf("expected digest truncated to 200 chars, got %d", len(result.Turns[0].Digest))
	}
}

func TestTranscriptParser_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir,
		`{"type":"user","message":{"role":"user","content":""},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"real message"},"timestamp":"2025-01-15T10:01:00Z"}`,
	)

	parser := NewTranscriptParser()
	result, err := parser.ParseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty content user messages are skipped.
	if result.TurnCount != 1 {
		t.Errorf("expected 1 turn (skipping empty content), got %d", result.TurnCount)
	}
}

// --- StructuralSummarizer tests ---

func TestStructuralSummarizer_EmptySession(t *testing.T) {
	summarizer := &StructuralSummarizer{}
	got := summarizer.Summarize(nil)
	if got != "(empty session)" {
		t.Errorf("expected '(empty session)', got %q", got)
	}
}

func TestStructuralSummarizer_WithTools(t *testing.T) {
	summarizer := &StructuralSummarizer{}
	turns := []models.SessionTurn{
		{Role: "user", Content: "Help me fix the login bug"},
		{Role: "assistant", Content: "I will look at it", ToolsUsed: []string{"Read", "Read", "Edit"}},
		{Role: "user", Content: "That fixed it"},
	}

	got := summarizer.Summarize(turns)
	if !strings.Contains(got, "Help me fix the login bug") {
		t.Errorf("expected first user message in summary, got: %s", got)
	}
	if !strings.Contains(got, "3 turns") {
		t.Errorf("expected turn count in summary, got: %s", got)
	}
	if !strings.Contains(got, "Read(2)") {
		t.Errorf("expected tool count in summary, got: %s", got)
	}
	if !strings.Contains(got, "Edit(1)") {
		t.Errorf("expected tool count in summary, got: %s", got)
	}
}

func TestStructuralSummarizer_NoTools(t *testing.T) {
	summarizer := &StructuralSummarizer{}
	turns := []models.SessionTurn{
		{Role: "user", Content: "Just chatting"},
		{Role: "assistant", Content: "Okay"},
	}

	got := summarizer.Summarize(turns)
	if !strings.Contains(got, "2 turns") {
		t.Errorf("expected turn count in summary, got: %s", got)
	}
	if strings.Contains(got, "tools:") {
		t.Errorf("expected no tools section when no tools used, got: %s", got)
	}
}

func TestStructuralSummarizer_LongTopic(t *testing.T) {
	summarizer := &StructuralSummarizer{}
	longMsg := strings.Repeat("a", 200)
	turns := []models.SessionTurn{
		{Role: "user", Content: longMsg},
	}

	got := summarizer.Summarize(turns)
	// Topic should be truncated to 100 chars.
	if len(got) > 200 {
		// The topic portion should be at most 100 chars.
		if !strings.HasPrefix(got, strings.Repeat("a", 100)) {
			t.Errorf("expected truncated topic in summary")
		}
	}
}

func TestStructuralSummarizer_NoUserMessage(t *testing.T) {
	summarizer := &StructuralSummarizer{}
	turns := []models.SessionTurn{
		{Role: "assistant", Content: "Auto-generated response"},
	}

	got := summarizer.Summarize(turns)
	if !strings.Contains(got, "(no user message)") {
		t.Errorf("expected '(no user message)' when no user turns, got: %s", got)
	}
}
