package core

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

func genKnowledgeTaskID(t *rapid.T) string {
	n := rapid.IntRange(0, 99999).Draw(t, "taskNum")
	return fmt.Sprintf("TASK-%05d", n)
}

func genAlpha(t *rapid.T, label string, minLen, maxLen int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	n := rapid.IntRange(minLen, maxLen).Draw(t, label+"Len")
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rapid.IntRange(0, len(letters)-1).Draw(t, label+"Char")]
	}
	return string(b)
}

// Feature: ai-dev-brain, Property 15: Knowledge Provenance Tracking
func TestKnowledgeProvenanceTracking(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := genKnowledgeTaskID(t)

		dir, err := os.MkdirTemp("", "knowledge-prov-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		ctxMgr := storage.NewContextManager(dir)
		commMgr := storage.NewCommunicationManager(dir)
		ke := NewKnowledgeExtractor(dir, ctxMgr, commMgr)

		// Generate wiki update content.
		topic := genAlpha(t, "topic", 3, 20)
		content := genAlpha(t, "content", 5, 50)

		knowledge := &models.ExtractedKnowledge{
			TaskID: taskID,
			WikiUpdates: []models.WikiUpdate{
				{
					Topic:   topic,
					Content: content,
					TaskID:  taskID,
				},
			},
		}

		if err := ke.UpdateWiki(knowledge); err != nil {
			t.Fatal(err)
		}

		// Verify provenance: every wiki file must contain "Learned from TASK-XXXXX".
		wikiDir := fmt.Sprintf("%s/docs/wiki", dir)
		entries, err := os.ReadDir(wikiDir)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(fmt.Sprintf("%s/%s", wikiDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			fileContent := string(data)
			if !strings.Contains(fileContent, fmt.Sprintf("Learned from %s", taskID)) {
				t.Fatalf("wiki file %s does not contain provenance 'Learned from %s'", entry.Name(), taskID)
			}
		}
	})
}

// Feature: ai-dev-brain, Property 16: ADR Creation Format
func TestADRCreationFormat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := genKnowledgeTaskID(t)

		dir, err := os.MkdirTemp("", "knowledge-adr-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		ctxMgr := storage.NewContextManager(dir)
		commMgr := storage.NewCommunicationManager(dir)
		ke := NewKnowledgeExtractor(dir, ctxMgr, commMgr)

		nConsequences := rapid.IntRange(0, 3).Draw(t, "nConsequences")
		consequences := make([]string, nConsequences)
		for i := range consequences {
			consequences[i] = genAlpha(t, fmt.Sprintf("consequence%d", i), 3, 30)
		}

		nAlternatives := rapid.IntRange(0, 3).Draw(t, "nAlternatives")
		alternatives := make([]string, nAlternatives)
		for i := range alternatives {
			alternatives[i] = genAlpha(t, fmt.Sprintf("alternative%d", i), 3, 30)
		}

		decision := models.Decision{
			Title:        genAlpha(t, "title", 3, 30),
			Context:      genAlpha(t, "context", 5, 50),
			Decision:     genAlpha(t, "decision", 5, 50),
			Consequences: consequences,
			Alternatives: alternatives,
		}

		path, err := ke.CreateADR(decision, taskID)
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)

		// Verify all required sections are present.
		requiredSections := []string{
			"**Status:**",
			"**Date:**",
			"**Source:**",
			"## Context",
			"## Decision",
			"## Consequences",
			"## Alternatives Considered",
		}

		for _, section := range requiredSections {
			if !strings.Contains(content, section) {
				t.Fatalf("ADR at %s missing required section: %s", path, section)
			}
		}

		// Verify provenance.
		if !strings.Contains(content, taskID) {
			t.Fatalf("ADR does not reference source task %s", taskID)
		}

		// Verify consequences are present.
		for _, c := range consequences {
			if !strings.Contains(content, c) {
				t.Fatalf("ADR missing consequence: %q", c)
			}
		}

		// Verify alternatives are present.
		for _, a := range alternatives {
			if !strings.Contains(content, a) {
				t.Fatalf("ADR missing alternative: %q", a)
			}
		}
	})
}
