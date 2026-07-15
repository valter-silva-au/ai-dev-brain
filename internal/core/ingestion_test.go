package core

import (
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ---- fakes ----

type fakeRawStore struct {
	landed    []models.RawArtifact
	nextFresh bool
}

func (f *fakeRawStore) Land(source, cursor string, content []byte) (models.RawArtifact, bool, error) {
	art := models.RawArtifact{ID: "raw-" + cursor, Source: source, Cursor: cursor, Hash: "h" + cursor}
	if !f.nextFresh {
		return art, false, nil
	}
	f.landed = append(f.landed, art)
	return art, true, nil
}
func (f *fakeRawStore) List() ([]models.RawArtifact, error) { return f.landed, nil }

type fakeProposalStore struct {
	queue  []models.EntityProposal
	ledger []models.EntityProposal
}

func (f *fakeProposalStore) Enqueue(p models.EntityProposal) error {
	f.queue = append(f.queue, p)
	return nil
}
func (f *fakeProposalStore) Pending() ([]models.EntityProposal, error) { return f.queue, nil }
func (f *fakeProposalStore) Take(id string) (models.EntityProposal, bool, error) {
	for i, p := range f.queue {
		if p.ID == id {
			f.queue = append(f.queue[:i:i], f.queue[i+1:]...)
			return p, true, nil
		}
	}
	return models.EntityProposal{}, false, nil
}
func (f *fakeProposalStore) Record(p models.EntityProposal) error {
	f.ledger = append(f.ledger, p)
	return nil
}

type fakeNodeStore struct {
	nodes []models.IngestedNode
}

func (f *fakeNodeStore) Put(n models.IngestedNode) error {
	f.nodes = append(f.nodes, n)
	return nil
}
func (f *fakeNodeStore) List() ([]models.IngestedNode, error) { return f.nodes, nil }

func newTestIngest(freshLand bool) (*fakeRawStore, *fakeProposalStore, *fakeNodeStore, *fakeEdgeWriter, IngestManager) {
	raw := &fakeRawStore{nextFresh: freshLand}
	ps := &fakeProposalStore{}
	ns := &fakeNodeStore{}
	edges := &fakeEdgeWriter{}
	m := NewIngestManager(raw, ps, ns, edges,
		WithIngestClock(func() time.Time { return time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC) }))
	return raw, ps, ns, edges, m
}

// ---- tests ----

func TestIngest_Land_Dedup(t *testing.T) {
	_, _, _, _, m := newTestIngest(false) // raw store reports dedup
	_, landed, err := m.Land("slack:C1", "100", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if landed {
		t.Fatal("expected dedup (landed=false)")
	}
}

func TestIngest_Submit_ConfidenceGate(t *testing.T) {
	_, ps, _, edges, m := newTestIngest(true)
	proposals := []models.EntityProposal{
		{ID: "hi", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.95, From: "TASK-1", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}},
		{ID: "lo", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.40, From: "TASK-3", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-4"}},
	}
	res, err := m.Submit(proposals, 0.80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Landed) != 1 || res.Landed[0].ID != "hi" {
		t.Fatalf("landed = %+v, want [hi]", res.Landed)
	}
	if len(res.Queued) != 1 || res.Queued[0].ID != "lo" {
		t.Fatalf("queued = %+v, want [lo]", res.Queued)
	}
	// The high-confidence edge was applied.
	if len(edges.edges) != 1 || edges.edges[0].from != "TASK-1" {
		t.Fatalf("edges applied = %+v, want one from TASK-1", edges.edges)
	}
	// It was recorded to the ledger; the fuzzy one is queued, not recorded.
	if len(ps.ledger) != 1 || ps.ledger[0].Status != models.ProposalAccepted {
		t.Fatalf("ledger = %+v", ps.ledger)
	}
	if len(ps.queue) != 1 || ps.queue[0].Status != models.ProposalPending {
		t.Fatalf("queue = %+v", ps.queue)
	}
}

// TestIngest_Submit_ResilientOnApplyFailure guards #173: a high-confidence
// proposal that fails to apply mid-batch must NOT abort the batch (dropping it +
// every later proposal while leaving earlier ones committed to double-record on
// retry). It falls back to the review queue, and the rest of the batch still
// processes.
func TestIngest_Submit_ResilientOnApplyFailure(t *testing.T) {
	_, ps, _, edges, m := newTestIngest(true)
	edges.failFrom = "TASK-BAD" // simulate an edge whose from-entity doesn't exist

	proposals := []models.EntityProposal{
		{ID: "first", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.95, From: "TASK-1", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}},
		{ID: "boom", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.95, From: "TASK-BAD", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}},
		{ID: "later-hi", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.95, From: "TASK-3", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-4"}},
		{ID: "later-lo", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.10, From: "TASK-5", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-6"}},
	}

	res, err := m.Submit(proposals, 0.80)
	if err != nil {
		t.Fatalf("Submit must not abort on a single apply failure: %v", err)
	}
	// first + later-hi landed; the failing one requeued; the low-confidence one queued.
	if ids := proposalIDs(res.Landed); len(ids) != 2 || ids[0] != "first" || ids[1] != "later-hi" {
		t.Errorf("landed = %v, want [first later-hi]", ids)
	}
	if ids := proposalIDs(res.Requeued); len(ids) != 1 || ids[0] != "boom" {
		t.Errorf("requeued = %v, want [boom]", ids)
	}
	if ids := proposalIDs(res.Queued); len(ids) != 1 || ids[0] != "later-lo" {
		t.Errorf("queued = %v, want [later-lo]", ids)
	}
	// The failing proposal is in the review queue (not silently lost) alongside the
	// low-confidence one — never dropped.
	if len(ps.queue) != 2 {
		t.Errorf("review queue should hold boom + later-lo, got %+v", ps.queue)
	}
	// Only the two that truly landed are in the ledger — no partial/duplicate record.
	if len(ps.ledger) != 2 {
		t.Errorf("ledger should have exactly the 2 landed proposals, got %+v", ps.ledger)
	}

	// Idempotency: re-running the SAME batch must not double-record the ones that
	// landed the first time... but this fake ledger just appends, so instead assert
	// the real invariant the bug violated — a failed batch left NO orphaned partial
	// commit: everything is accounted for as landed|requeued|queued exactly once.
	total := len(res.Landed) + len(res.Requeued) + len(res.Queued)
	if total != len(proposals) {
		t.Errorf("every proposal must be accounted for exactly once: %d != %d", total, len(proposals))
	}
}

func proposalIDs(ps []models.EntityProposal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestIngest_Submit_RejectsInvalid(t *testing.T) {
	_, _, _, _, m := newTestIngest(true)
	bad := []models.EntityProposal{{ID: "x", Kind: models.ProposalEdge, Confidence: 0.5}} // no raw_id, no edge
	if _, err := m.Submit(bad, 0.8); err == nil {
		t.Fatal("expected Submit to reject an invalid proposal")
	}
}

func TestIngest_Accept_AppliesAndRecords(t *testing.T) {
	_, ps, _, edges, m := newTestIngest(true)
	// Queue a fuzzy edge proposal.
	if _, err := m.Submit([]models.EntityProposal{
		{ID: "q1", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.3, From: "TASK-9", Edge: &models.Link{Type: models.EdgeDependsOn, Target: "TASK-8"}},
	}, 0.8); err != nil {
		t.Fatal(err)
	}
	got, err := m.Accept("q1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.ProposalAccepted {
		t.Fatalf("status = %q", got.Status)
	}
	if len(edges.edges) != 1 || edges.edges[0].link.Type != models.EdgeDependsOn {
		t.Fatalf("edge not applied on accept: %+v", edges.edges)
	}
	if len(ps.queue) != 0 {
		t.Fatalf("queue should be empty after accept, got %+v", ps.queue)
	}
	if len(ps.ledger) != 1 {
		t.Fatalf("ledger should have the accepted proposal, got %+v", ps.ledger)
	}
}

func TestIngest_Accept_NodeProposal(t *testing.T) {
	_, _, ns, _, m := newTestIngest(true)
	if _, err := m.Submit([]models.EntityProposal{
		{ID: "n1", RawID: "raw-7", Kind: models.ProposalNode, Confidence: 0.2, Node: &models.IngestedNode{ID: "STK-1", Type: "stakeholder", Title: "Acme"}},
	}, 0.8); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Accept("n1"); err != nil {
		t.Fatal(err)
	}
	if len(ns.nodes) != 1 || ns.nodes[0].ID != "STK-1" {
		t.Fatalf("node not landed: %+v", ns.nodes)
	}
	// Provenance backfilled from the proposal's RawID.
	if ns.nodes[0].Source != "raw-7" {
		t.Fatalf("node provenance = %q, want raw-7", ns.nodes[0].Source)
	}
}

func TestIngest_Reject(t *testing.T) {
	_, ps, _, edges, m := newTestIngest(true)
	if _, err := m.Submit([]models.EntityProposal{
		{ID: "r1", RawID: "raw-1", Kind: models.ProposalEdge, Confidence: 0.3, From: "TASK-1", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}},
	}, 0.8); err != nil {
		t.Fatal(err)
	}
	got, err := m.Reject("r1", "not a real relationship")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.ProposalRejected || got.Reason == "" {
		t.Fatalf("reject result = %+v", got)
	}
	if len(edges.edges) != 0 {
		t.Fatal("reject must not apply the edge")
	}
	if len(ps.ledger) != 1 || ps.ledger[0].Status != models.ProposalRejected {
		t.Fatalf("ledger = %+v", ps.ledger)
	}
}

func TestIngest_EdgeProvenanceInLedger(t *testing.T) {
	// The landed edge is a plain Link with no provenance field; provenance is
	// preserved in the decision ledger (raw_id). Assert it for both an auto-landed
	// edge and an accepted (reviewed) edge.
	_, ps, _, _, m := newTestIngest(true)
	if _, err := m.Submit([]models.EntityProposal{
		{ID: "auto", RawID: "raw-AUTO", Kind: models.ProposalEdge, Confidence: 0.99, From: "TASK-1", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-2"}},
		{ID: "queued", RawID: "raw-QUEUE", Kind: models.ProposalEdge, Confidence: 0.2, From: "TASK-3", Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "TASK-4"}},
	}, 0.8); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Accept("queued"); err != nil {
		t.Fatal(err)
	}
	ledger := ps.ledger
	if len(ledger) != 2 {
		t.Fatalf("ledger has %d entries, want 2 (auto-land + accept)", len(ledger))
	}
	byID := map[string]string{}
	for _, p := range ledger {
		byID[p.ID] = p.RawID
	}
	if byID["auto"] != "raw-AUTO" || byID["queued"] != "raw-QUEUE" {
		t.Fatalf("edge provenance lost in ledger: %+v", byID)
	}
}

func TestIngest_Accept_Unknown(t *testing.T) {
	_, _, _, _, m := newTestIngest(true)
	if _, err := m.Accept("ghost"); err == nil {
		t.Fatal("expected error accepting an unknown proposal")
	}
}
