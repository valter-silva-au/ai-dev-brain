package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// validConflictTypes is the set of allowed ConflictType values.
var validConflictTypes = map[ConflictType]bool{
	ConflictADRViolation:           true,
	ConflictPreviousDecision:       true,
	ConflictStakeholderRequirement: true,
}

// validSeverities is the set of allowed Severity values.
var validSeverities = map[Severity]bool{
	SeverityHigh:   true,
	SeverityMedium: true,
	SeverityLow:    true,
}

// Feature: ai-dev-brain, Property 17: Conflict Reporting Format
// For any detected conflict, the surfaced conflict information SHALL include:
// conflict type, source document/decision, description of the conflict,
// and severity level.
func TestProperty_ConflictReportingFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random ADR content with overlapping keywords.
		adrTitle := rapid.StringMatching(`[A-Za-z]{4,10}`).Draw(rt, "adrTitle")
		keyword1 := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword1")
		keyword2 := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword2")
		taskNum := rapid.IntRange(1, 999).Draw(rt, "taskNum")

		dir, err := os.MkdirTemp("", "conflict-prop17-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		// Create an ADR that uses both keywords.
		decisionsDir := filepath.Join(dir, "docs", "decisions")
		if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
			t.Fatalf("failed to create decisions dir: %v", err)
		}

		adrContent := fmt.Sprintf(`# ADR-0001: %s

**Status:** Accepted
**Date:** 2026-01-01

## Decision
We will use %s and %s for the implementation.
`, adrTitle, keyword1, keyword2)

		if err := os.WriteFile(filepath.Join(decisionsDir, "ADR-0001.md"), []byte(adrContent), 0o644); err != nil {
			t.Fatalf("failed to write ADR: %v", err)
		}

		// Create a previous task's design.md with decisions containing the keywords.
		otherTaskID := fmt.Sprintf("TASK-%05d", taskNum)
		ticketDir := filepath.Join(dir, "tickets", otherTaskID)
		if err := os.MkdirAll(ticketDir, 0o755); err != nil {
			t.Fatalf("failed to create ticket dir: %v", err)
		}

		designContent := fmt.Sprintf(`# Technical Design

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
| Use %s with %s | Performance reasons | 2026-01-05 |
`, keyword1, keyword2)

		if err := os.WriteFile(filepath.Join(ticketDir, "design.md"), []byte(designContent), 0o644); err != nil {
			t.Fatalf("failed to write design.md: %v", err)
		}

		// Create a wiki requirement with the keywords.
		wikiDir := filepath.Join(dir, "docs", "wiki")
		if err := os.MkdirAll(wikiDir, 0o755); err != nil {
			t.Fatalf("failed to create wiki dir: %v", err)
		}

		wikiContent := fmt.Sprintf(`# Requirements
All components must support %s and %s integration.
`, keyword1, keyword2)

		if err := os.WriteFile(filepath.Join(wikiDir, "reqs.md"), []byte(wikiContent), 0o644); err != nil {
			t.Fatalf("failed to write wiki: %v", err)
		}

		cd := NewConflictDetector(dir)

		// Propose changes that reference both keywords -- should trigger conflicts.
		proposedChanges := fmt.Sprintf("Replace %s with alternative and remove %s support", keyword1, keyword2)
		conflicts, err := cd.CheckForConflicts(ConflictContext{
			TaskID:          "TASK-99999",
			ProposedChanges: proposedChanges,
		})
		if err != nil {
			t.Fatalf("CheckForConflicts failed: %v", err)
		}

		// Property 17: every returned conflict must have valid type, non-empty
		// source, non-empty description, and valid severity.
		for i, c := range conflicts {
			if !validConflictTypes[c.Type] {
				t.Fatalf("conflict %d: invalid type %q", i, c.Type)
			}
			if c.Source == "" {
				t.Fatalf("conflict %d: source must not be empty", i)
			}
			if c.Description == "" {
				t.Fatalf("conflict %d: description must not be empty", i)
			}
			if c.Recommendation == "" {
				t.Fatalf("conflict %d: recommendation must not be empty", i)
			}
			if !validSeverities[c.Severity] {
				t.Fatalf("conflict %d: invalid severity %q", i, c.Severity)
			}
		}
	})
}

// Feature: ai-dev-brain, Property: Conflict Detection Self-Exclusion
// For any task, CheckForConflicts SHALL NOT flag the task's own decisions.
func TestProperty_ConflictSelfExclusion(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskNum := rapid.IntRange(1, 99999).Draw(rt, "taskNum")
		taskID := fmt.Sprintf("TASK-%05d", taskNum)
		keyword := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword")

		dir, err := os.MkdirTemp("", "conflict-self-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		// Create the task's own design.md with the keyword in decisions.
		ticketDir := filepath.Join(dir, "tickets", taskID)
		if err := os.MkdirAll(ticketDir, 0o755); err != nil {
			t.Fatalf("failed to create ticket dir: %v", err)
		}

		designContent := fmt.Sprintf(`# Technical Design

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
| Implement %s component | Required feature | 2026-01-01 |
`, keyword)

		if err := os.WriteFile(filepath.Join(ticketDir, "design.md"), []byte(designContent), 0o644); err != nil {
			t.Fatalf("failed to write design.md: %v", err)
		}

		cd := NewConflictDetector(dir)

		conflicts, err := cd.CheckForConflicts(ConflictContext{
			TaskID:          taskID,
			ProposedChanges: fmt.Sprintf("Update the %s component implementation", keyword),
		})
		if err != nil {
			t.Fatalf("CheckForConflicts failed: %v", err)
		}

		for _, c := range conflicts {
			if c.Type == ConflictPreviousDecision && c.Source == taskID {
				t.Fatalf("must not flag conflicts from the task's own decisions (source=%s)", c.Source)
			}
		}
	})
}
