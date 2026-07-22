package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ADRStore is the persistence seam the ADRManager depends on (the file-backed
// storage.FileADRStore satisfies it structurally, so app.go wires it without an
// adapter). The registry is authoritative for metadata/status; Body reads the
// human-authored MADR markdown.
type ADRStore interface {
	NextNumber() (int, error)
	Create(adr models.ADR, body string) error
	// CreateNext allocates the next number and appends the record atomically
	// (one lock), invoking build with the allocated number so number-dependent
	// fields stay consistent. It is the race-free replacement for a NextNumber
	// then Create pair. build MUST return that exact number (a mismatch is
	// rejected) and MUST NOT re-enter the store (it runs under the store's locks;
	// see FileADRStore.CreateNext).
	CreateNext(build func(number int) (models.ADR, string)) (models.ADR, error)
	List() ([]models.ADR, error)
	Get(number int) (models.ADR, bool, error)
	Update(adr models.ADR) error
	Body(adr models.ADR) (string, error)
}

// ADRManager owns architecture decision records (MADR). Per the house convention
// the constructor returns the interface so callers depend on behaviour.
type ADRManager interface {
	// New creates the next-numbered ADR (status "proposed"), scaffolding its MADR
	// markdown body, and returns the created record.
	New(title string, links []models.Link) (models.ADR, error)
	// List returns every ADR ordered by number.
	List() ([]models.ADR, error)
	// Get returns the ADR by number (found=false when absent).
	Get(number int) (models.ADR, bool, error)
	// Show returns the ADR by number together with its markdown body.
	Show(number int) (models.ADR, string, error)
	// SetStatus transitions an ADR to a new (valid) status.
	SetStatus(number int, status models.ADRStatus) (models.ADR, error)
}

type adrManager struct {
	store ADRStore
	now   func() time.Time
}

// ADRManagerOption customises an ADRManager.
type ADRManagerOption func(*adrManager)

// WithADRClock injects the clock (tests pin it for deterministic timestamps).
func WithADRClock(now func() time.Time) ADRManagerOption {
	return func(m *adrManager) {
		if now != nil {
			m.now = now
		}
	}
}

// NewADRManager wires an ADRManager over an ADRStore.
func NewADRManager(store ADRStore, opts ...ADRManagerOption) ADRManager {
	m := &adrManager{store: store, now: func() time.Time { return time.Now().UTC() }}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *adrManager) New(title string, links []models.Link) (models.ADR, error) {
	if strings.TrimSpace(title) == "" {
		return models.ADR{}, fmt.Errorf("adr title is required")
	}
	ts := m.now()
	// Allocate the number and append under one store lock (CreateNext), so two
	// concurrent New calls can never grab the same number. build runs with the
	// allocated number in hand, keeping number-dependent fields (the slug
	// fallback, the MADR heading rendered by renderMADR) consistent with what is
	// stored.
	adr, err := m.store.CreateNext(func(number int) (models.ADR, string) {
		slug := models.Slugify(title)
		if slug == "" {
			slug = fmt.Sprintf("adr-%04d", number)
		}
		adr := models.ADR{
			Number:  number,
			Title:   title,
			Status:  models.ADRProposed,
			Slug:    slug,
			Created: ts,
			Updated: ts,
			Links:   links,
		}
		return adr, renderMADR(adr)
	})
	if err != nil {
		return models.ADR{}, fmt.Errorf("create adr: %w", err)
	}
	return adr, nil
}

func (m *adrManager) List() ([]models.ADR, error) {
	adrs, err := m.store.List()
	if err != nil {
		return nil, err
	}
	sortADRsByNumber(adrs)
	return adrs, nil
}

func (m *adrManager) Get(number int) (models.ADR, bool, error) {
	return m.store.Get(number)
}

func (m *adrManager) Show(number int) (models.ADR, string, error) {
	adr, found, err := m.store.Get(number)
	if err != nil {
		return models.ADR{}, "", err
	}
	if !found {
		return models.ADR{}, "", fmt.Errorf("no adr %04d", number)
	}
	body, err := m.store.Body(adr)
	if err != nil {
		return adr, "", fmt.Errorf("read adr %04d body: %w", number, err)
	}
	return adr, body, nil
}

func (m *adrManager) SetStatus(number int, status models.ADRStatus) (models.ADR, error) {
	if !status.IsValid() {
		return models.ADR{}, fmt.Errorf("invalid adr status %q (want one of %v)", status, models.ValidADRStatuses)
	}
	adr, found, err := m.store.Get(number)
	if err != nil {
		return models.ADR{}, err
	}
	if !found {
		return models.ADR{}, fmt.Errorf("no adr %04d", number)
	}
	adr.Status = status
	adr.Updated = m.now()
	if err := m.store.Update(adr); err != nil {
		return models.ADR{}, err
	}
	return adr, nil
}

// sortADRsByNumber orders ADRs ascending by number (stable, deterministic).
func sortADRsByNumber(adrs []models.ADR) {
	for i := 1; i < len(adrs); i++ {
		for j := i; j > 0 && adrs[j-1].Number > adrs[j].Number; j-- {
			adrs[j-1], adrs[j] = adrs[j], adrs[j-1]
		}
	}
}

// renderMADR produces the initial MADR markdown body for a new ADR. The registry
// is authoritative for status, so the body's Status line is the scaffold value
// at creation (a human then fills in the sections). Kept inline (one template)
// rather than embedded to avoid an extra FS dependency.
func renderMADR(adr models.ADR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %d. %s\n\n", adr.Number, adr.Title)
	fmt.Fprintf(&b, "- Status: %s\n", adr.Status)
	fmt.Fprintf(&b, "- Date: %s\n\n", adr.Created.Format("2006-01-02"))
	b.WriteString("## Context and Problem Statement\n\n")
	b.WriteString("<!-- What is the issue we are seeing that motivates this decision? -->\n\n")
	b.WriteString("## Decision Drivers\n\n")
	b.WriteString("- <!-- a driver, e.g. a force, a concern -->\n\n")
	b.WriteString("## Considered Options\n\n")
	b.WriteString("- <!-- option 1 -->\n- <!-- option 2 -->\n\n")
	b.WriteString("## Decision Outcome\n\n")
	b.WriteString("Chosen option: \"<!-- option -->\", because <!-- justification -->.\n\n")
	b.WriteString("### Consequences\n\n")
	b.WriteString("- Good, because <!-- positive consequence -->\n")
	b.WriteString("- Bad, because <!-- negative consequence -->\n")
	return b.String()
}
