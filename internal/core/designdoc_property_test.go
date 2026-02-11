package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

func genDesignTaskID(t *rapid.T) string {
	n := rapid.IntRange(0, 99999).Draw(t, "taskNum")
	return fmt.Sprintf("TASK-%05d", n)
}

func genAlphaStr(t *rapid.T, label string, minLen, maxLen int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	n := rapid.IntRange(minLen, maxLen).Draw(t, label+"Len")
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rapid.IntRange(0, len(letters)-1).Draw(t, label+"Char")]
	}
	return string(b)
}

// Feature: ai-dev-brain, Property 22: Task Design Document Bootstrap
// For any newly bootstrapped task, a design.md file SHALL be created containing
// the task title, creation date, and placeholder sections for architecture and decisions.
func TestDesignDocBootstrapProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "designdoc-bootstrap-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		commMgr := storage.NewCommunicationManager(dir)
		gen := NewTaskDesignDocGenerator(dir, commMgr).(*taskDesignDocGenerator)

		taskID := genDesignTaskID(t)

		if err := gen.InitializeDesignDoc(taskID); err != nil {
			t.Fatal(err)
		}

		// Verify file exists.
		path := filepath.Join(dir, "tickets", taskID, "design.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("design.md not created for %s: %v", taskID, err)
		}

		content := string(data)

		// Must contain the task ID in the title.
		if !strings.Contains(content, taskID) {
			t.Fatalf("design.md does not contain task ID %s", taskID)
		}

		// Must contain creation date (today's date in YYYY-MM-DD format).
		today := time.Now().Format("2006-01-02")
		if !strings.Contains(content, today) {
			t.Fatalf("design.md does not contain creation date %s", today)
		}

		// Must contain architecture section.
		if !strings.Contains(content, "## Architecture") {
			t.Fatalf("design.md missing Architecture section")
		}

		// Must contain decisions section.
		if !strings.Contains(content, "## Technical Decisions") {
			t.Fatalf("design.md missing Technical Decisions section")
		}

		// Must contain overview section.
		if !strings.Contains(content, "## Overview") {
			t.Fatalf("design.md missing Overview section")
		}

		// Must contain components section.
		if !strings.Contains(content, "## Components") {
			t.Fatalf("design.md missing Components section")
		}
	})
}

// Feature: ai-dev-brain, Property 23: Task Design Document Context Population
// For any task with related communications tagged as requirements, the design.md
// SHALL be populated with relevant stakeholder requirements from those sources.
func TestDesignDocContextPopulationProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "designdoc-populate-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		commMgr := storage.NewCommunicationManager(dir)
		gen := NewTaskDesignDocGenerator(dir, commMgr).(*taskDesignDocGenerator)

		taskID := genDesignTaskID(t)

		if err := gen.InitializeDesignDoc(taskID); err != nil {
			t.Fatal(err)
		}

		// Generate random requirement communications.
		nReqs := rapid.IntRange(1, 5).Draw(t, "nReqs")
		for i := 0; i < nReqs; i++ {
			comm := models.Communication{
				Date:    time.Date(2026, 2, rapid.IntRange(1, 28).Draw(t, fmt.Sprintf("day%d", i)), 0, 0, 0, 0, time.UTC),
				Source:  genAlphaStr(t, fmt.Sprintf("source%d", i), 3, 10),
				Contact: genAlphaStr(t, fmt.Sprintf("contact%d", i), 3, 10),
				Topic:   genAlphaStr(t, fmt.Sprintf("topic%d", i), 3, 20),
				Content: genAlphaStr(t, fmt.Sprintf("reqcontent%d", i), 10, 50),
				Tags:    []models.CommunicationTag{models.TagRequirement},
			}
			if err := commMgr.AddCommunication(taskID, comm); err != nil {
				t.Fatal(err)
			}
		}

		if err := gen.PopulateFromContext(taskID); err != nil {
			t.Fatal(err)
		}

		doc, err := gen.GetDesignDoc(taskID)
		if err != nil {
			t.Fatal(err)
		}

		// Must have at least nReqs stakeholder requirements.
		if len(doc.StakeholderRequirements) < nReqs {
			t.Fatalf("expected at least %d stakeholder requirements, got %d",
				nReqs, len(doc.StakeholderRequirements))
		}
	})
}

// Feature: ai-dev-brain, Property 24: Technical Decision Extraction
// For any communication tagged with a decision, the decision SHALL be extracted
// and added to the task's design document with source attribution.
func TestTechnicalDecisionExtractionProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "designdoc-decisions-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		commMgr := storage.NewCommunicationManager(dir)
		gen := NewTaskDesignDocGenerator(dir, commMgr).(*taskDesignDocGenerator)

		taskID := genDesignTaskID(t)

		// Create decision-tagged communications.
		nDecisions := rapid.IntRange(1, 5).Draw(t, "nDecisions")
		expectedContents := make([]string, nDecisions)

		for i := 0; i < nDecisions; i++ {
			content := genAlphaStr(t, fmt.Sprintf("deccontent%d", i), 10, 50)
			expectedContents[i] = content
			comm := models.Communication{
				Date:    time.Date(2026, 2, rapid.IntRange(1, 28).Draw(t, fmt.Sprintf("decday%d", i)), 0, 0, 0, 0, time.UTC),
				Source:  genAlphaStr(t, fmt.Sprintf("decsource%d", i), 3, 10),
				Contact: genAlphaStr(t, fmt.Sprintf("deccontact%d", i), 3, 10),
				Topic:   genAlphaStr(t, fmt.Sprintf("dectopic%d", i), 3, 20),
				Content: content,
				Tags:    []models.CommunicationTag{models.TagDecision},
			}
			if err := commMgr.AddCommunication(taskID, comm); err != nil {
				t.Fatal(err)
			}
		}

		// Also add a non-decision communication that should NOT be extracted.
		_ = commMgr.AddCommunication(taskID, models.Communication{
			Date:    time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			Source:  "email",
			Contact: "boss",
			Topic:   "timeline",
			Content: "need this done soon",
			Tags:    []models.CommunicationTag{models.TagActionItem},
		})

		decisions, err := gen.ExtractFromCommunications(taskID)
		if err != nil {
			t.Fatal(err)
		}

		// Must extract exactly nDecisions (not the action item).
		if len(decisions) != nDecisions {
			t.Fatalf("expected %d decisions, got %d", nDecisions, len(decisions))
		}

		// Every extracted decision must have ADRCandidate=true.
		for i, d := range decisions {
			if !d.ADRCandidate {
				t.Fatalf("decision %d should be ADR candidate", i)
			}
		}

		// Every extracted decision must have a non-empty Source attribution.
		for i, d := range decisions {
			if d.Source == "" {
				t.Fatalf("decision %d has empty source attribution", i)
			}
		}

		// Every decision content must match one of the expected contents.
		for _, expected := range expectedContents {
			found := false
			for _, d := range decisions {
				if d.Decision == expected {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected decision content %q not found in extracted decisions", expected)
			}
		}
	})
}

// Feature: ai-dev-brain, Property: Design Document Format Round-Trip
// For any design document written via formatDesignDoc, parseDesignDoc must recover
// the same overview, architecture, components, decisions, ADRs, and requirements.
func TestDesignDocFormatRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := genDesignTaskID(t)

		nComps := rapid.IntRange(0, 3).Draw(t, "nComps")
		components := make([]ComponentDescription, nComps)
		for i := range components {
			components[i] = ComponentDescription{
				Name:    genAlphaStr(t, fmt.Sprintf("compname%d", i), 3, 15),
				Purpose: genAlphaStr(t, fmt.Sprintf("comppurpose%d", i), 5, 30),
			}
		}

		nDecisions := rapid.IntRange(0, 3).Draw(t, "nDecisions")
		decisions := make([]TechnicalDecision, nDecisions)
		for i := range decisions {
			decisions[i] = TechnicalDecision{
				Decision:  genAlphaStr(t, fmt.Sprintf("dec%d", i), 5, 30),
				Rationale: genAlphaStr(t, fmt.Sprintf("rat%d", i), 5, 30),
				Source:    genAlphaStr(t, fmt.Sprintf("src%d", i), 3, 15),
				Date:      time.Date(2026, 2, rapid.IntRange(1, 28).Draw(t, fmt.Sprintf("decdate%d", i)), 0, 0, 0, 0, time.UTC),
			}
		}

		nReqs := rapid.IntRange(0, 3).Draw(t, "nReqs")
		reqs := make([]string, nReqs)
		for i := range reqs {
			reqs[i] = genAlphaStr(t, fmt.Sprintf("req%d", i), 5, 30)
		}

		doc := &TaskDesignDocument{
			TaskID:                  taskID,
			Title:                   taskID,
			Overview:                genAlphaStr(t, "overview", 5, 50),
			Architecture:            "graph TB\n    A --> B",
			Components:              components,
			TechnicalDecisions:      decisions,
			StakeholderRequirements: reqs,
			LastUpdated:             time.Date(2026, 2, 10, 14, 30, 0, 0, time.UTC),
		}

		content := formatDesignDoc(doc)
		parsed := parseDesignDoc(taskID, content)

		if parsed.Overview != doc.Overview {
			t.Fatalf("overview mismatch: got %q, want %q", parsed.Overview, doc.Overview)
		}
		if parsed.Architecture != doc.Architecture {
			t.Fatalf("architecture mismatch: got %q, want %q", parsed.Architecture, doc.Architecture)
		}
		if len(parsed.Components) != len(doc.Components) {
			t.Fatalf("components count mismatch: got %d, want %d", len(parsed.Components), len(doc.Components))
		}
		for i, comp := range doc.Components {
			if parsed.Components[i].Name != comp.Name {
				t.Fatalf("component %d name mismatch: got %q, want %q", i, parsed.Components[i].Name, comp.Name)
			}
			if parsed.Components[i].Purpose != comp.Purpose {
				t.Fatalf("component %d purpose mismatch: got %q, want %q", i, parsed.Components[i].Purpose, comp.Purpose)
			}
		}
		if len(parsed.TechnicalDecisions) != len(doc.TechnicalDecisions) {
			t.Fatalf("decisions count mismatch: got %d, want %d", len(parsed.TechnicalDecisions), len(doc.TechnicalDecisions))
		}
		for i, dec := range doc.TechnicalDecisions {
			if parsed.TechnicalDecisions[i].Decision != dec.Decision {
				t.Fatalf("decision %d mismatch: got %q, want %q", i, parsed.TechnicalDecisions[i].Decision, dec.Decision)
			}
		}
		if len(parsed.StakeholderRequirements) != len(doc.StakeholderRequirements) {
			t.Fatalf("requirements count mismatch: got %d, want %d", len(parsed.StakeholderRequirements), len(doc.StakeholderRequirements))
		}
	})
}

// Feature: ai-dev-brain, Property 25: Design Document as Knowledge Source
// For any archived task with a design.md, the Knowledge_Extractor SHALL use
// design.md as a primary source for extracting technical learnings and
// identifying ADR candidates.
func TestDesignDocAsKnowledgeSourceProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "designdoc-knowledge-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		commMgr := storage.NewCommunicationManager(dir)
		ctxMgr := storage.NewContextManager(dir)
		gen := NewTaskDesignDocGenerator(dir, commMgr).(*taskDesignDocGenerator)
		ke := NewKnowledgeExtractor(dir, ctxMgr, commMgr)

		taskID := genDesignTaskID(t)

		// Initialize context so the knowledge extractor can load it.
		if _, err := ctxMgr.InitializeContext(taskID); err != nil {
			t.Fatal(err)
		}

		// Initialize and populate design doc.
		if err := gen.InitializeDesignDoc(taskID); err != nil {
			t.Fatal(err)
		}

		// Generate random technical decisions for the design doc.
		nDecisions := rapid.IntRange(1, 4).Draw(t, "nDecisions")
		doc, err := gen.GetDesignDoc(taskID)
		if err != nil {
			t.Fatal(err)
		}

		overview := genAlphaStr(t, "overview", 10, 40)
		doc.Overview = overview

		nComps := rapid.IntRange(1, 3).Draw(t, "nComps")
		doc.Components = make([]ComponentDescription, nComps)
		for i := range doc.Components {
			doc.Components[i] = ComponentDescription{
				Name:    genAlphaStr(t, fmt.Sprintf("compname%d", i), 3, 15),
				Purpose: genAlphaStr(t, fmt.Sprintf("comppurpose%d", i), 5, 30),
			}
		}

		doc.TechnicalDecisions = make([]TechnicalDecision, nDecisions)
		for i := range doc.TechnicalDecisions {
			doc.TechnicalDecisions[i] = TechnicalDecision{
				Decision:  genAlphaStr(t, fmt.Sprintf("ksdec%d", i), 5, 30),
				Rationale: genAlphaStr(t, fmt.Sprintf("ksrat%d", i), 5, 30),
				Source:    genAlphaStr(t, fmt.Sprintf("kssrc%d", i), 3, 15),
				Date:      time.Date(2026, 2, rapid.IntRange(1, 28).Draw(t, fmt.Sprintf("ksdate%d", i)), 0, 0, 0, 0, time.UTC),
			}
		}

		if err := gen.writeDesignDoc(doc); err != nil {
			t.Fatal(err)
		}

		// Extract knowledge -- should incorporate design.md.
		knowledge, err := ke.ExtractFromTask(taskID)
		if err != nil {
			t.Fatal(err)
		}

		// Verify decisions from design doc are included in extracted knowledge.
		for _, td := range doc.TechnicalDecisions {
			found := false
			for _, d := range knowledge.Decisions {
				if d.Decision == td.Decision {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("design doc decision %q not found in extracted knowledge decisions", td.Decision)
			}
		}

		// Verify overview is included as a learning.
		foundOverview := false
		for _, l := range knowledge.Learnings {
			if strings.Contains(l, overview) {
				foundOverview = true
				break
			}
		}
		if !foundOverview {
			t.Fatalf("design doc overview not found in extracted learnings")
		}

		// Verify component descriptions are included as learnings.
		for _, comp := range doc.Components {
			foundComp := false
			for _, l := range knowledge.Learnings {
				if strings.Contains(l, comp.Name) && strings.Contains(l, comp.Purpose) {
					foundComp = true
					break
				}
			}
			if !foundComp {
				t.Fatalf("design doc component %q not found in extracted learnings", comp.Name)
			}
		}
	})
}
