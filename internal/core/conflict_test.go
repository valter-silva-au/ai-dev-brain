package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckForConflicts_NoDocsOrTickets(t *testing.T) {
	dir := t.TempDir()
	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00001",
		ProposedChanges: "switch authentication to session tokens",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestCheckForConflicts_ADRViolation(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	adrContent := `# ADR-0001: Use JWT tokens for authentication

**Status:** Accepted
**Date:** 2026-01-15
**Source:** TASK-00010

## Context
We need a stateless authentication mechanism.

## Decision
Use JWT tokens for all API authentication. All services must validate
JWT tokens using the shared secret key.

## Consequences
- Stateless authentication across all services
- Token expiry must be handled by clients
`
	if err := os.WriteFile(filepath.Join(decisionsDir, "ADR-0001.md"), []byte(adrContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Replace JWT tokens with session-based authentication for API services",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conflicts) == 0 {
		t.Fatal("expected at least one ADR conflict")
	}

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictADRViolation {
			found = true
			if c.Severity != SeverityHigh {
				t.Errorf("ADR violations should be high severity, got %s", c.Severity)
			}
			if c.Source != "ADR-0001.md" {
				t.Errorf("expected source ADR-0001.md, got %s", c.Source)
			}
		}
	}
	if !found {
		t.Error("expected an adr_violation conflict type")
	}
}

func TestCheckForConflicts_IgnatesNonAcceptedADR(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	adrContent := `# ADR-0002: Use GraphQL

**Status:** Superseded
**Date:** 2026-01-20

## Decision
Use GraphQL for the frontend API layer with Apollo server.
`
	if err := os.WriteFile(filepath.Join(decisionsDir, "ADR-0002.md"), []byte(adrContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Switch from GraphQL to REST API using Apollo migration",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range conflicts {
		if c.Type == ConflictADRViolation && c.Source == "ADR-0002.md" {
			t.Error("superseded ADRs should not produce conflicts")
		}
	}
}

func TestCheckForConflicts_PreviousDecision(t *testing.T) {
	dir := t.TempDir()
	ticketDir := filepath.Join(dir, "tickets", "TASK-00050")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatal(err)
	}

	designContent := `# Technical Design

## Overview
Payment processing service

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
| Use Stripe for payment processing | Best developer experience and documentation | 2026-01-10 |
| Store payment tokens encrypted | Security compliance requirement | 2026-01-12 |
`
	if err := os.WriteFile(filepath.Join(ticketDir, "design.md"), []byte(designContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Migrate payment processing from Stripe to a custom payment gateway with unencrypted tokens",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conflicts) == 0 {
		t.Fatal("expected at least one previous decision conflict")
	}

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictPreviousDecision && c.Source == "TASK-00050" {
			found = true
			if c.Severity != SeverityMedium {
				t.Errorf("previous decision conflicts should be medium severity, got %s", c.Severity)
			}
		}
	}
	if !found {
		t.Error("expected a previous_decision conflict from TASK-00050")
	}
}

func TestCheckForConflicts_SkipsOwnTask(t *testing.T) {
	dir := t.TempDir()
	ticketDir := filepath.Join(dir, "tickets", "TASK-00099")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatal(err)
	}

	designContent := `# Technical Design

## Decisions
| Decision | Rationale | Date |
|----------|-----------|------|
| Use Redis for caching layer | Performance requirements need sub-ms latency | 2026-02-01 |
`
	if err := os.WriteFile(filepath.Join(ticketDir, "design.md"), []byte(designContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Switch Redis caching layer to Memcached for performance",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range conflicts {
		if c.Type == ConflictPreviousDecision && c.Source == "TASK-00099" {
			t.Error("should not flag conflicts from the task's own decisions")
		}
	}
}

func TestCheckForConflicts_StakeholderRequirement(t *testing.T) {
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "docs", "wiki")
	if err := os.MkdirAll(wikiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	reqContent := `# Security Requirements

All user data must be encrypted at rest using AES-256.
Authentication tokens must expire within 24 hours.
Password storage must use bcrypt with minimum cost factor 12.
`
	if err := os.WriteFile(filepath.Join(wikiDir, "security-requirements.md"), []byte(reqContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cd := NewConflictDetector(dir)

	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Store user passwords using MD5 hash without encryption, authentication tokens never expire",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conflicts) == 0 {
		t.Fatal("expected at least one stakeholder requirement conflict")
	}

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictStakeholderRequirement {
			found = true
			if c.Severity != SeverityMedium {
				t.Errorf("stakeholder requirement conflicts should be medium severity, got %s", c.Severity)
			}
		}
	}
	if !found {
		t.Error("expected a stakeholder_requirement conflict type")
	}
}

func TestCheckForConflicts_NoMatchReturnsEmpty(t *testing.T) {
	dir := t.TempDir()

	// Set up an ADR, task decision, and wiki requirement.
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	wikiDir := filepath.Join(dir, "docs", "wiki")
	ticketDir := filepath.Join(dir, "tickets", "TASK-00050")
	for _, d := range []string{decisionsDir, wikiDir, ticketDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	os.WriteFile(filepath.Join(decisionsDir, "ADR-0001.md"), []byte(`# ADR-0001: Use PostgreSQL
**Status:** Accepted
## Decision
Use PostgreSQL as the primary database for all transactional data.
`), 0o644)

	os.WriteFile(filepath.Join(ticketDir, "design.md"), []byte(`# Design
## Decisions
| Decision | Rationale | Date |
| Use Docker for deployments | Container standardization | 2026-01 |
`), 0o644)

	os.WriteFile(filepath.Join(wikiDir, "coding-standards.md"), []byte(`# Coding Standards
All Go code must pass golangci-lint with default configuration.
`), 0o644)

	cd := NewConflictDetector(dir)

	// Proposed changes have no overlap with any existing content.
	conflicts, err := cd.CheckForConflicts(ConflictContext{
		TaskID:          "TASK-00099",
		ProposedChanges: "Add a new button to the homepage navbar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for unrelated changes, got %d", len(conflicts))
	}
}

func TestExtractSection(t *testing.T) {
	content := `# Title

## Context
Some context here.
More context.

## Decision
The actual decision content.
With details.

## Consequences
Some consequences.
`
	section := extractSection(content, "## Decision")
	if section == "" {
		t.Fatal("expected non-empty section")
	}
	if !contains(section, "actual decision content") {
		t.Errorf("section should contain decision content, got: %s", section)
	}
	if contains(section, "consequences") {
		t.Error("section should not include the next heading's content")
	}
}

func TestExtractKeywords(t *testing.T) {
	text := "use jwt tokens for all api authentication"
	keywords := extractKeywords(text)

	expectedPresent := []string{"tokens", "authentication"}
	for _, kw := range expectedPresent {
		found := false
		for _, k := range keywords {
			if k == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected keyword %q in results", kw)
		}
	}

	// Short words and stop words should be excluded.
	for _, kw := range keywords {
		if len(kw) < 4 {
			t.Errorf("keyword %q is too short (< 4 chars)", kw)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
