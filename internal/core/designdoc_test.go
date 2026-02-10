package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func newTestDesignDocGenerator(t *testing.T) (*taskDesignDocGenerator, string) {
	t.Helper()
	dir := t.TempDir()
	commMgr := storage.NewCommunicationManager(dir)
	gen := NewTaskDesignDocGenerator(dir, commMgr).(*taskDesignDocGenerator)
	return gen, dir
}

func TestInitializeDesignDoc(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00001"

	err := gen.InitializeDesignDoc(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, err := gen.GetDesignDoc(taskID)
	if err != nil {
		t.Fatalf("unexpected error reading design doc: %v", err)
	}
	if doc.TaskID != taskID {
		t.Fatalf("expected task ID %q, got %q", taskID, doc.TaskID)
	}
}

func TestInitializeDesignDoc_CreatesFile(t *testing.T) {
	gen, dir := newTestDesignDocGenerator(t)
	taskID := "TASK-00002"

	gen.InitializeDesignDoc(taskID)

	path := filepath.Join(dir, "tickets", taskID, "design.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected design.md to be created at %s", path)
	}
}

func TestInitializeDesignDoc_ContainsExpectedSections(t *testing.T) {
	gen, dir := newTestDesignDocGenerator(t)
	taskID := "TASK-00003"

	gen.InitializeDesignDoc(taskID)

	path := filepath.Join(dir, "tickets", taskID, "design.md")
	data, _ := os.ReadFile(path)
	content := string(data)

	expectedSections := []string{
		"## Overview",
		"## Architecture",
		"## Components",
		"## Technical Decisions",
		"## Related ADRs",
		"## Stakeholder Requirements",
	}
	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Fatalf("design doc missing section %q", section)
		}
	}
}

func TestUpdateDesignDoc_Overview(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00004"

	gen.InitializeDesignDoc(taskID)

	err := gen.UpdateDesignDoc(taskID, DesignUpdate{
		Section: "overview",
		Content: "Implement OAuth2 PKCE flow",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := gen.GetDesignDoc(taskID)
	if doc.Overview != "Implement OAuth2 PKCE flow" {
		t.Fatalf("expected overview to be updated, got %q", doc.Overview)
	}
}

func TestUpdateDesignDoc_Architecture(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00005"

	gen.InitializeDesignDoc(taskID)

	mermaid := "graph LR\n    Auth --> Token\n    Token --> API"
	err := gen.UpdateDesignDoc(taskID, DesignUpdate{
		Section: "architecture",
		Content: mermaid,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := gen.GetDesignDoc(taskID)
	if doc.Architecture != mermaid {
		t.Fatalf("expected architecture updated, got %q", doc.Architecture)
	}
}

func TestUpdateDesignDoc_Decisions(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00006"

	gen.InitializeDesignDoc(taskID)

	err := gen.UpdateDesignDoc(taskID, DesignUpdate{
		Section: "decisions",
		Content: "Use JWT tokens for auth",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := gen.GetDesignDoc(taskID)
	if len(doc.TechnicalDecisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(doc.TechnicalDecisions))
	}
	if doc.TechnicalDecisions[0].Decision != "Use JWT tokens for auth" {
		t.Fatalf("expected decision content, got %q", doc.TechnicalDecisions[0].Decision)
	}
}

func TestUpdateDesignDoc_Components(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00007"

	gen.InitializeDesignDoc(taskID)

	err := gen.UpdateDesignDoc(taskID, DesignUpdate{
		Section: "components",
		Content: "AuthService",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := gen.GetDesignDoc(taskID)
	if len(doc.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(doc.Components))
	}
	if doc.Components[0].Name != "AuthService" {
		t.Fatalf("expected component name AuthService, got %q", doc.Components[0].Name)
	}
}

func TestUpdateDesignDoc_InvalidSection(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00008"

	gen.InitializeDesignDoc(taskID)

	err := gen.UpdateDesignDoc(taskID, DesignUpdate{
		Section: "invalid",
		Content: "test",
	})
	if err == nil {
		t.Fatal("expected error for invalid section")
	}
}

func TestExtractFromCommunications(t *testing.T) {
	gen, dir := newTestDesignDocGenerator(t)
	taskID := "TASK-00009"
	commMgr := storage.NewCommunicationManager(dir)

	// Add a communication tagged as decision.
	commMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "John",
		Topic:   "Auth approach",
		Content: "Use OAuth2 with PKCE",
		Tags:    []models.CommunicationTag{models.TagDecision},
	})

	// Add a non-decision communication.
	commMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC),
		Source:  "Email",
		Contact: "Jane",
		Topic:   "Timeline",
		Content: "Need this by Friday",
		Tags:    []models.CommunicationTag{models.TagActionItem},
	})

	decisions, err := gen.ExtractFromCommunications(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Decision != "Use OAuth2 with PKCE" {
		t.Fatalf("expected decision content, got %q", decisions[0].Decision)
	}
	if !decisions[0].ADRCandidate {
		t.Fatal("expected decision to be ADR candidate")
	}
}

func TestExtractFromCommunications_Empty(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00010"

	decisions, err := gen.ExtractFromCommunications(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestGenerateArchitectureDiagram_Empty(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00011"

	gen.InitializeDesignDoc(taskID)

	diagram, err := gen.GenerateArchitectureDiagram(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(diagram, "No components defined") {
		t.Fatalf("expected empty diagram placeholder, got %q", diagram)
	}
}

func TestGenerateArchitectureDiagram_WithComponents(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)
	taskID := "TASK-00012"

	gen.InitializeDesignDoc(taskID)

	// Write a design doc with components.
	doc, _ := gen.GetDesignDoc(taskID)
	doc.Components = []ComponentDescription{
		{Name: "Auth", Purpose: "Authentication", Dependencies: []string{"Token"}},
		{Name: "Token", Purpose: "Token management"},
	}
	gen.writeDesignDoc(doc)

	diagram, err := gen.GenerateArchitectureDiagram(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(diagram, "graph TB") {
		t.Fatalf("expected mermaid graph, got %q", diagram)
	}
	if !strings.Contains(diagram, "Auth") {
		t.Fatalf("expected Auth node in diagram, got %q", diagram)
	}
	if !strings.Contains(diagram, "Token") {
		t.Fatalf("expected Token node in diagram, got %q", diagram)
	}
	if !strings.Contains(diagram, "-->") {
		t.Fatalf("expected dependency arrow in diagram, got %q", diagram)
	}
}

func TestPopulateFromContext(t *testing.T) {
	gen, dir := newTestDesignDocGenerator(t)
	taskID := "TASK-00013"
	commMgr := storage.NewCommunicationManager(dir)

	gen.InitializeDesignDoc(taskID)

	// Add a requirement communication.
	commMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "Alice",
		Topic:   "PKCE requirement",
		Content: "Must support PKCE flow",
		Tags:    []models.CommunicationTag{models.TagRequirement},
	})

	// Add a decision communication.
	commMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC),
		Source:  "Meeting",
		Contact: "Bob",
		Topic:   "Auth provider",
		Content: "Use Auth0",
		Tags:    []models.CommunicationTag{models.TagDecision},
	})

	err := gen.PopulateFromContext(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := gen.GetDesignDoc(taskID)

	if len(doc.StakeholderRequirements) == 0 {
		t.Fatal("expected stakeholder requirements to be populated")
	}
	if len(doc.TechnicalDecisions) == 0 {
		t.Fatal("expected technical decisions to be populated")
	}
}

func TestGetDesignDoc_NotFound(t *testing.T) {
	gen, _ := newTestDesignDocGenerator(t)

	_, err := gen.GetDesignDoc("TASK-99999")
	if err == nil {
		t.Fatal("expected error for nonexistent design doc")
	}
}

func TestParseDesignDoc_FullRoundTrip(t *testing.T) {
	doc := &TaskDesignDocument{
		TaskID:       "TASK-00020",
		Title:        "TASK-00020",
		Overview:     "Implement feature X",
		Architecture: "graph TB\n    A --> B",
		Components: []ComponentDescription{
			{
				Name:         "ServiceA",
				Purpose:      "Handles auth",
				Interfaces:   []string{"Login", "Logout"},
				Dependencies: []string{"Database"},
			},
		},
		TechnicalDecisions: []TechnicalDecision{
			{
				Decision:  "Use PostgreSQL",
				Rationale: "Better JSON support",
				Source:    "slack-john",
				Date:      time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			},
		},
		RelatedADRs:             []string{"[ADR-0001](../../docs/decisions/ADR-0001.md)"},
		StakeholderRequirements: []string{"Must support PKCE flow"},
		LastUpdated:             time.Date(2026, 2, 10, 14, 30, 0, 0, time.UTC),
	}

	content := formatDesignDoc(doc)
	parsed := parseDesignDoc("TASK-00020", content)

	if parsed.TaskID != doc.TaskID {
		t.Fatalf("task ID mismatch: got %q, want %q", parsed.TaskID, doc.TaskID)
	}
	if parsed.Overview != doc.Overview {
		t.Fatalf("overview mismatch: got %q, want %q", parsed.Overview, doc.Overview)
	}
	if parsed.Architecture != doc.Architecture {
		t.Fatalf("architecture mismatch: got %q, want %q", parsed.Architecture, doc.Architecture)
	}
	if len(parsed.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(parsed.Components))
	}
	if parsed.Components[0].Name != "ServiceA" {
		t.Fatalf("component name mismatch: got %q", parsed.Components[0].Name)
	}
	if parsed.Components[0].Purpose != "Handles auth" {
		t.Fatalf("component purpose mismatch: got %q", parsed.Components[0].Purpose)
	}
	if len(parsed.TechnicalDecisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(parsed.TechnicalDecisions))
	}
	if parsed.TechnicalDecisions[0].Decision != "Use PostgreSQL" {
		t.Fatalf("decision mismatch: got %q", parsed.TechnicalDecisions[0].Decision)
	}
	if len(parsed.RelatedADRs) != 1 {
		t.Fatalf("expected 1 ADR, got %d", len(parsed.RelatedADRs))
	}
	if len(parsed.StakeholderRequirements) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(parsed.StakeholderRequirements))
	}
}

func TestSanitizeForMermaid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"AuthService", "AuthService"},
		{"My Component", "My_Component"},
		{"auth-service", "auth_service"},
		{"123", "123"},
	}
	for _, tc := range tests {
		result := sanitizeForMermaid(tc.input)
		if result != tc.expected {
			t.Fatalf("sanitizeForMermaid(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestFindRelatedADRs(t *testing.T) {
	gen, dir := newTestDesignDocGenerator(t)
	taskID := "TASK-00021"

	// Create decisions directory with an ADR referencing our task.
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	os.MkdirAll(decisionsDir, 0o755)
	adrContent := "# ADR-0001: Use OAuth2\n\n**Source:** TASK-00021\n\n## Decision\nUse OAuth2."
	os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-use-oauth2.md"), []byte(adrContent), 0o644)

	// Create an unrelated ADR.
	unrelatedContent := "# ADR-0002: Use PostgreSQL\n\n**Source:** TASK-00099\n\n## Decision\nUse PostgreSQL."
	os.WriteFile(filepath.Join(decisionsDir, "ADR-0002-use-postgresql.md"), []byte(unrelatedContent), 0o644)

	adrs, err := gen.findRelatedADRs(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adrs) != 1 {
		t.Fatalf("expected 1 related ADR, got %d", len(adrs))
	}
	if !strings.Contains(adrs[0], "ADR-0001") {
		t.Fatalf("expected related ADR to reference ADR-0001, got %q", adrs[0])
	}
}
