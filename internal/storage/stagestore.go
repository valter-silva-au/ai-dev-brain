package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// OrgIndex is the on-disk registry of organizations (orgs/index.yaml).
type OrgIndex struct {
	Organizations []models.Organization `yaml:"organizations"`
}

// InitiativeIndex is the on-disk registry of initiatives (initiatives/index.yaml).
type InitiativeIndex struct {
	Initiatives []models.Initiative `yaml:"initiatives"`
}

// FileStageStore persists organizations and initiatives as workspace-level YAML
// registries. It stores them as METADATA (orgs/index.yaml, initiatives/index.yaml)
// so the physical tickets/<platform>/<org>/<repo> correlation layout is untouched
// and no migration is required. It mirrors the FileSessionStoreManager conventions:
// an RWMutex for concurrency, load/save "unsafe" helpers, a seeded-empty index when
// the file is missing, and 0o755 dirs / 0o644 files.
type FileStageStore struct {
	orgsPath        string
	initiativesPath string
	mu              sync.RWMutex
}

// NewFileStageStore creates a file-based stage store rooted at basePath. The two
// registries live at basePath/orgs/index.yaml and basePath/initiatives/index.yaml.
func NewFileStageStore(basePath string) *FileStageStore {
	return &FileStageStore{
		orgsPath:        filepath.Join(basePath, "orgs", "index.yaml"),
		initiativesPath: filepath.Join(basePath, "initiatives", "index.yaml"),
	}
}

// ----- organizations -----

func (s *FileStageStore) loadOrgsUnsafe() (*OrgIndex, error) {
	data, err := readFileOrEmpty(s.orgsPath)
	if err != nil {
		return nil, fmt.Errorf("read orgs index: %w", err)
	}
	index := &OrgIndex{Organizations: []models.Organization{}}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, index); err != nil {
			return nil, fmt.Errorf("parse orgs index: %w", err)
		}
	}
	if index.Organizations == nil {
		index.Organizations = []models.Organization{}
	}
	return index, nil
}

func (s *FileStageStore) saveOrgsUnsafe(index *OrgIndex) error {
	return writeYAML(s.orgsPath, index)
}

// CreateOrganization appends org to the registry. It errors if an organization with
// the same ID already exists.
func (s *FileStageStore) CreateOrganization(org models.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadOrgsUnsafe()
	if err != nil {
		return err
	}
	for _, existing := range index.Organizations {
		if existing.ID == org.ID {
			return fmt.Errorf("organization %q already exists", org.ID)
		}
	}
	index.Organizations = append(index.Organizations, org)
	return s.saveOrgsUnsafe(index)
}

// GetOrganization returns the organization with the given ID. found is false when no
// such organization exists.
func (s *FileStageStore) GetOrganization(id string) (models.Organization, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadOrgsUnsafe()
	if err != nil {
		return models.Organization{}, false, err
	}
	for _, org := range index.Organizations {
		if org.ID == id {
			return org, true, nil
		}
	}
	return models.Organization{}, false, nil
}

// ListOrganizations returns all organizations in registry order.
func (s *FileStageStore) ListOrganizations() ([]models.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadOrgsUnsafe()
	if err != nil {
		return nil, err
	}
	return index.Organizations, nil
}

// ----- initiatives -----

func (s *FileStageStore) loadInitiativesUnsafe() (*InitiativeIndex, error) {
	data, err := readFileOrEmpty(s.initiativesPath)
	if err != nil {
		return nil, fmt.Errorf("read initiatives index: %w", err)
	}
	index := &InitiativeIndex{Initiatives: []models.Initiative{}}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, index); err != nil {
			return nil, fmt.Errorf("parse initiatives index: %w", err)
		}
	}
	if index.Initiatives == nil {
		index.Initiatives = []models.Initiative{}
	}
	return index, nil
}

func (s *FileStageStore) saveInitiativesUnsafe(index *InitiativeIndex) error {
	return writeYAML(s.initiativesPath, index)
}

// CreateInitiative appends init to the registry. It errors if an initiative with the
// same ID already exists.
func (s *FileStageStore) CreateInitiative(init models.Initiative) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadInitiativesUnsafe()
	if err != nil {
		return err
	}
	for _, existing := range index.Initiatives {
		if existing.ID == init.ID {
			return fmt.Errorf("initiative %q already exists", init.ID)
		}
	}
	index.Initiatives = append(index.Initiatives, init)
	return s.saveInitiativesUnsafe(index)
}

// GetInitiative returns the initiative with the given ID. found is false when no such
// initiative exists.
func (s *FileStageStore) GetInitiative(id string) (models.Initiative, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadInitiativesUnsafe()
	if err != nil {
		return models.Initiative{}, false, err
	}
	for _, init := range index.Initiatives {
		if init.ID == id {
			return init, true, nil
		}
	}
	return models.Initiative{}, false, nil
}

// ListInitiatives returns all initiatives in registry order.
func (s *FileStageStore) ListInitiatives() ([]models.Initiative, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadInitiativesUnsafe()
	if err != nil {
		return nil, err
	}
	return index.Initiatives, nil
}

// UpdateInitiative replaces the stored initiative with the same ID. It errors if no
// initiative with that ID exists.
func (s *FileStageStore) UpdateInitiative(init models.Initiative) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadInitiativesUnsafe()
	if err != nil {
		return err
	}
	for i, existing := range index.Initiatives {
		if existing.ID == init.ID {
			index.Initiatives[i] = init
			return s.saveInitiativesUnsafe(index)
		}
	}
	return fmt.Errorf("initiative %q not found", init.ID)
}

// ----- shared helpers -----

// readFileOrEmpty returns the file contents, or nil bytes (no error) when the file
// does not exist — the seeded-empty-registry convention shared with the session store.
func readFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// writeYAML marshals v and writes it to path, creating the parent directory (0o755)
// and the file (0o644) per the workspace persistence conventions.
func writeYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create registry directory: %w", err)
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write registry file: %w", err)
	}
	return nil
}
