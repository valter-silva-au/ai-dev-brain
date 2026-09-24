package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileGraphStore persists the DERIVED graph index (a rebuildable cache, never a
// source of truth) as workspace-level YAML at basePath/graph/index.yaml. It
// mirrors the FileStageStore conventions: an RWMutex, a seeded-empty index when
// the file is missing, and the shared readFileOrEmpty/writeYAML helpers
// (0o755 dirs / 0o644 files). Deleting graph/index.yaml costs nothing — the
// graph is reconstructable from the entities' authoritative frontmatter.
type FileGraphStore struct {
	indexPath string
	mu        sync.RWMutex
}

// NewFileGraphStore creates a file-based graph-index store rooted at basePath.
func NewFileGraphStore(basePath string) *FileGraphStore {
	return &FileGraphStore{
		indexPath: filepath.Join(basePath, "graph", "index.yaml"),
	}
}

// SaveGraphIndex writes the derived index to graph/index.yaml.
func (s *FileGraphStore) SaveGraphIndex(index models.GraphIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeYAML(s.indexPath, index)
}

// LoadGraphIndex reads the derived index. found is false (with no error) when
// no index has been materialised yet.
func (s *FileGraphStore) LoadGraphIndex() (models.GraphIndex, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := readFileOrEmpty(s.indexPath)
	if err != nil {
		return models.GraphIndex{}, false, fmt.Errorf("read graph index: %w", err)
	}
	if len(data) == 0 {
		return models.GraphIndex{}, false, nil
	}
	var index models.GraphIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return models.GraphIndex{}, false, fmt.Errorf("parse graph index: %w", err)
	}
	return index, true, nil
}
