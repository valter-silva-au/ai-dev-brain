package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileADRStore persists architecture decision records. The registry
// (adr/index.yaml) is the authoritative index (number/title/status/links); the
// human-authored MADR body lives at docs/adr/NNNN-<slug>.md. It mirrors the
// other registries' conventions (RWMutex, seeded-empty when missing, the shared
// readFileOrEmpty/writeYAML helpers). ADRs participate in the graph via app.go's
// graph source, which turns each into an adr:NNNN node carrying its links.
type FileADRStore struct {
	indexPath string
	docsDir   string
	mu        sync.RWMutex
}

// NewFileADRStore creates an ADR store rooted at basePath.
func NewFileADRStore(basePath string) *FileADRStore {
	return &FileADRStore{
		indexPath: filepath.Join(basePath, "adr", "index.yaml"),
		docsDir:   filepath.Join(basePath, "docs", "adr"),
	}
}

func (s *FileADRStore) loadUnsafe() (models.ADRIndex, error) {
	data, err := readFileOrEmpty(s.indexPath)
	if err != nil {
		return models.ADRIndex{}, fmt.Errorf("read adr index: %w", err)
	}
	if len(data) == 0 {
		return models.ADRIndex{}, nil
	}
	var idx models.ADRIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.ADRIndex{}, fmt.Errorf("parse adr index: %w", err)
	}
	return idx, nil
}

// NextNumber returns the next ADR number (max existing + 1, or 1 when empty).
func (s *FileADRStore) NextNumber() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return 0, err
	}
	max := 0
	for _, a := range idx.ADRs {
		if a.Number > max {
			max = a.Number
		}
	}
	return max + 1, nil
}

// CreateNext allocates the next ADR number and appends the record under a SINGLE
// lock acquisition, closing the read-allocate-write race that separate
// NextNumber + Create calls leave open (two processes could otherwise read the
// same max and allocate a duplicate number, so one Create then fails). build is
// invoked with the allocated number while the lock is held, so the caller can
// render number-dependent fields (the slug fallback, the MADR heading). The
// markdown body is written first (same all-or-nothing reasoning as Create). It
// returns the stored record.
//
// CALLBACK CONTRACT: build MUST return an ADR whose Number is the number it was
// given. A callback returning a different number is a programming error and is
// REJECTED (rather than silently overwritten), because the number is baked into
// the body/slug/filename build produced — forcing a mismatching number would
// persist markdown inconsistent with the index.
//
// NON-REENTRANT: build runs while BOTH this store's in-process mutex AND the
// cross-process file lock are held. It MUST NOT call back into any FileADRStore
// method (Create, CreateNext, List, Get, Update, NextNumber) — the mutex is not
// reentrant, so doing so self-deadlocks. Keep build to pure, number-dependent
// rendering.
func (s *FileADRStore) CreateNext(build func(number int) (models.ADR, string)) (models.ADR, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := acquireRegistryLock(s.indexPath)
	if err != nil {
		return models.ADR{}, fmt.Errorf("lock adr registry: %w", err)
	}
	defer unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return models.ADR{}, err
	}
	max := 0
	for _, a := range idx.ADRs {
		if a.Number > max {
			max = a.Number
		}
	}
	number := max + 1
	adr, body := build(number)
	if adr.Number != number {
		return models.ADR{}, fmt.Errorf("adr build callback returned number %d, want the allocated %d (the callback must use the number it was given)", adr.Number, number)
	}

	if err := os.MkdirAll(s.docsDir, 0o755); err != nil {
		return models.ADR{}, fmt.Errorf("create adr docs dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.docsDir, adr.Filename()), []byte(body), 0o644); err != nil {
		return models.ADR{}, fmt.Errorf("write adr markdown: %w", err)
	}
	idx.ADRs = append(idx.ADRs, adr)
	if err := writeYAML(s.indexPath, idx); err != nil {
		return models.ADR{}, err
	}
	return adr, nil
}

// Create appends adr to the registry and writes its MADR body to
// docs/adr/NNNN-<slug>.md. It errors if an ADR with the same number exists.
// Callers that allocate the number from NextNumber should prefer CreateNext,
// which makes allocate-and-append atomic across processes; Create remains for
// callers supplying an explicit number.
func (s *FileADRStore) Create(adr models.ADR, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := acquireRegistryLock(s.indexPath)
	if err != nil {
		return err
	}
	defer unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for _, existing := range idx.ADRs {
		if existing.Number == adr.Number {
			return fmt.Errorf("adr %04d already exists", adr.Number)
		}
	}
	// Write the markdown body FIRST: a failure here (read-only dir, quota, a bad
	// filename) then leaves the registry untouched, so Create is all-or-nothing
	// as far as the authoritative index is concerned. If the markdown succeeds
	// but the registry write fails, the orphaned file is harmless (not indexed).
	if err := os.MkdirAll(s.docsDir, 0o755); err != nil {
		return fmt.Errorf("create adr docs dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.docsDir, adr.Filename()), []byte(body), 0o644); err != nil {
		return fmt.Errorf("write adr markdown: %w", err)
	}
	idx.ADRs = append(idx.ADRs, adr)
	return writeYAML(s.indexPath, idx)
}

// List returns every ADR in registry order.
func (s *FileADRStore) List() ([]models.ADR, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.ADRs, nil
}

// Get returns the ADR with the given number. found is false (no error) when no
// such ADR exists.
func (s *FileADRStore) Get(number int) (models.ADR, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return models.ADR{}, false, err
	}
	for _, a := range idx.ADRs {
		if a.Number == number {
			return a, true, nil
		}
	}
	return models.ADR{}, false, nil
}

// Update replaces the ADR with the same number in the registry. It errors if no
// ADR with that number exists. The markdown body is not rewritten (the registry
// is authoritative for status/metadata; the body is the human decision text).
func (s *FileADRStore) Update(adr models.ADR) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := acquireRegistryLock(s.indexPath)
	if err != nil {
		return err
	}
	defer unlock()

	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for i := range idx.ADRs {
		if idx.ADRs[i].Number == adr.Number {
			idx.ADRs[i] = adr
			return writeYAML(s.indexPath, idx)
		}
	}
	return fmt.Errorf("adr %04d not found", adr.Number)
}

// Body returns the MADR markdown body for the ADR with the given number.
func (s *FileADRStore) Body(adr models.ADR) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.docsDir, adr.Filename()))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
