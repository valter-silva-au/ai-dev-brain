package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileRawStore_LandAndDedup(t *testing.T) {
	dir := t.TempDir()
	s := NewFileRawStore(dir)

	art, landed, err := s.Land("slack:C1", "100", []byte("hello world"))
	if err != nil {
		t.Fatalf("Land error = %v", err)
	}
	if !landed {
		t.Fatal("first landing should be fresh")
	}
	if art.Hash == "" || art.ContentPath == "" {
		t.Fatalf("artifact missing provenance: %+v", art)
	}
	// The immutable content file exists.
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(art.ContentPath))); err != nil {
		t.Fatalf("raw content not written: %v", err)
	}

	// Same content again → dedup on hash (even from a different source position).
	_, landed2, err := s.Land("slack:C1", "999", []byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if landed2 {
		t.Fatal("identical content should dedup on hash")
	}

	// Same source+cursor, different content → dedup on cursor.
	_, landed3, err := s.Land("slack:C1", "100", []byte("different content"))
	if err != nil {
		t.Fatal(err)
	}
	if landed3 {
		t.Fatal("same source+cursor should dedup")
	}

	// New content, new cursor → fresh landing.
	_, landed4, err := s.Land("slack:C1", "101", []byte("brand new"))
	if err != nil {
		t.Fatal(err)
	}
	if !landed4 {
		t.Fatal("new content + new cursor should land")
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("manifest has %d artifacts, want 2", len(list))
	}
}

func TestFileRawStore_EmptyCursorDedupsOnHashOnly(t *testing.T) {
	s := NewFileRawStore(t.TempDir())
	// A cursorless source dedups on content hash only: same source, empty cursor,
	// DIFFERENT content must both land (no cursor to dedup on).
	if _, landed, err := s.Land("file:notes.md", "", []byte("first")); err != nil || !landed {
		t.Fatalf("first cursorless landing: landed %v err %v", landed, err)
	}
	if _, landed, err := s.Land("file:notes.md", "", []byte("second")); err != nil || !landed {
		t.Fatalf("second cursorless landing (different content) should land: landed %v err %v", landed, err)
	}
	// Same content again → dedup on hash.
	if _, landed, err := s.Land("file:notes.md", "", []byte("first")); err != nil || landed {
		t.Fatalf("identical cursorless content should dedup: landed %v err %v", landed, err)
	}
	list, _ := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 landed artifacts, got %d", len(list))
	}
}

func TestFileProposalStore_QueueTakeRecord(t *testing.T) {
	s := NewFileProposalStore(t.TempDir())
	p := models.EntityProposal{ID: "p1", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.4, From: "TASK-1", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}, Status: models.ProposalPending}
	if err := s.Enqueue(p); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "p1" {
		t.Fatalf("pending = %+v", pending)
	}

	// Take a missing id → not found.
	if _, ok, err := s.Take("ghost"); err != nil || ok {
		t.Fatalf("Take(ghost) = ok %v err %v, want false/nil", ok, err)
	}
	// Take the real one → found + removed from queue.
	got, ok, err := s.Take("p1")
	if err != nil || !ok || got.ID != "p1" {
		t.Fatalf("Take(p1) = %+v ok %v err %v", got, ok, err)
	}
	pending, _ = s.Pending()
	if len(pending) != 0 {
		t.Fatalf("queue should be empty after take, got %+v", pending)
	}

	// Record to the ledger.
	got.Status = models.ProposalAccepted
	if err := s.Record(got); err != nil {
		t.Fatal(err)
	}
	ledger, err := s.Ledger()
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 || ledger[0].Status != models.ProposalAccepted {
		t.Fatalf("ledger = %+v", ledger)
	}
}

func TestFileNodeStore_PutIdempotent(t *testing.T) {
	s := NewFileNodeStore(t.TempDir())
	n := models.IngestedNode{ID: "STK-1", Type: "stakeholder", Title: "Acme", Source: "raw-1"}
	if err := s.Put(n); err != nil {
		t.Fatal(err)
	}
	// Put again with a new title → replaced, not duplicated.
	n.Title = "Acme Corp"
	if err := s.Put(n); err != nil {
		t.Fatal(err)
	}
	nodes, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Title != "Acme Corp" {
		t.Fatalf("nodes = %+v, want one with updated title", nodes)
	}
}
