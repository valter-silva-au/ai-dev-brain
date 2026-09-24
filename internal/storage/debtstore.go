package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileDebtStore persists architecture-audit / tech-debt items as workspace-level
// YAML at debt/index.yaml. It mirrors the other registries' conventions
// (RWMutex, seeded-empty when missing, shared readFileOrEmpty/writeYAML). Items
// are lightweight, priority-triageable records — not tickets — so an audit can
// enumerate debt without minting worktrees.
type FileDebtStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileDebtStore creates a tech-debt store rooted at basePath.
func NewFileDebtStore(basePath string) *FileDebtStore {
	return &FileDebtStore{path: filepath.Join(basePath, "debt", "index.yaml")}
}

func (s *FileDebtStore) loadUnsafe() (models.DebtIndex, error) {
	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.DebtIndex{}, fmt.Errorf("read debt index: %w", err)
	}
	if len(data) == 0 {
		return models.DebtIndex{}, nil
	}
	var idx models.DebtIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.DebtIndex{}, fmt.Errorf("parse debt index: %w", err)
	}
	return idx, nil
}

// NextID returns the next debt id (DEBT-NNNN) based on the current count.
func (s *FileDebtStore) NextID() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return "", err
	}
	max := 0
	for _, it := range idx.Items {
		var n int
		if _, err := fmt.Sscanf(it.ID, "DEBT-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("DEBT-%04d", max+1), nil
}

// Add appends a debt item to the registry.
func (s *FileDebtStore) Add(item models.DebtItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for _, existing := range idx.Items {
		if existing.ID == item.ID {
			return fmt.Errorf("debt item %q already exists", item.ID)
		}
	}
	idx.Items = append(idx.Items, item)
	return writeYAML(s.path, idx)
}

// List returns every debt item in registry order.
func (s *FileDebtStore) List() ([]models.DebtItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.Items, nil
}

// Update replaces the item with the same id. It errors if none exists.
func (s *FileDebtStore) Update(item models.DebtItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	for i := range idx.Items {
		if idx.Items[i].ID == item.ID {
			idx.Items[i] = item
			return writeYAML(s.path, idx)
		}
	}
	return fmt.Errorf("debt item %q not found", item.ID)
}
