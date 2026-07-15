package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file implements the staged ingestion pipeline engine (decision D8):
//
//	Land raw content (immutable, provenance + dedup) → Submit extraction
//	proposals → confidence-gate: auto-land the certain, queue the fuzzy →
//	Accept/Reject the queued.
//
// Per the house convention core defines the RawStore/ProposalStore/NodeStore
// seams (file-backed impls live in internal/storage); edge proposals land via
// the same EdgeWriter the rule engine uses (#119), and node proposals land into
// the ingested-node registry, which the graph source includes so an ingested
// node participates in the graph like any entity.

// RawStore lands immutable raw artifacts with provenance + dedup.
type RawStore interface {
	Land(source, cursor string, content []byte) (models.RawArtifact, bool, error)
	List() ([]models.RawArtifact, error)
}

// ProposalStore holds the review queue + the decision ledger.
type ProposalStore interface {
	Enqueue(models.EntityProposal) error
	Pending() ([]models.EntityProposal, error)
	Take(id string) (models.EntityProposal, bool, error)
	Record(models.EntityProposal) error
}

// NodeStore holds typed nodes landed from ingestion (a GraphSource contributor).
type NodeStore interface {
	Put(models.IngestedNode) error
	List() ([]models.IngestedNode, error)
}

// SubmitResult reports what a Submit did: which proposals auto-landed (high
// confidence), which were queued for review (fuzzy), and which were high-confidence
// but could not apply and so fell back to the review queue (Requeued) rather than
// abort the batch or be silently dropped (#173).
type SubmitResult struct {
	Landed   []models.EntityProposal
	Queued   []models.EntityProposal
	Requeued []models.EntityProposal
}

// IngestManager runs the staged ingestion pipeline. The constructor returns the
// interface per the house convention.
type IngestManager interface {
	// Land writes raw content immutably with provenance; landed=false on a dedup
	// hit (identical content or the same source+cursor already landed).
	Land(source, cursor string, content []byte) (models.RawArtifact, bool, error)
	// RawArtifacts lists everything landed (the provenance ledger).
	RawArtifacts() ([]models.RawArtifact, error)
	// Submit routes extraction proposals through the confidence gate: those at or
	// above threshold auto-land and are recorded; the rest go to the review queue.
	Submit(proposals []models.EntityProposal, threshold float64) (SubmitResult, error)
	// Pending returns the proposals awaiting a review decision.
	Pending() ([]models.EntityProposal, error)
	// Accept applies a queued proposal and records it. Errors (and re-queues the
	// proposal) if applying fails, so a proposal is never silently lost.
	Accept(id string) (models.EntityProposal, error)
	// Reject drops a queued proposal with a reason, recording the decision.
	Reject(id, reason string) (models.EntityProposal, error)
}

type ingestManager struct {
	raw       RawStore
	proposals ProposalStore
	nodes     NodeStore
	edges     EdgeWriter
	now       func() time.Time
}

// IngestOption customises an IngestManager.
type IngestOption func(*ingestManager)

// WithIngestClock injects the clock used to stamp landings/decisions.
func WithIngestClock(now func() time.Time) IngestOption {
	return func(m *ingestManager) {
		if now != nil {
			m.now = now
		}
	}
}

// NewIngestManager wires the ingestion engine. edges may be nil (edge proposals
// then fail to apply); nodes may be nil (node proposals then fail to apply).
func NewIngestManager(raw RawStore, proposals ProposalStore, nodes NodeStore, edges EdgeWriter, opts ...IngestOption) IngestManager {
	m := &ingestManager{
		raw:       raw,
		proposals: proposals,
		nodes:     nodes,
		edges:     edges,
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *ingestManager) Land(source, cursor string, content []byte) (models.RawArtifact, bool, error) {
	return m.raw.Land(source, cursor, content)
}

func (m *ingestManager) RawArtifacts() ([]models.RawArtifact, error) {
	return m.raw.List()
}

func (m *ingestManager) Submit(proposals []models.EntityProposal, threshold float64) (SubmitResult, error) {
	var res SubmitResult
	// Validate the WHOLE batch up front so a malformed proposal fails cleanly with
	// nothing committed (a validation error is an authoring bug, not a per-item
	// runtime failure that should leave a partial commit).
	prepared := make([]models.EntityProposal, len(proposals))
	for i, p := range proposals {
		if err := p.Validate(); err != nil {
			return res, err
		}
		if p.ID == "" {
			p.ID = proposalID(p)
		}
		p.Created = m.now()
		prepared[i] = p
	}

	for _, p := range prepared {
		if p.Confidence >= threshold {
			p.Status = models.ProposalAccepted
			p.Decided = m.now()
			if err := m.apply(p); err != nil {
				// An auto-land apply failure (e.g. an edge whose from-entity does
				// not exist yet) must NOT abort the batch, drop this proposal, or
				// skip the remaining ones — that left a partial commit that
				// double-recorded on retry (#173). Fall back to the review queue,
				// exactly as Accept() does, and keep processing. A re-queue failure
				// IS fatal (the store is broken).
				p.Status = models.ProposalPending
				if reErr := m.proposals.Enqueue(p); reErr != nil {
					return res, fmt.Errorf("auto-land proposal %q failed (%v) and re-queue also failed: %w", p.ID, err, reErr)
				}
				res.Requeued = append(res.Requeued, p)
				continue
			}
			if err := m.proposals.Record(p); err != nil {
				return res, err
			}
			res.Landed = append(res.Landed, p)
		} else {
			p.Status = models.ProposalPending
			if err := m.proposals.Enqueue(p); err != nil {
				return res, err
			}
			res.Queued = append(res.Queued, p)
		}
	}
	return res, nil
}

func (m *ingestManager) Pending() ([]models.EntityProposal, error) {
	return m.proposals.Pending()
}

func (m *ingestManager) Accept(id string) (models.EntityProposal, error) {
	p, ok, err := m.proposals.Take(id)
	if err != nil {
		return models.EntityProposal{}, err
	}
	if !ok {
		return models.EntityProposal{}, fmt.Errorf("no pending proposal %q", id)
	}
	p.Status = models.ProposalAccepted
	p.Decided = m.now()
	if err := m.apply(p); err != nil {
		// Re-queue so a transient apply failure (e.g. the from-entity does not
		// exist yet) does not silently discard the proposal. The apply error is the
		// root cause callers care about, so it is the wrapped (%w) one; a re-queue
		// failure is added as trailing context.
		p.Status = models.ProposalPending
		if reErr := m.proposals.Enqueue(p); reErr != nil {
			return models.EntityProposal{}, fmt.Errorf("apply proposal %q: %w (re-queue also failed: %v)", id, err, reErr)
		}
		return models.EntityProposal{}, fmt.Errorf("apply proposal %q: %w", id, err)
	}
	if err := m.proposals.Record(p); err != nil {
		return models.EntityProposal{}, err
	}
	return p, nil
}

func (m *ingestManager) Reject(id, reason string) (models.EntityProposal, error) {
	p, ok, err := m.proposals.Take(id)
	if err != nil {
		return models.EntityProposal{}, err
	}
	if !ok {
		return models.EntityProposal{}, fmt.Errorf("no pending proposal %q", id)
	}
	p.Status = models.ProposalRejected
	p.Reason = reason
	p.Decided = m.now()
	if err := m.proposals.Record(p); err != nil {
		return models.EntityProposal{}, err
	}
	return p, nil
}

// apply lands an accepted proposal: an edge onto its source entity's frontmatter
// (via the shared EdgeWriter), or a node into the ingested-node registry. Node
// landings backfill provenance (Source ← RawID) and a created stamp.
func (m *ingestManager) apply(p models.EntityProposal) error {
	switch p.Kind {
	case models.ProposalEdge:
		if m.edges == nil {
			return fmt.Errorf("edge proposal but no edge writer wired")
		}
		// The landed edge is a plain Link{Type,Target} on the source entity's
		// frontmatter (the closed #109 graph vocabulary, which carries no
		// provenance field). Edge provenance back to the raw artifact is preserved
		// in the decision ledger (ingested/ledger.yaml): Record() stores the full
		// proposal — RawID included — for every accepted edge, so the ledger is the
		// system of record for ingested-edge provenance (nodes, by contrast, carry
		// Source on the node itself).
		return m.edges.AddEdge(p.From, *p.Edge)
	case models.ProposalNode:
		if m.nodes == nil {
			return fmt.Errorf("node proposal but no node store wired")
		}
		node := *p.Node
		if node.Source == "" {
			node.Source = p.RawID
		}
		if node.Created.IsZero() {
			node.Created = m.now()
		}
		return m.nodes.Put(node)
	default:
		return fmt.Errorf("unknown proposal kind %q", p.Kind)
	}
}

// proposalID derives a stable id for a proposal that arrives without one, from
// its provenance + payload, so the review queue can reference it deterministically.
func proposalID(p models.EntityProposal) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s", p.RawID, p.Kind, p.From)
	if p.Edge != nil {
		fmt.Fprintf(h, "|%s|%s", p.Edge.Type, p.Edge.Target)
	}
	if p.Node != nil {
		fmt.Fprintf(h, "|%s|%s", p.Node.ID, p.Node.Type)
	}
	return "prop-" + hex.EncodeToString(h.Sum(nil))[:12]
}
