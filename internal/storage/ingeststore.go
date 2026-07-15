package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// This file backs the staged ingestion pipeline (decision D8) with three
// file-backed stores rooted in the workspace:
//
//   - FileRawStore     raw/ immutable content + raw/manifest.yaml provenance ledger
//   - FileProposalStore ingested/queue.yaml (review queue) + ingested/ledger.yaml (decisions)
//   - FileNodeStore     ingested/nodes.yaml (typed nodes landed from ingestion)
//
// They mirror the FileStageStore / FileRuleStore conventions: an RWMutex, a
// seeded-empty document when the file is missing, and the shared
// readFileOrEmpty/writeYAML helpers (0o755 dirs / 0o644 files).

// ----- raw landing -----

// FileRawStore lands immutable raw artifacts under raw/ and records provenance
// in raw/manifest.yaml. Landing is idempotent: content already seen (by hash)
// or a source position already seen (by source+cursor) is not re-landed.
type FileRawStore struct {
	basePath     string
	manifestPath string
	mu           sync.Mutex
	now          func() time.Time
}

// NewFileRawStore creates a raw store rooted at basePath.
func NewFileRawStore(basePath string) *FileRawStore {
	return &FileRawStore{
		basePath:     basePath,
		manifestPath: filepath.Join(basePath, "raw", "manifest.yaml"),
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *FileRawStore) loadManifestUnsafe() (models.RawManifest, error) {
	data, err := readFileOrEmpty(s.manifestPath)
	if err != nil {
		return models.RawManifest{}, fmt.Errorf("read raw manifest: %w", err)
	}
	if len(data) == 0 {
		return models.RawManifest{}, nil
	}
	var m models.RawManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return models.RawManifest{}, fmt.Errorf("parse raw manifest: %w", err)
	}
	return m, nil
}

// Land writes content immutably under raw/ and records it in the manifest. It
// returns the artifact and landed=true on a fresh landing; on a dedup hit
// (identical content, or the same source+cursor already landed) it returns the
// EXISTING artifact and landed=false without writing anything.
func (s *FileRawStore) Land(source, cursor string, content []byte) (models.RawArtifact, bool, error) {
	if strings.TrimSpace(source) == "" {
		return models.RawArtifact{}, false, fmt.Errorf("raw landing needs a source")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	manifest, err := s.loadManifestUnsafe()
	if err != nil {
		return models.RawArtifact{}, false, err
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	// Dedup: identical content, or the same source position already landed.
	for _, a := range manifest.Artifacts {
		if a.Hash == hash || (cursor != "" && a.Source == source && a.Cursor == cursor) {
			return a, false, nil
		}
	}

	id := hash[:12]
	rel := filepath.ToSlash(filepath.Join("raw", sanitizeSourceSlug(source), id))
	abs := filepath.Join(s.basePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return models.RawArtifact{}, false, fmt.Errorf("create raw dir: %w", err)
	}
	// The content file is addressed by its content hash, so an existing file at
	// abs already holds identical bytes — the stat check just avoids a redundant
	// rewrite. The real guard against a duplicate manifest entry is the mutex
	// (this whole read-modify-write is serialized), not this stat.
	if _, statErr := os.Stat(abs); statErr == nil {
		// Identical content already on disk; skip the write, still record it.
	} else if err := os.WriteFile(abs, content, 0o644); err != nil {
		return models.RawArtifact{}, false, fmt.Errorf("write raw content: %w", err)
	}

	art := models.RawArtifact{
		ID:          id,
		Source:      source,
		Cursor:      cursor,
		Hash:        hash,
		ContentPath: rel,
		Landed:      s.now(),
	}
	manifest.Artifacts = append(manifest.Artifacts, art)
	if err := writeYAML(s.manifestPath, manifest); err != nil {
		return models.RawArtifact{}, false, err
	}
	return art, true, nil
}

// List returns every landed artifact (the provenance ledger).
func (s *FileRawStore) List() ([]models.RawArtifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadManifestUnsafe()
	if err != nil {
		return nil, err
	}
	return m.Artifacts, nil
}

// ----- review queue + decision ledger -----

// FileProposalStore holds the pending review queue (ingested/queue.yaml) and an
// append-only decision ledger (ingested/ledger.yaml) recording accepted/rejected
// proposals with their provenance.
type FileProposalStore struct {
	queuePath  string
	ledgerPath string
	mu         sync.Mutex
}

// NewFileProposalStore creates a proposal store rooted at basePath.
func NewFileProposalStore(basePath string) *FileProposalStore {
	return &FileProposalStore{
		queuePath:  filepath.Join(basePath, "ingested", "queue.yaml"),
		ledgerPath: filepath.Join(basePath, "ingested", "ledger.yaml"),
	}
}

func (s *FileProposalStore) loadQueueUnsafe() (models.ProposalQueue, error) {
	data, err := readFileOrEmpty(s.queuePath)
	if err != nil {
		return models.ProposalQueue{}, fmt.Errorf("read proposal queue: %w", err)
	}
	if len(data) == 0 {
		return models.ProposalQueue{}, nil
	}
	var q models.ProposalQueue
	if err := yaml.Unmarshal(data, &q); err != nil {
		return models.ProposalQueue{}, fmt.Errorf("parse proposal queue: %w", err)
	}
	return q, nil
}

func (s *FileProposalStore) loadLedgerUnsafe() (models.ProposalQueue, error) {
	data, err := readFileOrEmpty(s.ledgerPath)
	if err != nil {
		return models.ProposalQueue{}, fmt.Errorf("read proposal ledger: %w", err)
	}
	if len(data) == 0 {
		return models.ProposalQueue{}, nil
	}
	var l models.ProposalQueue
	if err := yaml.Unmarshal(data, &l); err != nil {
		return models.ProposalQueue{}, fmt.Errorf("parse proposal ledger: %w", err)
	}
	return l, nil
}

// Enqueue appends a pending proposal to the review queue.
func (s *FileProposalStore) Enqueue(p models.EntityProposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.loadQueueUnsafe()
	if err != nil {
		return err
	}
	q.Proposals = append(q.Proposals, p)
	return writeYAML(s.queuePath, q)
}

// Pending returns the queued proposals awaiting a decision.
func (s *FileProposalStore) Pending() ([]models.EntityProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.loadQueueUnsafe()
	if err != nil {
		return nil, err
	}
	return q.Proposals, nil
}

// Take removes the proposal with the given id from the queue and returns it. ok
// is false when no such proposal is queued.
func (s *FileProposalStore) Take(id string) (models.EntityProposal, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.loadQueueUnsafe()
	if err != nil {
		return models.EntityProposal{}, false, err
	}
	kept := make([]models.EntityProposal, 0, len(q.Proposals))
	var found models.EntityProposal
	ok := false
	for _, p := range q.Proposals {
		if !ok && p.ID == id {
			found = p
			ok = true
			continue
		}
		kept = append(kept, p)
	}
	if !ok {
		return models.EntityProposal{}, false, nil
	}
	q.Proposals = kept
	if err := writeYAML(s.queuePath, q); err != nil {
		return models.EntityProposal{}, false, err
	}
	return found, true, nil
}

// Record appends a decided proposal to the audit ledger (provenance for every
// landed/rejected proposal).
func (s *FileProposalStore) Record(p models.EntityProposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLedgerUnsafe()
	if err != nil {
		return err
	}
	l.Proposals = append(l.Proposals, p)
	return writeYAML(s.ledgerPath, l)
}

// Ledger returns the decision ledger (accepted + rejected proposals).
func (s *FileProposalStore) Ledger() ([]models.EntityProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.loadLedgerUnsafe()
	if err != nil {
		return nil, err
	}
	return l.Proposals, nil
}

// ----- ingested node registry -----

// FileNodeStore holds typed nodes landed from ingestion (ingested/nodes.yaml).
// It is a GraphSource contributor: an ingested node's links[] become graph edges
// just like a task's or initiative's. It uses an RWMutex (unlike the other two
// ingestion stores) because List() is read on every graph rebuild — a high
// read:write ratio that benefits from concurrent reads.
type FileNodeStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileNodeStore creates an ingested-node store rooted at basePath.
func NewFileNodeStore(basePath string) *FileNodeStore {
	return &FileNodeStore{path: filepath.Join(basePath, "ingested", "nodes.yaml")}
}

func (s *FileNodeStore) loadUnsafe() (models.NodeIndex, error) {
	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.NodeIndex{}, fmt.Errorf("read node index: %w", err)
	}
	if len(data) == 0 {
		return models.NodeIndex{}, nil
	}
	var idx models.NodeIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.NodeIndex{}, fmt.Errorf("parse node index: %w", err)
	}
	return idx, nil
}

// Put inserts or replaces a node by id (idempotent on the id).
func (s *FileNodeStore) Put(node models.IngestedNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	replaced := false
	for i := range idx.Nodes {
		if idx.Nodes[i].ID == node.ID {
			idx.Nodes[i] = node
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Nodes = append(idx.Nodes, node)
	}
	return writeYAML(s.path, idx)
}

// List returns every ingested node.
func (s *FileNodeStore) List() ([]models.IngestedNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.Nodes, nil
}

// Get returns the ingested node with the given id. found=false (nil error) means
// no such node — the seam AddEdge uses to resolve an edge's from/to against the
// ingested-node registry, so an edge from an ingested node can be landed (#174).
func (s *FileNodeStore) Get(id string) (models.IngestedNode, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return models.IngestedNode{}, false, err
	}
	for _, n := range idx.Nodes {
		if n.ID == id {
			return n, true, nil
		}
	}
	return models.IngestedNode{}, false, nil
}

// sanitizeSourceSlug reduces a connector source (e.g. "slack:C123", "file:a/b")
// to a filesystem-safe single path segment.
func sanitizeSourceSlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "source"
	}
	return slug
}
