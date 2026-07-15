package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// DebtStore is the persistence seam for tech-debt items (storage.FileDebtStore
// satisfies it structurally).
type DebtStore interface {
	NextID() (string, error)
	Add(item models.DebtItem) error
	List() ([]models.DebtItem, error)
	Update(item models.DebtItem) error
}

// DebtManager owns the architecture-audit / tech-debt registry.
type DebtManager interface {
	// Add records a new open debt item (priority defaults to P2).
	Add(title, area, note string, priority models.Priority) (models.DebtItem, error)
	// List returns items triage-ordered: open before resolved, then by priority
	// (P0→P3), then by id.
	List() ([]models.DebtItem, error)
	// Resolve marks a debt item resolved.
	Resolve(id string) (models.DebtItem, error)
}

type debtManager struct {
	store DebtStore
	now   func() time.Time
}

// DebtManagerOption customises a DebtManager.
type DebtManagerOption func(*debtManager)

// WithDebtClock injects the clock (tests pin it for deterministic timestamps).
func WithDebtClock(now func() time.Time) DebtManagerOption {
	return func(m *debtManager) {
		if now != nil {
			m.now = now
		}
	}
}

// NewDebtManager wires a DebtManager over a DebtStore.
func NewDebtManager(store DebtStore, opts ...DebtManagerOption) DebtManager {
	m := &debtManager{store: store, now: func() time.Time { return time.Now().UTC() }}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *debtManager) Add(title, area, note string, priority models.Priority) (models.DebtItem, error) {
	if strings.TrimSpace(title) == "" {
		return models.DebtItem{}, fmt.Errorf("debt item title is required")
	}
	if priority == "" {
		priority = models.PriorityP2
	} else if !priority.IsValid() {
		// Reject an out-of-set priority (e.g. a lowercase "p0" typo). List()
		// sorts lexically by priority, so an invalid value would sort into an
		// arbitrary position and mis-triage the item — dead last for "p0"
		// since ASCII 'p' > 'P' (#158).
		return models.DebtItem{}, fmt.Errorf("invalid priority: %s (must be P0, P1, P2, or P3)", priority)
	}
	id, err := m.store.NextID()
	if err != nil {
		return models.DebtItem{}, err
	}
	item := models.DebtItem{
		ID:       id,
		Title:    title,
		Priority: priority,
		Status:   models.DebtOpen,
		Area:     area,
		Note:     note,
		Created:  m.now(),
	}
	if err := m.store.Add(item); err != nil {
		return models.DebtItem{}, err
	}
	return item, nil
}

func (m *debtManager) List() ([]models.DebtItem, error) {
	items, err := m.store.List()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		// Open items sort before resolved.
		if (a.Status == models.DebtOpen) != (b.Status == models.DebtOpen) {
			return a.Status == models.DebtOpen
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority // P0 < P1 < P2 < P3 lexically
		}
		return a.ID < b.ID
	})
	return items, nil
}

func (m *debtManager) Resolve(id string) (models.DebtItem, error) {
	items, err := m.store.List()
	if err != nil {
		return models.DebtItem{}, err
	}
	for _, it := range items {
		if it.ID == id {
			if it.Status == models.DebtResolved {
				return it, nil // idempotent
			}
			it.Status = models.DebtResolved
			ts := m.now()
			it.Resolved = &ts
			if err := m.store.Update(it); err != nil {
				return models.DebtItem{}, err
			}
			return it, nil
		}
	}
	return models.DebtItem{}, fmt.Errorf("no debt item %q", id)
}
