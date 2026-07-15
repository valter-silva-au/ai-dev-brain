package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileCRMStore persists sales deals as workspace-level YAML at crm/index.yaml.
// It mirrors the other registries' conventions (RWMutex, seeded-empty when
// missing, shared readFileOrEmpty/writeYAML).
type FileCRMStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileCRMStore creates a CRM store rooted at basePath.
func NewFileCRMStore(basePath string) *FileCRMStore {
	return &FileCRMStore{path: filepath.Join(basePath, "crm", "index.yaml")}
}

func (s *FileCRMStore) loadUnsafe() (models.CRMIndex, error) {
	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.CRMIndex{}, fmt.Errorf("read crm index: %w", err)
	}
	if len(data) == 0 {
		return models.CRMIndex{}, nil
	}
	var idx models.CRMIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.CRMIndex{}, fmt.Errorf("parse crm index: %w", err)
	}
	return idx, nil
}

// NextID returns the next deal id (DEAL-NNNN) based on the current max.
func (s *FileCRMStore) NextID() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return "", err
	}
	max := 0
	for _, d := range idx.Deals {
		var n int
		if _, err := fmt.Sscanf(d.ID, "DEAL-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("DEAL-%04d", max+1), nil
}

// Add appends a deal to the registry (errors on duplicate id).
func (s *FileCRMStore) Add(deal models.Deal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for _, existing := range idx.Deals {
		if existing.ID == deal.ID {
			return fmt.Errorf("deal %q already exists", deal.ID)
		}
	}
	idx.Deals = append(idx.Deals, deal)
	return writeYAML(s.path, idx)
}

// List returns every deal in registry order.
func (s *FileCRMStore) List() ([]models.Deal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.Deals, nil
}

// Get returns the deal by id (found=false when absent).
func (s *FileCRMStore) Get(id string) (models.Deal, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return models.Deal{}, false, err
	}
	for _, d := range idx.Deals {
		if d.ID == id {
			return d, true, nil
		}
	}
	return models.Deal{}, false, nil
}

// Update replaces the deal with the same id (errors if none exists).
func (s *FileCRMStore) Update(deal models.Deal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for i := range idx.Deals {
		if idx.Deals[i].ID == deal.ID {
			idx.Deals[i] = deal
			return writeYAML(s.path, idx)
		}
	}
	return fmt.Errorf("deal %q not found", deal.ID)
}
