package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// CRMStore is the persistence seam for sales deals (storage.FileCRMStore
// satisfies it structurally).
type CRMStore interface {
	NextID() (string, error)
	Add(deal models.Deal) error
	List() ([]models.Deal, error)
	Get(id string) (models.Deal, bool, error)
	Update(deal models.Deal) error
}

// CRMManager owns the MEDDPICC/Bowtie deal registry (#135 step 18).
type CRMManager interface {
	// Add records a new deal at the given Bowtie stage (default awareness) with
	// optional initial MEDDPICC evidence.
	Add(name string, stage models.BowtieStage, meddpicc models.MEDDPICC) (models.Deal, error)
	// List returns deals ordered by Bowtie funnel stage, then id.
	List() ([]models.Deal, error)
	// Get returns a deal by id.
	Get(id string) (models.Deal, bool, error)
	// SetStage advances/moves a deal to a new (valid) Bowtie stage.
	SetStage(id string, stage models.BowtieStage) (models.Deal, error)
}

type crmManager struct {
	store CRMStore
	now   func() time.Time
}

// NewCRMManager wires a CRMManager over a CRMStore.
func NewCRMManager(store CRMStore) CRMManager {
	return &crmManager{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (m *crmManager) Add(name string, stage models.BowtieStage, meddpicc models.MEDDPICC) (models.Deal, error) {
	if strings.TrimSpace(name) == "" {
		return models.Deal{}, fmt.Errorf("deal name is required")
	}
	if stage == "" {
		stage = models.BowtieAwareness
	}
	if !stage.IsValid() {
		return models.Deal{}, fmt.Errorf("invalid Bowtie stage %q (want one of %v)", stage, models.ValidBowtieStages)
	}
	id, err := m.store.NextID()
	if err != nil {
		return models.Deal{}, err
	}
	ts := m.now()
	deal := models.Deal{ID: id, Name: name, Stage: stage, MEDDPICC: meddpicc, Created: ts, Updated: ts}
	if err := m.store.Add(deal); err != nil {
		return models.Deal{}, err
	}
	return deal, nil
}

func (m *crmManager) List() ([]models.Deal, error) {
	deals, err := m.store.List()
	if err != nil {
		return nil, err
	}
	// Order by Bowtie funnel position, then id, for a stable pipeline view.
	rank := map[models.BowtieStage]int{}
	for i, s := range models.ValidBowtieStages {
		rank[s] = i
	}
	sort.SliceStable(deals, func(i, j int) bool {
		if rank[deals[i].Stage] != rank[deals[j].Stage] {
			return rank[deals[i].Stage] < rank[deals[j].Stage]
		}
		return deals[i].ID < deals[j].ID
	})
	return deals, nil
}

func (m *crmManager) Get(id string) (models.Deal, bool, error) {
	return m.store.Get(id)
}

func (m *crmManager) SetStage(id string, stage models.BowtieStage) (models.Deal, error) {
	if !stage.IsValid() {
		return models.Deal{}, fmt.Errorf("invalid Bowtie stage %q (want one of %v)", stage, models.ValidBowtieStages)
	}
	deal, found, err := m.store.Get(id)
	if err != nil {
		return models.Deal{}, err
	}
	if !found {
		return models.Deal{}, fmt.Errorf("no deal %q", id)
	}
	deal.Stage = stage
	deal.Updated = m.now()
	if err := m.store.Update(deal); err != nil {
		return models.Deal{}, err
	}
	return deal, nil
}
