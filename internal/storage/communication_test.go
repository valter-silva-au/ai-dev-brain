package storage

import (
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
	mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "slack")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by source, got %d", len(results))
	}
}

func TestSearchCommunications_ByContact(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "john")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by contact, got %d", len(results))
	}
}

func TestSearchCommunications_ByDate(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "2026-02-05")
	if len(results) != 1 {
		t.Fatalf("expected 1 result searching by date, got %d", len(results))
	}
}

func TestSearchCommunications_NoMatch(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	mgr.AddCommunication("TASK-00001", comm)

	results, _ := mgr.SearchCommunications("TASK-00001", "nonexistent-term-xyz")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchCommunications_CaseInsensitive(t *testing.T) {
	mgr := newTestCommManager(t)
	comm := sampleCommunication()
	mgr.AddCommunication("TASK-00001", comm)

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
