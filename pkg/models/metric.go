package models

import (
	"fmt"
	"strings"
	"time"
)

// Metric is a product/PMF measurement recorded against an initiative (decision
// D11). Metrics are PROVENANCE-CARRYING graph nodes: each is manual-entry-first
// (Source defaults to "manual") with room for connector-fed values later, and it
// participates in the workspace graph (a part_of edge toward its initiative) so
// it is reachable via the #109 graph. Stage gates read metric values for numeric
// thresholds (e.g. the MVP→Launch Sean-Ellis ≥40% bar) regardless of source.
type Metric struct {
	Initiative string    `yaml:"initiative" json:"initiative"`
	Name       string    `yaml:"name" json:"name"`
	Value      float64   `yaml:"value" json:"value"`
	Unit       string    `yaml:"unit,omitempty" json:"unit,omitempty"`
	Source     string    `yaml:"source" json:"source"`
	Recorded   time.Time `yaml:"recorded" json:"recorded"`
	Note       string    `yaml:"note,omitempty" json:"note,omitempty"`
}

// MetricIndex is the metric registry (metrics/index.yaml). One entry per
// (initiative, name) — re-recording updates the current value in place.
type MetricIndex struct {
	Metrics []Metric `yaml:"metrics" json:"metrics"`
}

// GraphID is the metric's node id in the typed graph: metric:<initiative>:<name>.
// Deterministic so the derived index is stable across rebuilds.
func (m Metric) GraphID() string {
	return "metric:" + m.Initiative + ":" + m.Name
}

// Validate checks a metric is well-formed: an initiative and a name are
// required. Value/Unit/Source are free-form (a metric can legitimately be 0).
func (m Metric) Validate() error {
	if strings.TrimSpace(m.Initiative) == "" {
		return fmt.Errorf("metric needs an initiative")
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("metric needs a name")
	}
	return nil
}
