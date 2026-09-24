package models

// This file models a document PROGRAM: a phase-ordered pack of artifact
// templates (a PRD, a design doc, a test plan, …) plus the dependency edges
// between them. A program is data — a `program.yaml` manifest at the root of a
// template pack — so a new program is a pack you drop in, not a Go change.
//
// The dependency model is deliberately PRESENCE-BASED: a template's `requires`
// names other TEMPLATE IDS, and a requirement is satisfied when that template's
// declared `output` file exists in the workspace. This mirrors how the
// founder-playbook stage gates treat file evidence (internal/core/stagegate.go)
// rather than introducing a second, competing completion state machine.

// ProgramTrigger says what causes a template to become actionable.
type ProgramTrigger string

const (
	// TriggerPhase is the default: the template is offered as soon as its
	// requirements are satisfied and its phase is in play.
	TriggerPhase ProgramTrigger = "phase"
	// TriggerManual means a human asks for it explicitly; it is never surfaced as
	// "ready" by the program status view.
	TriggerManual ProgramTrigger = "manual"
	// TriggerEvent means an external event (an incident, an audit request) calls
	// for it. Like manual, it is never auto-surfaced as ready.
	TriggerEvent ProgramTrigger = "event"
)

// RequiresMode says how a template's `requires` list is quantified.
type RequiresMode string

const (
	// RequiresAll (the default) demands every listed template be generated.
	RequiresAll RequiresMode = "all"
	// RequiresAny demands at least one listed template be generated — for
	// artifacts that can be informed by any one of several upstream inputs.
	RequiresAny RequiresMode = "any"
)

// ProgramMeta is the manifest's `program:` block — the identity and provenance
// of the pack.
type ProgramMeta struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Version int    `yaml:"version,omitempty" json:"version,omitempty"`
	// Lineage lists the PUBLIC sources the program's structure derives from, as
	// free-form citation strings. It is REQUIRED and must be non-empty: every
	// program in this repo has to be able to say where it came from, which keeps
	// the packs auditable and provably free of non-public provenance.
	Lineage []string `yaml:"lineage" json:"lineage"`
	// Description is an optional one-liner for `adb program list`.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ProgramPhase is one ordered stage of a program. Phase order is manifest order.
type ProgramPhase struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// HumanReview is the mandatory "a human must look at this" contract on every
// template. It is required precisely because these artifacts are generated: the
// note tells the reviewer what to look at, and the risk says what it costs to
// get wrong. A template without it cannot be part of a valid program.
type HumanReview struct {
	Required bool   `yaml:"required" json:"required"`
	Note     string `yaml:"note,omitempty" json:"note,omitempty"`
	Risk     string `yaml:"risk,omitempty" json:"risk,omitempty"`
}

// ProgramTemplate is one artifact template in a program.
type ProgramTemplate struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	// Phase is the id of the ProgramPhase this template belongs to.
	Phase string `yaml:"phase" json:"phase"`
	// Path is the template file's location relative to the pack root
	// (forward-slash, an embed-FS path).
	Path string `yaml:"path" json:"path"`
	// Output is the generated artifact's path relative to the workspace root.
	// Its existence is what makes this template "generated" (presence-based).
	Output string `yaml:"output" json:"output"`
	// Requires lists the TEMPLATE IDS this artifact depends on — never file
	// paths. Resolving ids (not paths) is what lets ValidateProgram catch typos
	// and cycles instead of silently never firing.
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
	// RequiresMode quantifies Requires; empty means RequiresAll.
	RequiresMode RequiresMode `yaml:"requires_mode,omitempty" json:"requires_mode,omitempty"`
	// Reads are glob hints naming context an agent should read before drafting.
	// They are HINTS ONLY and are never gated on: a missing read does not block
	// or delay a template, it just means less context was available.
	Reads []string `yaml:"reads,omitempty" json:"reads,omitempty"`
	// Trigger is what makes this template actionable; empty means TriggerPhase.
	Trigger ProgramTrigger `yaml:"trigger,omitempty" json:"trigger,omitempty"`
	// HumanReview is REQUIRED on every template.
	HumanReview *HumanReview `yaml:"human_review,omitempty" json:"human_review,omitempty"`
}

// EffectiveTrigger returns the template's trigger, defaulting to TriggerPhase.
func (t ProgramTemplate) EffectiveTrigger() ProgramTrigger {
	if t.Trigger == "" {
		return TriggerPhase
	}
	return t.Trigger
}

// EffectiveRequiresMode returns the template's requires mode, defaulting to
// RequiresAll.
func (t ProgramTemplate) EffectiveRequiresMode() RequiresMode {
	if t.RequiresMode == "" {
		return RequiresAll
	}
	return t.RequiresMode
}

// Program is a parsed `program.yaml` manifest.
type Program struct {
	Program   ProgramMeta       `yaml:"program" json:"program"`
	Phases    []ProgramPhase    `yaml:"phases" json:"phases"`
	Templates []ProgramTemplate `yaml:"templates" json:"templates"`
	// Root is the pack root the manifest was loaded from (not persisted).
	Root string `yaml:"-" json:"-"`
}

// Template returns the template with the given id.
func (p *Program) Template(id string) (ProgramTemplate, bool) {
	for _, t := range p.Templates {
		if t.ID == id {
			return t, true
		}
	}
	return ProgramTemplate{}, false
}

// PhaseIndex returns the manifest position of a phase id (-1 if unknown), which
// is the program's phase ORDER.
func (p *Program) PhaseIndex(id string) int {
	for i, ph := range p.Phases {
		if ph.ID == id {
			return i
		}
	}
	return -1
}

// TemplateStatus classifies one template in a workspace.
type TemplateStatus string

const (
	// TemplateGenerated means the template's output file exists.
	TemplateGenerated TemplateStatus = "generated"
	// TemplateReady means it is not generated, is phase-triggered, and its
	// requirements are satisfied — draft it next.
	TemplateReady TemplateStatus = "ready"
	// TemplateBlocked means it is not generated, is phase-triggered, and its
	// requirements are NOT satisfied.
	TemplateBlocked TemplateStatus = "blocked"
	// TemplatePending means it is manual- or event-triggered, so it waits on a
	// human or an external event rather than on the dependency graph.
	TemplatePending TemplateStatus = "pending"
)
