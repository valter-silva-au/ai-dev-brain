package models

import "time"

// BowtieStage is a deal's position in the Bowtie funnel (Winning by Design):
// the pre-sale narrowing (awareness → education → selection) and the post-sale
// widening (onboarding → impact → expansion). Modelling both halves keeps
// expansion revenue in view, not just new logos.
type BowtieStage string

const (
	BowtieAwareness  BowtieStage = "awareness"
	BowtieEducation  BowtieStage = "education"
	BowtieSelection  BowtieStage = "selection"
	BowtieOnboarding BowtieStage = "onboarding"
	BowtieImpact     BowtieStage = "impact"
	BowtieExpansion  BowtieStage = "expansion"
)

// ValidBowtieStages is the ordered, canonical Bowtie funnel.
var ValidBowtieStages = []BowtieStage{
	BowtieAwareness, BowtieEducation, BowtieSelection,
	BowtieOnboarding, BowtieImpact, BowtieExpansion,
}

// IsValid reports whether s is one of the canonical Bowtie stages.
func (s BowtieStage) IsValid() bool {
	for _, v := range ValidBowtieStages {
		if s == v {
			return true
		}
	}
	return false
}

// MEDDPICC holds the eight enterprise-sales qualification dimensions. Each field
// is free-form evidence text; an empty field is an unqualified dimension. The
// qualification score is the count of filled fields (see Deal.Score).
type MEDDPICC struct {
	Metrics          string `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	EconomicBuyer    string `yaml:"economic_buyer,omitempty" json:"economic_buyer,omitempty"`
	DecisionCriteria string `yaml:"decision_criteria,omitempty" json:"decision_criteria,omitempty"`
	DecisionProcess  string `yaml:"decision_process,omitempty" json:"decision_process,omitempty"`
	PaperProcess     string `yaml:"paper_process,omitempty" json:"paper_process,omitempty"`
	IdentifyPain     string `yaml:"identify_pain,omitempty" json:"identify_pain,omitempty"`
	Champion         string `yaml:"champion,omitempty" json:"champion,omitempty"`
	Competition      string `yaml:"competition,omitempty" json:"competition,omitempty"`
}

// Filled returns how many of the eight MEDDPICC dimensions carry evidence.
func (m MEDDPICC) Filled() int {
	n := 0
	for _, v := range []string{
		m.Metrics, m.EconomicBuyer, m.DecisionCriteria, m.DecisionProcess,
		m.PaperProcess, m.IdentifyPain, m.Champion, m.Competition,
	} {
		if v != "" {
			n++
		}
	}
	return n
}

// Deal is one sales opportunity tracked in the CRM (#135 step 18): a Bowtie
// funnel stage plus MEDDPICC qualification. Lightweight registry data
// (crm/index.yaml), not a ticket.
type Deal struct {
	ID       string      `yaml:"id" json:"id"` // DEAL-NNNN
	Name     string      `yaml:"name" json:"name"`
	Stage    BowtieStage `yaml:"stage" json:"stage"`
	MEDDPICC MEDDPICC    `yaml:"meddpicc" json:"meddpicc"`
	Created  time.Time   `yaml:"created" json:"created"`
	Updated  time.Time   `yaml:"updated" json:"updated"`
}

// Score is the deal's MEDDPICC qualification (0–8): filled dimensions.
func (d Deal) Score() int { return d.MEDDPICC.Filled() }

// CRMIndex is the deal registry document (crm/index.yaml).
type CRMIndex struct {
	Deals []Deal `yaml:"deals" json:"deals"`
}
