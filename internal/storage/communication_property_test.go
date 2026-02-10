package storage

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

func genCommunication(t *rapid.T) models.Communication {
	sources := []string{"slack", "email", "teams", "meeting"}
	tags := []models.CommunicationTag{
		models.TagRequirement, models.TagDecision,
		models.TagBlocker, models.TagQuestion, models.TagActionItem,
	}

	nTags := rapid.IntRange(0, 3).Draw(t, "nTags")
	commTags := make([]models.CommunicationTag, nTags)
	for i := range commTags {
		commTags[i] = tags[rapid.IntRange(0, len(tags)-1).Draw(t, "tagIdx")]
	}

	return models.Communication{
		Date:    time.Date(2026, time.Month(rapid.IntRange(1, 12).Draw(t, "month")), rapid.IntRange(1, 28).Draw(t, "day"), 0, 0, 0, 0, time.UTC),
		Source:  sources[rapid.IntRange(0, len(sources)-1).Draw(t, "sourceIdx")],
		Contact: genAlphaString(t, "contact", 1, 20),
		Topic:   genAlphaString(t, "topic", 1, 30),
		Content: genAlphaString(t, "content", 1, 200),
		Tags:    commTags,
	}
}

// Feature: ai-dev-brain, Property 7: Communication Filename Format
func TestCommunicationFilenameFormat(t *testing.T) {
	// Pattern: YYYY-MM-DD-source-contact-topic.md
	filenamePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-.+\.md$`)

	rapid.Check(t, func(t *rapid.T) {
		comm := genCommunication(t)
		filename := CommunicationFilename(comm)

		if !filenamePattern.MatchString(filename) {
			t.Fatalf("filename %q does not match expected pattern YYYY-MM-DD-source-contact-topic.md", filename)
		}

		// Verify the date portion matches the communication date.
		expectedDate := comm.Date.Format("2006-01-02")
		if !strings.HasPrefix(filename, expectedDate) {
			t.Fatalf("filename %q does not start with date %s", filename, expectedDate)
		}

		// Verify the source is present.
		lowerSource := strings.ToLower(comm.Source)
		sanitizedSource := sanitizeForFilename(lowerSource)
		if !strings.Contains(filename, sanitizedSource) {
			t.Fatalf("filename %q does not contain source %q", filename, sanitizedSource)
		}

		// Verify the contact is present.
		sanitizedContact := sanitizeForFilename(comm.Contact)
		if !strings.Contains(filename, sanitizedContact) {
			t.Fatalf("filename %q does not contain contact %q", filename, sanitizedContact)
		}

		// Verify .md extension.
		if !strings.HasSuffix(filename, ".md") {
			t.Fatalf("filename %q does not end with .md", filename)
		}
	})
}

// Feature: ai-dev-brain, Property 8: Communication Search Round-Trip
func TestCommunicationSearchRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "comm-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		mgr := NewCommunicationManager(dir).(*fileCommunicationManager)
		taskID := genTaskID(t)

		comm := genCommunication(t)
		if err := mgr.AddCommunication(taskID, comm); err != nil {
			t.Fatal(err)
		}

		// Search by content should find it.
		if len(comm.Content) >= 3 {
			query := strings.ToLower(comm.Content[:3])
			results, err := mgr.SearchCommunications(taskID, query)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range results {
				if strings.Contains(strings.ToLower(r.Content), query) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("search for content %q did not find communication", query)
			}
		}

		// Search by source should find it.
		results, err := mgr.SearchCommunications(taskID, strings.ToLower(comm.Source))
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatalf("search by source %q returned no results", comm.Source)
		}

		// Search by contact should find it.
		results, err = mgr.SearchCommunications(taskID, strings.ToLower(comm.Contact))
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatalf("search by contact %q returned no results", comm.Contact)
		}

		// Search by date should find it.
		results, err = mgr.SearchCommunications(taskID, comm.Date.Format("2006-01-02"))
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatalf("search by date %s returned no results", comm.Date.Format("2006-01-02"))
		}
	})
}

// Feature: ai-dev-brain, Property: Communication Format Round-Trip
// Any communication written with formatCommunication must parse back to the same fields.
func TestCommunicationFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		comm := genCommunication(t)
		formatted := formatCommunication(comm)
		parsed := parseCommunicationMarkdown(formatted)

		if parsed.Date.Format("2006-01-02") != comm.Date.Format("2006-01-02") {
			t.Fatalf("date mismatch: got %q, want %q",
				parsed.Date.Format("2006-01-02"), comm.Date.Format("2006-01-02"))
		}
		if parsed.Source != comm.Source {
			t.Fatalf("source mismatch: got %q, want %q", parsed.Source, comm.Source)
		}
		if parsed.Contact != comm.Contact {
			t.Fatalf("contact mismatch: got %q, want %q", parsed.Contact, comm.Contact)
		}
		if parsed.Topic != comm.Topic {
			t.Fatalf("topic mismatch: got %q, want %q", parsed.Topic, comm.Topic)
		}
		if parsed.Content != comm.Content {
			t.Fatalf("content mismatch: got %q, want %q", parsed.Content, comm.Content)
		}
		if len(parsed.Tags) != len(comm.Tags) {
			t.Fatalf("tags length mismatch: got %d, want %d", len(parsed.Tags), len(comm.Tags))
		}
		for i, tag := range comm.Tags {
			if parsed.Tags[i] != tag {
				t.Fatalf("tag %d mismatch: got %q, want %q", i, parsed.Tags[i], tag)
			}
		}
	})
}

// Feature: ai-dev-brain, Property: GetAll returns all stored communications
func TestCommunicationGetAllCountProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "comm-getall-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		mgr := NewCommunicationManager(dir).(*fileCommunicationManager)
		taskID := genTaskID(t)
		count := rapid.IntRange(1, 10).Draw(t, "count")

		for i := 0; i < count; i++ {
			comm := models.Communication{
				Date:    time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC),
				Source:  fmt.Sprintf("source%d", i),
				Contact: fmt.Sprintf("contact%d", i),
				Topic:   fmt.Sprintf("topic%d", i),
				Content: genAlphaString(t, fmt.Sprintf("content%d", i), 5, 50),
				Tags:    []models.CommunicationTag{models.TagDecision},
			}
			if err := mgr.AddCommunication(taskID, comm); err != nil {
				t.Fatal(err)
			}
		}

		all, err := mgr.GetAllCommunications(taskID)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != count {
			t.Fatalf("expected %d communications, got %d", count, len(all))
		}
	})
}
