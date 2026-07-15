package core

import (
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// SLOStore is the persistence seam for service-level objectives
// (storage.FileSLOStore satisfies it structurally).
type SLOStore interface {
	Set(slo models.SLO) error
	List() ([]models.SLO, error)
}

// SLOManager owns the SLO/SLA registry (#131 step 17).
type SLOManager interface {
	// Set records or updates an SLO target (upsert by name).
	Set(name string, objective float64, window, description string) (models.SLO, error)
	// List returns every recorded SLO.
	List() ([]models.SLO, error)
}

type sloManager struct {
	store SLOStore
	now   func() time.Time
}

// NewSLOManager wires an SLOManager over an SLOStore.
func NewSLOManager(store SLOStore) SLOManager {
	return &sloManager{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (m *sloManager) Set(name string, objective float64, window, description string) (models.SLO, error) {
	slo := models.SLO{
		Name:        name,
		Objective:   objective,
		Window:      window,
		Description: description,
		Updated:     m.now(),
	}
	if err := slo.Validate(); err != nil {
		return models.SLO{}, err
	}
	if err := m.store.Set(slo); err != nil {
		return models.SLO{}, err
	}
	return slo, nil
}

func (m *sloManager) List() ([]models.SLO, error) {
	return m.store.List()
}
