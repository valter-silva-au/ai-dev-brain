package models

import (
	"fmt"
	"strings"
	"time"
)

// SLO is a service-level objective / agreement target (#131 step 17). It records
// a measurable reliability commitment — an Objective (target percentage, e.g.
// 99.9) over a Window (e.g. "30d") — so a launched product's reliability bar is
// explicit and auditable. Kept in a workspace registry (slo/index.yaml).
type SLO struct {
	Name        string    `yaml:"name" json:"name"`
	Objective   float64   `yaml:"objective" json:"objective"` // target percentage in (0,100]
	Window      string    `yaml:"window,omitempty" json:"window,omitempty"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	Updated     time.Time `yaml:"updated" json:"updated"`
}

// SLOIndex is the SLO registry document (slo/index.yaml).
type SLOIndex struct {
	SLOs []SLO `yaml:"slos" json:"slos"`
}

// Validate checks an SLO is well-formed: a name and an objective in (0,100].
func (s SLO) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("slo needs a name")
	}
	if s.Objective <= 0 || s.Objective > 100 {
		return fmt.Errorf("slo %q objective %.3f must be in (0,100]", s.Name, s.Objective)
	}
	return nil
}
