package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileRuleStore persists the declarative automation rule set (decision D7) as
// workspace-level YAML at basePath/automation/rules.yaml — the SOURCE OF TRUTH
// for automation, authored by a human and read by the rule engine. It mirrors
// the FileStageStore / FileGraphStore conventions: an RWMutex, a seeded-empty
// set when the file is missing, and the shared readFileOrEmpty/writeYAML helpers
// (0o755 dirs / 0o644 files).
type FileRuleStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileRuleStore creates a file-based rule store rooted at basePath.
func NewFileRuleStore(basePath string) *FileRuleStore {
	return &FileRuleStore{
		path: filepath.Join(basePath, "automation", "rules.yaml"),
	}
}

// Load reads the rule set. A missing file yields an empty set (no error) — the
// seeded-empty convention shared with the other registries.
func (s *FileRuleStore) Load() (models.RuleSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.RuleSet{}, fmt.Errorf("read rules: %w", err)
	}
	if len(data) == 0 {
		return models.RuleSet{}, nil
	}
	var set models.RuleSet
	if err := yaml.Unmarshal(data, &set); err != nil {
		return models.RuleSet{}, fmt.Errorf("parse rules: %w", err)
	}
	return set, nil
}

// Save validates the rule set and writes it to automation/rules.yaml. Validation
// happens here (the write surface) so a malformed set never lands on disk, while
// read stays tolerant.
func (s *FileRuleStore) Save(set models.RuleSet) error {
	if err := set.Validate(); err != nil {
		return fmt.Errorf("invalid rule set: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeYAML(s.path, set)
}
