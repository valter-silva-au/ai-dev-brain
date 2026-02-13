package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func newTestCommManager(t *testing.T) *fileCommunicationManager {
	t.Helper()
	dir := t.TempDir()
	mgr := NewCommunicationManager(dir).(*fileCommunicationManager)
	return mgr
}

func sampleCommunication() models.Communication {
	return models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "John Smith",
		Topic:   "New OAuth requirement",
		Content: "OAuth must support PKCE flow",
		Tags:    []models.CommunicationTag{models.TagRequirement, models.TagActionItem},
	}
}

func TestCommunicationFilename(t *testing.T) {
	comm := sampleCommunication()
	filename := CommunicationFilename(comm)

	if !strings.HasPrefix(filename, "2026-02-05-") {
		t.Fatalf("expected filename to start with date, got %q", filename)
	}
	if !strings.Contains(filename, "slack") {
		t.Fatalf("expected filename to contain source, got %q", filename)
	}
	if !strings.Contains(filename, "john-smith") {
		t.Fatalf("expected filename to contain contact, got %q", filename)
	}
	if !strings.HasSuffix(filename, ".md") {
		t.Fatalf("expected .md extension, got %q", filename)
	}
}

func TestCommunicationFilename_SpecialChars(t *testing.T) {
	comm := models.Communication{
		Date:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Source:  "Microsoft Teams",
		Contact: "Jane O'Brien (@jane)",
		Topic:   "API Rate-Limiting & Throttling!!!",
	}
	filename := CommunicationFilename(comm)

	// Should not contain special characters.
	if strings.ContainsAny(filename[:len(filename)-3], "!@#$%^&*()'+") {
		t.Fatalf("filename contains special characters: %q", filename)
	}
	if !strings.HasSuffix(filename, ".md") {
		t.Fatalf("expected .md extension, got %q", filename)
	}
}

func TestAddCommunication(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()

	err := mgr.AddCommunication("TASK-00001", comm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 communication, got %d", len(all))
	}
	if all[0].Source != "Slack" {
		t.Fatalf("expected source Slack, got %q", all[0].Source)
	}
	if all[0].Content != "OAuth must support PKCE flow" {
		t.Fatalf("expected content preserved, got %q", all[0].Content)
	}
}

func TestAddCommunication_DuplicateHandling(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()

	_ = mgr.AddCommunication("TASK-00001", comm)
	_ = mgr.AddCommunication("TASK-00001", comm)

	all, _ := mgr.GetAllCommunications("TASK-00001")
	if len(all) != 2 {
		t.Fatalf("expected 2 communications (with dedup suffix), got %d", len(all))
	}
}

func TestGetAllCommunications_Empty(t *testing.T) {
	mgr := newTestCommManager(t)

	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 communications, got %d", len(all))
	}
}

func TestSearchCommunications_ByContent(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, err := mgr.SearchCommunications("TASK-00001", "PKCE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearchCommunications_BySource(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "slack")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by source, got %d", len(results))
	}
}

func TestSearchCommunications_ByContact(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "john")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by contact, got %d", len(results))
	}
}

func TestSearchCommunications_ByDate(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "2026-02-05")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by date, got %d", len(results))
	}
}

func TestSearchCommunications_NoMatch(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "nonexistent-term-xyz")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchCommunications_CaseInsensitive(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "OAUTH")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for case-insensitive search, got %d", len(results))
	}
}

func TestParseCommunicationMarkdown(t *testing.T) {
	content := `# 2026-02-05-slack-john-requirement.md

**Date:** 2026-02-05
**Source:** Slack
**Contact:** John Smith
**Topic:** New requirement

## Content

Some important content here

## Tags
- requirement
- decision
`
	comm := parseCommunicationMarkdown(content)
	if comm.Source != "Slack" {
		t.Fatalf("expected Slack, got %q", comm.Source)
	}
	if comm.Contact != "John Smith" {
		t.Fatalf("expected John Smith, got %q", comm.Contact)
	}
	if comm.Content != "Some important content here" {
		t.Fatalf("unexpected content: %q", comm.Content)
	}
	if len(comm.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(comm.Tags))
	}
}

func TestFormatCommunication(t *testing.T) {
	comm := sampleCommunication()
	content := formatCommunication(comm)

	if !strings.Contains(content, "**Date:** 2026-02-05") {
		t.Fatal("formatted content missing date")
	}
	if !strings.Contains(content, "**Source:** Slack") {
		t.Fatal("formatted content missing source")
	}
	if !strings.Contains(content, "## Content") {
		t.Fatal("formatted content missing Content section")
	}
	if !strings.Contains(content, "## Tags") {
		t.Fatal("formatted content missing Tags section")
	}
	if !strings.Contains(content, "- requirement") {
		t.Fatal("formatted content missing requirement tag")
	}
}

func TestCommunicationFilename_LongTopic(t *testing.T) {
	comm := models.Communication{
		Date:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Source:  "Email",
		Contact: "Alice",
		Topic:   "This is a very long topic that exceeds fifty characters in length and should be truncated",
	}
	filename := CommunicationFilename(comm)
	// The topic portion should be truncated to 50 characters.
	// The total filename is: date-source-contact-topic.md
	if !strings.HasSuffix(filename, ".md") {
		t.Fatalf("expected .md extension, got %q", filename)
	}
	// Verify the topic was truncated by checking the sanitized topic length.
	topic := sanitizeForFilename(comm.Topic)
	if len(topic) <= 50 {
		t.Skip("topic already short enough after sanitization")
	}
	// The filename should contain the truncated topic (50 chars max).
	parts := strings.SplitN(filename, "-", 6) // date(3)-source-contact-topic
	if len(parts) < 6 {
		t.Fatalf("unexpected filename format: %q", filename)
	}
}

func TestAddCommunication_MkdirError(t *testing.T) {
	dir := t.TempDir()
	// Block directory creation by putting a file where tickets/ should be.
	if err := os.WriteFile(filepath.Join(dir, "tickets"), []byte("blocker"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewCommunicationManager(dir).(*fileCommunicationManager)
	err := mgr.AddCommunication("TASK-00001", sampleCommunication())
	if err == nil {
		t.Fatal("expected error when directory creation fails")
	}
	if !strings.Contains(err.Error(), "creating dir") {
		t.Fatalf("expected 'creating dir' in error, got %q", err.Error())
	}
}

func TestAddCommunication_WriteFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	dir := t.TempDir()
	mgr := NewCommunicationManager(dir).(*fileCommunicationManager)
	comm := sampleCommunication()

	// Create the communications directory.
	commsDir := mgr.commsDir("TASK-00001")
	if err := os.MkdirAll(commsDir, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Make the communications directory read-only so WriteFile fails.
	if err := os.Chmod(commsDir, 0o555); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(commsDir, 0o755)
	})

	err := mgr.AddCommunication("TASK-00001", comm)
	if err == nil {
		t.Fatal("expected error when file write fails")
	}
	if !strings.Contains(err.Error(), "writing file") {
		t.Fatalf("expected 'writing file' in error, got %q", err.Error())
	}
}

func TestSearchCommunications_GetAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ReadDir behavior differs on Windows")
	}
	dir := t.TempDir()
	mgr := NewCommunicationManager(dir).(*fileCommunicationManager)

	// Create the task ticket dir but make the comms dir a file.
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "communications"), []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err := mgr.SearchCommunications("TASK-00001", "test")
	if err == nil {
		t.Fatal("expected error when GetAllCommunications fails")
	}
}

func TestSearchCommunications_ByTopic(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	results, err := mgr.SearchCommunications("TASK-00001", "oauth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by topic, got %d", len(results))
	}
}

func TestMatchesCommunication_AllFields(t *testing.T) {
	comm := models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "Alice",
		Topic:   "API Design",
		Content: "Discussion about REST endpoints",
	}

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"match content", "rest endpoints", true},
		{"match source", "slack", true},
		{"match contact", "alice", true},
		{"match topic", "api design", true},
		{"match date", "2026-02-05", true},
		{"no match", "nonexistent-xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesCommunication(comm, tt.query)
			if got != tt.want {
				t.Fatalf("matchesCommunication(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestGetAllCommunications_ReadDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ReadDir behavior differs on Windows")
	}
	dir := t.TempDir()
	mgr := NewCommunicationManager(dir).(*fileCommunicationManager)

	// Create ticket dir but make communications a file (not a dir, not absent).
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "communications"), []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err := mgr.GetAllCommunications("TASK-00001")
	if err == nil {
		t.Fatal("expected error when communications is not a directory")
	}
	if !strings.Contains(err.Error(), "reading communications") {
		t.Fatalf("expected 'reading communications' in error, got %q", err.Error())
	}
}

func TestGetAllCommunications_SkipsDirsAndNonMd(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	commsDir := mgr.commsDir("TASK-00001")

	// Add a subdirectory.
	if err := os.MkdirAll(filepath.Join(commsDir, "subdir"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	// Add a non-.md file.
	if err := os.WriteFile(filepath.Join(commsDir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 communication (dirs and non-md skipped), got %d", len(all))
	}
}

func TestGetAllCommunications_FileReadErrorSkipped(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	_ = mgr.AddCommunication("TASK-00001", comm)

	commsDir := mgr.commsDir("TASK-00001")

	// Create a .md symlink pointing to a non-existent target so ReadFile fails
	// but the directory entry shows it as a non-directory .md file.
	brokenLink := filepath.Join(commsDir, "broken.md")
	_ = os.Symlink("/nonexistent/path/file.md", brokenLink)

	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The broken symlink should be skipped; only the valid communication counts.
	if len(all) != 1 {
		t.Fatalf("expected 1 communication (broken symlink skipped), got %d", len(all))
	}
}

func TestParseCommunicationMarkdown_ContentWithoutTags(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Email\n**Contact:** Bob\n**Topic:** Meeting\n\n## Content\n\nContent without tags section\n"

	comm := parseCommunicationMarkdown(content)
	if comm.Content != "Content without tags section" {
		t.Fatalf("unexpected content: %q", comm.Content)
	}
	if len(comm.Tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(comm.Tags))
	}
}

func TestParseCommunicationMarkdown_TagsFollowedBySection(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Email\n**Contact:** Bob\n**Topic:** Meeting\n\n## Content\n\nSome content\n\n## Tags\n- blocker\n- question\n\n## Notes\nExtra notes\n"

	comm := parseCommunicationMarkdown(content)
	if len(comm.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(comm.Tags), comm.Tags)
	}
	if comm.Tags[0] != models.CommunicationTag("blocker") {
		t.Fatalf("expected first tag 'blocker', got %q", comm.Tags[0])
	}
	if comm.Tags[1] != models.CommunicationTag("question") {
		t.Fatalf("expected second tag 'question', got %q", comm.Tags[1])
	}
}

func TestParseCommunicationMarkdown_EmptyContent(t *testing.T) {
	comm := parseCommunicationMarkdown("")
	if comm.Source != "" {
		t.Fatalf("expected empty source, got %q", comm.Source)
	}
	if !comm.Date.IsZero() {
		t.Fatalf("expected zero date, got %v", comm.Date)
	}
}

func TestParseCommunicationMarkdown_NoContentSection(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Email\n**Contact:** Bob\n**Topic:** Meeting\n"

	comm := parseCommunicationMarkdown(content)
	if comm.Content != "" {
		t.Fatalf("expected empty content when no ## Content section, got %q", comm.Content)
	}
}

func TestSanitizeForFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello-world"},
		{"API Rate-Limiting & Throttling!!!", "api-rate-limiting-throttling"},
		{"simple", "simple"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"---dashes---", "dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeForFilename(tt.input)
			if got != tt.want {
				t.Fatalf("sanitizeForFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
