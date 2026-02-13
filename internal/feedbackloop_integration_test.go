package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/internal/integration"
	"github.com/drapaimern/ai-dev-brain/internal/storage"
)

// ---------------------------------------------------------------------------
// helpers for feedback loop tests
// ---------------------------------------------------------------------------

// writeInboxFile creates a markdown file with YAML frontmatter in the inbox.
func writeInboxFile(t *testing.T, inboxDir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(inboxDir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("writing inbox file %s: %v", filename, err)
	}
}

// setupFeedbackLoop builds a FeedbackLoopOrchestrator wired to a file channel
// adapter in a temp directory, with a real KnowledgeManager and storage.
// It returns the orchestrator, the inbox directory path, and the channel base directory.
func setupFeedbackLoop(t *testing.T) (core.FeedbackLoopOrchestrator, string, string) {
	t.Helper()

	baseDir := t.TempDir()

	// Set up knowledge store.
	ksm := storage.NewKnowledgeStoreManager(baseDir)
	_ = ksm.Load()
	ksAdapter := &knowledgeStoreAdapter{mgr: ksm}
	km := core.NewKnowledgeManager(ksAdapter)

	// Set up channel registry with a file channel adapter.
	channelDir := filepath.Join(baseDir, "channels")
	adapter, err := integration.NewFileChannelAdapter(integration.FileChannelConfig{
		Name:    "test",
		BaseDir: channelDir,
	})
	if err != nil {
		t.Fatalf("creating file channel adapter: %v", err)
	}

	reg := core.NewChannelRegistry()
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	// Set up a backlog store (needed for the orchestrator constructor, not used heavily).
	blMgr := storage.NewBacklogManager(baseDir)
	blAdapter := &backlogStoreAdapter{mgr: blMgr}

	orch := core.NewFeedbackLoopOrchestrator(reg, km, blAdapter, nil)

	inboxDir := filepath.Join(channelDir, "inbox")
	return orch, inboxDir, channelDir
}

// =========================================================================
// Feedback loop integration tests
// =========================================================================

func TestFeedbackLoop_EndToEndWithFileChannel(t *testing.T) {
	orch, inboxDir, _ := setupFeedbackLoop(t)

	// Place a markdown file in the inbox with YAML frontmatter.
	writeInboxFile(t, inboxDir, "item-001.md", `---
id: item-001
from: alice
subject: Database migration notes
date: "2025-01-15"
priority: medium
status: pending
tags:
  - database
  - migration
---

We decided to use PostgreSQL for the new service. The migration from MySQL
should be done in three phases: schema sync, data copy, and cutover. This
approach minimizes downtime and allows rollback at each phase boundary.
`)

	// Run the feedback loop.
	result, err := orch.Run(core.RunOptions{})
	if err != nil {
		t.Fatalf("running feedback loop: %v", err)
	}

	// Verify items were fetched and processed.
	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched, got %d", result.ItemsFetched)
	}
	if result.ItemsProcessed != 1 {
		t.Errorf("expected 1 item processed, got %d", result.ItemsProcessed)
	}
	if result.KnowledgeAdded != 1 {
		t.Errorf("expected 1 knowledge entry added, got %d", result.KnowledgeAdded)
	}

	// Verify the inbox file was marked as processed.
	data, err := os.ReadFile(filepath.Join(inboxDir, "item-001.md"))
	if err != nil {
		t.Fatalf("reading processed inbox file: %v", err)
	}
	if !strings.Contains(string(data), "status: processed") {
		t.Error("expected inbox file to be marked as processed")
	}
}

func TestFeedbackLoop_WithTaskReference(t *testing.T) {
	orch, inboxDir, channelDir := setupFeedbackLoop(t)

	// Place a file that references a task ID.
	writeInboxFile(t, inboxDir, "item-002.md", `---
id: item-002
from: bob
subject: "Update on TASK-00042 authentication work"
date: "2025-01-16"
priority: high
status: pending
tags:
  - auth
---

The JWT token validation is now passing all tests for TASK-00042.
Implemented RS256 signing as discussed. Ready for review.
`)

	result, err := orch.Run(core.RunOptions{})
	if err != nil {
		t.Fatalf("running feedback loop: %v", err)
	}

	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched, got %d", result.ItemsFetched)
	}
	if result.ItemsProcessed != 1 {
		t.Errorf("expected 1 item processed, got %d", result.ItemsProcessed)
	}
	// Task-referenced items should generate an output.
	if result.OutputsDelivered != 1 {
		t.Errorf("expected 1 output delivered, got %d", result.OutputsDelivered)
	}
	if result.KnowledgeAdded != 1 {
		t.Errorf("expected 1 knowledge added, got %d", result.KnowledgeAdded)
	}

	// Verify the output was written to the outbox.
	outboxDir := filepath.Join(channelDir, "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err != nil {
		t.Fatalf("reading outbox: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 outbox file, got %d", len(entries))
	}
	outData, err := os.ReadFile(filepath.Join(outboxDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading outbox file: %v", err)
	}
	if !strings.Contains(string(outData), "TASK-00042") {
		t.Error("expected outbox file to reference TASK-00042")
	}
}

func TestFeedbackLoop_DryRun(t *testing.T) {
	orch, inboxDir, channelDir := setupFeedbackLoop(t)

	// Place an item in the inbox.
	writeInboxFile(t, inboxDir, "item-003.md", `---
id: item-003
from: carol
subject: "TASK-00010 sprint planning notes"
date: "2025-01-17"
priority: medium
status: pending
tags:
  - planning
---

Sprint 5 planning complete. Priorities set for the next two weeks.
Key focus areas include API performance and documentation updates.
`)

	// Run in dry-run mode.
	result, err := orch.Run(core.RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("running feedback loop (dry-run): %v", err)
	}

	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched, got %d", result.ItemsFetched)
	}
	if result.ItemsProcessed != 1 {
		t.Errorf("expected 1 item processed, got %d", result.ItemsProcessed)
	}

	// Dry run should report outputs but not actually write them.
	if result.OutputsDelivered != 1 {
		t.Errorf("expected 1 output delivered (dry-run counted), got %d", result.OutputsDelivered)
	}

	// Verify the inbox file was NOT marked as processed.
	data, err := os.ReadFile(filepath.Join(inboxDir, "item-003.md"))
	if err != nil {
		t.Fatalf("reading inbox file after dry-run: %v", err)
	}
	if strings.Contains(string(data), "status: processed") {
		t.Error("dry-run should not mark inbox file as processed")
	}

	// Verify no outbox file was written.
	outboxDir := filepath.Join(channelDir, "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err != nil {
		t.Fatalf("reading outbox: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run should not write outbox files, got %d", len(entries))
	}
}

func TestFeedbackLoop_ChannelFilter(t *testing.T) {
	baseDir := t.TempDir()

	// Set up knowledge store.
	ksm := storage.NewKnowledgeStoreManager(baseDir)
	_ = ksm.Load()
	ksAdapter := &knowledgeStoreAdapter{mgr: ksm}
	km := core.NewKnowledgeManager(ksAdapter)

	// Create two channel adapters.
	channelDir1 := filepath.Join(baseDir, "channel-alpha")
	adapter1, err := integration.NewFileChannelAdapter(integration.FileChannelConfig{
		Name:    "alpha",
		BaseDir: channelDir1,
	})
	if err != nil {
		t.Fatalf("creating alpha adapter: %v", err)
	}

	channelDir2 := filepath.Join(baseDir, "channel-beta")
	adapter2, err := integration.NewFileChannelAdapter(integration.FileChannelConfig{
		Name:    "beta",
		BaseDir: channelDir2,
	})
	if err != nil {
		t.Fatalf("creating beta adapter: %v", err)
	}

	reg := core.NewChannelRegistry()
	if err := reg.Register(adapter1); err != nil {
		t.Fatalf("registering alpha: %v", err)
	}
	if err := reg.Register(adapter2); err != nil {
		t.Fatalf("registering beta: %v", err)
	}

	blMgr := storage.NewBacklogManager(baseDir)
	blAdapter := &backlogStoreAdapter{mgr: blMgr}

	orch := core.NewFeedbackLoopOrchestrator(reg, km, blAdapter, nil)

	// Write items to both inboxes.
	writeInboxFile(t, filepath.Join(channelDir1, "inbox"), "alpha-item.md", `---
id: alpha-item
subject: Alpha channel item
date: "2025-01-18"
status: pending
tags:
  - alpha
---

Content from the alpha channel with enough text to be considered substantive for processing.
`)

	writeInboxFile(t, filepath.Join(channelDir2, "inbox"), "beta-item.md", `---
id: beta-item
subject: Beta channel item
date: "2025-01-18"
status: pending
tags:
  - beta
---

Content from the beta channel with enough text to be considered substantive for processing.
`)

	// Run with channel filter -- only process alpha.
	result, err := orch.Run(core.RunOptions{ChannelFilter: "alpha"})
	if err != nil {
		t.Fatalf("running feedback loop with filter: %v", err)
	}

	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched (alpha only), got %d", result.ItemsFetched)
	}
}

func TestFeedbackLoop_SkipsNonSubstantiveContent(t *testing.T) {
	orch, inboxDir, _ := setupFeedbackLoop(t)

	// Place a file with minimal content (no tags, short body).
	writeInboxFile(t, inboxDir, "item-short.md", `---
id: item-short
from: dave
subject: Hi
date: "2025-01-19"
status: pending
---

Hello!
`)

	result, err := orch.Run(core.RunOptions{})
	if err != nil {
		t.Fatalf("running feedback loop: %v", err)
	}

	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched, got %d", result.ItemsFetched)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 item skipped, got %d", result.Skipped)
	}
	if result.ItemsProcessed != 0 {
		t.Errorf("expected 0 items processed, got %d", result.ItemsProcessed)
	}
}

func TestFeedbackLoop_EmptyInbox(t *testing.T) {
	orch, _, _ := setupFeedbackLoop(t)

	result, err := orch.Run(core.RunOptions{})
	if err != nil {
		t.Fatalf("running feedback loop: %v", err)
	}

	if result.ItemsFetched != 0 {
		t.Errorf("expected 0 items fetched, got %d", result.ItemsFetched)
	}
	if result.ItemsProcessed != 0 {
		t.Errorf("expected 0 items processed, got %d", result.ItemsProcessed)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestFeedbackLoop_AppWiringCreatesOrchestrator(t *testing.T) {
	app := newTestApp(t)

	if app.FeedbackLoop == nil {
		t.Fatal("expected FeedbackLoop to be wired in App")
	}

	// Run the loop on an empty channel (should succeed with zero items).
	result, err := app.FeedbackLoop.Run(core.RunOptions{})
	if err != nil {
		t.Fatalf("running feedback loop via App wiring: %v", err)
	}

	if result.ItemsFetched != 0 {
		t.Errorf("expected 0 items fetched, got %d", result.ItemsFetched)
	}
}
