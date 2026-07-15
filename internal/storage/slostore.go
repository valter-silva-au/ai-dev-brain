package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileSLOStore persists service-level objectives as workspace-level YAML at
// slo/index.yaml. One entry per name: Set upserts. It mirrors the other
// registries' conventions (RWMutex, seeded-empty when missing, shared
// readFileOrEmpty/writeYAML).
type FileSLOStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileSLOStore creates an SLO store rooted at basePath.
func NewFileSLOStore(basePath string) *FileSLOStore {
	return &FileSLOStore{path: filepath.Join(basePath, "slo", "index.yaml")}
}

func (s *FileSLOStore) loadUnsafe() (models.SLOIndex, error) {
	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.SLOIndex{}, fmt.Errorf("read slo index: %w", err)
	}
	if len(data) == 0 {
		return models.SLOIndex{}, nil
	}
	var idx models.SLOIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.SLOIndex{}, fmt.Errorf("parse slo index: %w", err)
	}
	return idx, nil
}

// Set inserts or updates the SLO by name.
func (s *FileSLOStore) Set(slo models.SLO) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for i := range idx.SLOs {
		if idx.SLOs[i].Name == slo.Name {
			idx.SLOs[i] = slo
			return writeYAML(s.path, idx)
		}
	}
	idx.SLOs = append(idx.SLOs, slo)
	return writeYAML(s.path, idx)
}

// List returns every SLO in registry order.
func (s *FileSLOStore) List() ([]models.SLO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.SLOs, nil
}
