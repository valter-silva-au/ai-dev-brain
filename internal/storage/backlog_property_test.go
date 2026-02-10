package storage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

func genTaskID(t *rapid.T) string {
	n := rapid.IntRange(0, 99999).Draw(t, "taskNum")
	return fmt.Sprintf("TASK-%05d", n)
}

func genTaskStatus(t *rapid.T) models.TaskStatus {
	statuses := []models.TaskStatus{
		models.StatusBacklog, models.StatusInProgress, models.StatusBlocked,
		models.StatusReview, models.StatusDone, models.StatusArchived,
	}
	return statuses[rapid.IntRange(0, len(statuses)-1).Draw(t, "statusIdx")]
}

func genPriority(t *rapid.T) models.Priority {
	priorities := []models.Priority{models.P0, models.P1, models.P2, models.P3}
	return priorities[rapid.IntRange(0, len(priorities)-1).Draw(t, "priorityIdx")]
}

func genAlphaString(t *rapid.T, label string, minLen, maxLen int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	n := rapid.IntRange(minLen, maxLen).Draw(t, label+"Len")
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rapid.IntRange(0, len(letters)-1).Draw(t, label+"Char")]
	}
	return string(b)
}

func genBacklogEntry(t *rapid.T) BacklogEntry {
	owner := "@" + genAlphaString(t, "owner", 1, 10)
	repo := "github.com/" + genAlphaString(t, "org", 2, 8) + "/" + genAlphaString(t, "repoName", 2, 8)
	branch := genAlphaString(t, "branch", 1, 20)
	title := genAlphaString(t, "title", 1, 40)

	nTags := rapid.IntRange(0, 3).Draw(t, "nTags")
	tags := make([]string, nTags)
	for i := range tags {
		tags[i] = genAlphaString(t, fmt.Sprintf("tag%d", i), 1, 10)
	}

	nBlocked := rapid.IntRange(0, 2).Draw(t, "nBlocked")
	blockedBy := make([]string, nBlocked)
	for i := range blockedBy {
		blockedBy[i] = genTaskID(t)
	}

	nRelated := rapid.IntRange(0, 2).Draw(t, "nRelated")
	related := make([]string, nRelated)
	for i := range related {
		related[i] = genTaskID(t)
	}

	return BacklogEntry{
		ID:        genTaskID(t),
		Title:     title,
		Status:    genTaskStatus(t),
		Priority:  genPriority(t),
		Owner:     owner,
		Repo:      repo,
		Branch:    branch,
		Created:   time.Now().Format(time.RFC3339),
		Tags:      tags,
		BlockedBy: blockedBy,
		Related:   related,
	}
}

// Feature: ai-dev-brain, Property 5: Backlog Serialization Round-Trip
func TestBacklogSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		entries := rapid.SliceOfN(rapid.Custom(genBacklogEntry), 1, 20).Draw(t, "entries")

		// Deduplicate by ID (rapid may generate duplicates).
		seen := make(map[string]bool)
		var unique []BacklogEntry
		for _, e := range entries {
			if !seen[e.ID] {
				seen[e.ID] = true
				unique = append(unique, e)
			}
		}

		dir, err := os.MkdirTemp("", "backlog-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		mgr := NewBacklogManager(dir).(*fileBacklogManager)
		for _, e := range unique {
			if err := mgr.AddTask(e); err != nil {
				t.Fatal(err)
			}
		}

		if err := mgr.Save(); err != nil {
			t.Fatal(err)
		}

		mgr2 := NewBacklogManager(dir).(*fileBacklogManager)
		if err := mgr2.Load(); err != nil {
			t.Fatal(err)
		}

		loaded, err := mgr2.GetAllTasks()
		if err != nil {
			t.Fatal(err)
		}

		if len(loaded) != len(unique) {
			t.Fatalf("expected %d entries, got %d", len(unique), len(loaded))
		}

		for _, orig := range unique {
			found, err := mgr2.GetTask(orig.ID)
			if err != nil {
				t.Fatalf("task %s not found after round-trip", orig.ID)
			}
			if found.Title != orig.Title {
				t.Fatalf("task %s title mismatch: %q vs %q", orig.ID, found.Title, orig.Title)
			}
			if found.Status != orig.Status {
				t.Fatalf("task %s status mismatch: %q vs %q", orig.ID, found.Status, orig.Status)
			}
			if found.Priority != orig.Priority {
				t.Fatalf("task %s priority mismatch: %q vs %q", orig.ID, found.Priority, orig.Priority)
			}
			if found.Owner != orig.Owner {
				t.Fatalf("task %s owner mismatch: %q vs %q", orig.ID, found.Owner, orig.Owner)
			}
			if found.Repo != orig.Repo {
				t.Fatalf("task %s repo mismatch: %q vs %q", orig.ID, found.Repo, orig.Repo)
			}
			if found.Branch != orig.Branch {
				t.Fatalf("task %s branch mismatch: %q vs %q", orig.ID, found.Branch, orig.Branch)
			}
			if found.Created != orig.Created {
				t.Fatalf("task %s created mismatch: %q vs %q", orig.ID, found.Created, orig.Created)
			}
			if len(found.Tags) != len(orig.Tags) {
				t.Fatalf("task %s tags length mismatch: %d vs %d", orig.ID, len(found.Tags), len(orig.Tags))
			}
			if len(found.BlockedBy) != len(orig.BlockedBy) {
				t.Fatalf("task %s blocked_by length mismatch: %d vs %d", orig.ID, len(found.BlockedBy), len(orig.BlockedBy))
			}
			if len(found.Related) != len(orig.Related) {
				t.Fatalf("task %s related length mismatch: %d vs %d", orig.ID, len(found.Related), len(orig.Related))
			}
		}
	})
}

// Feature: ai-dev-brain, Property 6: Backlog Filter Correctness
func TestBacklogFilterCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		entries := rapid.SliceOfN(rapid.Custom(genBacklogEntry), 1, 20).Draw(t, "entries")

		// Deduplicate.
		seen := make(map[string]bool)
		var unique []BacklogEntry
		for _, e := range entries {
			if !seen[e.ID] {
				seen[e.ID] = true
				unique = append(unique, e)
			}
		}

		dir, err := os.MkdirTemp("", "backlog-filter-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		mgr := NewBacklogManager(dir).(*fileBacklogManager)
		for _, e := range unique {
			if err := mgr.AddTask(e); err != nil {
				t.Fatal(err)
			}
		}

		// Generate a filter.
		nFilterStatus := rapid.IntRange(0, 3).Draw(t, "nFilterStatus")
		filterStatuses := make([]models.TaskStatus, nFilterStatus)
		for i := range filterStatuses {
			filterStatuses[i] = genTaskStatus(t)
		}

		nFilterPriority := rapid.IntRange(0, 2).Draw(t, "nFilterPriority")
		filterPriorities := make([]models.Priority, nFilterPriority)
		for i := range filterPriorities {
			filterPriorities[i] = genPriority(t)
		}

		owners := append([]string{""}, ownersFromEntries(unique)...)
		repos := append([]string{""}, reposFromEntries(unique)...)

		nFilterTags := rapid.IntRange(0, 2).Draw(t, "nFilterTags")
		filterTags := make([]string, nFilterTags)
		for i := range filterTags {
			filterTags[i] = genAlphaString(t, fmt.Sprintf("filterTag%d", i), 1, 10)
		}

		filter := BacklogFilter{
			Status:   filterStatuses,
			Priority: filterPriorities,
			Owner:    owners[rapid.IntRange(0, len(owners)-1).Draw(t, "ownerIdx")],
			Repo:     repos[rapid.IntRange(0, len(repos)-1).Draw(t, "repoIdx")],
			Tags:     filterTags,
		}

		result, err := mgr.FilterTasks(filter)
		if err != nil {
			t.Fatal(err)
		}

		// Verify: every returned entry matches ALL filter criteria.
		for _, r := range result {
			if len(filter.Status) > 0 && !containsStatus(filter.Status, r.Status) {
				t.Fatalf("task %s status %q does not match filter %v", r.ID, r.Status, filter.Status)
			}
			if len(filter.Priority) > 0 && !containsPriority(filter.Priority, r.Priority) {
				t.Fatalf("task %s priority %q does not match filter %v", r.ID, r.Priority, filter.Priority)
			}
			if filter.Owner != "" && r.Owner != filter.Owner {
				t.Fatalf("task %s owner %q does not match filter %q", r.ID, r.Owner, filter.Owner)
			}
			if filter.Repo != "" && r.Repo != filter.Repo {
				t.Fatalf("task %s repo %q does not match filter %q", r.ID, r.Repo, filter.Repo)
			}
			if len(filter.Tags) > 0 && !hasAllTags(r.Tags, filter.Tags) {
				t.Fatalf("task %s tags %v do not match filter tags %v", r.ID, r.Tags, filter.Tags)
			}
		}

		// Verify: no matching entry was omitted.
		resultIDs := make(map[string]bool)
		for _, r := range result {
			resultIDs[r.ID] = true
		}
		for _, e := range unique {
			if matchesFilter(e, filter) && !resultIDs[e.ID] {
				t.Fatalf("task %s matches filter but was not returned", e.ID)
			}
		}
	})
}

func ownersFromEntries(entries []BacklogEntry) []string {
	set := make(map[string]bool)
	for _, e := range entries {
		set[e.Owner] = true
	}
	var result []string
	for k := range set {
		result = append(result, k)
	}
	return result
}

func reposFromEntries(entries []BacklogEntry) []string {
	set := make(map[string]bool)
	for _, e := range entries {
		set[e.Repo] = true
	}
	var result []string
	for k := range set {
		result = append(result, k)
	}
	return result
}
