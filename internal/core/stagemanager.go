package core

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// StageStore is the persistence seam the StageManager depends on. It is defined here
// (where it is consumed) so core stays ignorant of internal/storage; an adapter in
// internal/app.go bridges it to the concrete file-backed registries. Get* return
// found=false rather than a sentinel error when the entity is absent, matching the
// issuesync.Provider.Get convention.
type StageStore interface {
	CreateOrganization(org models.Organization) error
	GetOrganization(id string) (models.Organization, bool, error)
	ListOrganizations() ([]models.Organization, error)

	CreateInitiative(init models.Initiative) error
	GetInitiative(id string) (models.Initiative, bool, error)
	ListInitiatives() ([]models.Initiative, error)
	UpdateInitiative(init models.Initiative) error
}

// StageManager owns Organization/Initiative registration and the Stage dimension on
// initiatives. Stage is orthogonal to TaskStatus. New capabilities expose an interface
// (per the house convention that constructors return interfaces) so the CLI and future
// callers depend on behaviour, not the concrete type.
type StageManager interface {
	CreateOrganization(name, gitHost string) (models.Organization, error)
	ListOrganizations() ([]models.Organization, error)
	GetOrganization(id string) (models.Organization, error)

	CreateInitiative(name, orgID string) (models.Initiative, error)
	ListInitiatives() ([]models.Initiative, error)
	GetInitiative(id string) (models.Initiative, error)

	// SetStage sets the stage of an initiative. It rejects an unknown stage and a
	// missing initiative, and stamps Updated in UTC.
	SetStage(initiativeID string, stage models.Stage) (models.Initiative, error)

	// AdvanceStage evaluates the StageGate for the initiative's current stage and,
	// on a clean pass, advances it to the next stage and records the gate result
	// durably on the initiative. When required deterministic evidence is unmet it
	// does NOT advance (Advanced=false) and returns the evaluated gate so callers
	// can report exactly which items are missing — UNLESS opts.Override is set,
	// which advances past the blocked gate and records the reason (human-only).
	// A clean pass or an override emits stage.advanced; an override additionally
	// emits stage.override. It errors on genuine failures (unknown initiative, no
	// gate defined for the current stage, or an override without a reason).
	AdvanceStage(initiativeID string, opts AdvanceOptions) (AdvanceResult, error)

	// EvidenceDir returns the directory that holds an initiative's gate-evidence
	// artifacts (<workspace>/initiatives/<id>/evidence). It errors if the
	// initiative is unknown or no evidence root is configured. Callers scaffold
	// validation worksheets into this directory.
	EvidenceDir(initiativeID string) (string, error)
}

// AdvanceOptions modulates AdvanceStage. Override advances PAST a blocked gate
// and is HUMAN-ONLY (decision D5): it requires a non-empty Reason, which is
// logged on the gate and in the stage.override event.
//
// Automated marks the caller as an automation/agent (the scheduler, a rule).
// Per D5 an automation may advance ONLY on a clean pass and NEVER for a
// human-only gate (Launch→Scale) or with an Override — AdvanceStage enforces
// this, so the human-only guarantee is code, not just convention. The CLI leaves
// Automated false (a human at the keyboard).
type AdvanceOptions struct {
	Override  bool
	Reason    string
	Automated bool
}

// AdvanceResult reports the outcome of an AdvanceStage call. Advanced is true
// when the stage moved (a clean gate pass OR an override). Overridden is true
// only when the move was a human override of a blocked gate. When Advanced is
// false the gate was blocked; Gate carries the per-item evaluation so the
// caller can print the missing deterministic items.
type AdvanceResult struct {
	Initiative models.Initiative
	From       models.Stage
	To         models.Stage
	Gate       models.GateState
	Advanced   bool
	Overridden bool
}

type stageManager struct {
	store            StageStore
	now              func() time.Time
	basePath         string
	eventLogger      EventLogger
	governanceLogger EventLogger
	verdicts         VerdictSource
	metrics          MetricSource
}

// StageManagerOption configures optional StageManager capabilities without
// churning the constructor signature (existing callers pass no options).
type StageManagerOption func(*stageManager)

// WithEvidenceRoot sets the workspace root under which per-initiative gate
// evidence lives (<basePath>/initiatives/<id>/evidence/). Required for
// AdvanceStage to find deterministic evidence artifacts; without it every
// deterministic item evaluates as missing.
func WithEvidenceRoot(basePath string) StageManagerOption {
	return func(m *stageManager) { m.basePath = basePath }
}

// WithEventLogger wires the (optional) event logger the StageManager uses to
// emit stage.advanced / stage.override governance events. Nil-safe: without it,
// advances still work but emit no events.
func WithEventLogger(l EventLogger) StageManagerOption {
	return func(m *stageManager) { m.eventLogger = l }
}

// WithGovernanceLogger wires the (optional) governance event logger — a stream
// DISTINCT from the dev-telemetry event log (decision D19). stage.advanced /
// stage.override are written here as an auditable governance record separate
// from the high-volume task/agent telemetry. Nil-safe: without it, governance
// events still go to the dev event logger (WithEventLogger) alone.
func WithGovernanceLogger(l EventLogger) StageManagerOption {
	return func(m *stageManager) { m.governanceLogger = l }
}

// WithVerdictSource wires the (optional) adversarial verdict source used to turn a
// gate's judgment items into a real pass/fail rather than an always-"pending"
// placeholder (hybrid gates, D4). Nil-safe: without it, judgment items degrade to
// pending and never block, exactly as in the deterministic-only Increment 1.
func WithVerdictSource(vs VerdictSource) StageManagerOption {
	return func(m *stageManager) { m.verdicts = vs }
}

// WithMetricSource wires the (optional) metric source a gate's numeric-threshold
// items read (decision D11 — the MVP→Launch Sean-Ellis ≥40% + effort bar). Nil-safe:
// without it, a gateMetric item evaluates as missing and blocks (there is no metric
// to satisfy it), with a detail line pointing at `adb pmf`.
func WithMetricSource(ms MetricSource) StageManagerOption {
	return func(m *stageManager) { m.metrics = ms }
}

// NewStageManager returns a StageManager backed by store.
func NewStageManager(store StageStore, opts ...StageManagerOption) StageManager {
	m := &stageManager{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// evidenceDir returns the directory that holds an initiative's gate-evidence
// artifacts. It is workspace METADATA (a sibling of initiatives/index.yaml),
// NOT part of the tickets/<platform>/<org>/<repo> path layout.
func (m *stageManager) evidenceDir(initiativeID string) string {
	if m.basePath == "" {
		return ""
	}
	return filepath.Join(m.basePath, "initiatives", initiativeID, "evidence")
}

// EvidenceDir returns an existing initiative's evidence directory. It verifies
// the initiative exists (and that an evidence root is configured) so a caller
// scaffolding worksheets fails clearly on a typo rather than creating a stray dir.
func (m *stageManager) EvidenceDir(initiativeID string) (string, error) {
	if m.basePath == "" {
		return "", fmt.Errorf("evidence root not configured (construct the StageManager with WithEvidenceRoot)")
	}
	if _, found, err := m.store.GetInitiative(initiativeID); err != nil {
		return "", err
	} else if !found {
		return "", fmt.Errorf("initiative %q not found", initiativeID)
	}
	return m.evidenceDir(initiativeID), nil
}

// CreateOrganization registers a new organization. The ID is the slug of name; an
// empty or duplicate ID is rejected.
func (m *stageManager) CreateOrganization(name, gitHost string) (models.Organization, error) {
	id := models.Slugify(name)
	if id == "" {
		return models.Organization{}, fmt.Errorf("organization name %q produces an empty id", name)
	}
	org := models.Organization{
		ID:      id,
		Name:    name,
		GitHost: gitHost,
		Created: m.now(),
	}
	if err := m.store.CreateOrganization(org); err != nil {
		return models.Organization{}, err
	}
	return org, nil
}

func (m *stageManager) ListOrganizations() ([]models.Organization, error) {
	return m.store.ListOrganizations()
}

func (m *stageManager) GetOrganization(id string) (models.Organization, error) {
	org, found, err := m.store.GetOrganization(id)
	if err != nil {
		return models.Organization{}, err
	}
	if !found {
		return models.Organization{}, fmt.Errorf("organization %q not found", id)
	}
	return org, nil
}

// CreateInitiative registers a new initiative under an existing organization. It
// defaults the stage to Idea. The ID is the slug of name; an empty or duplicate ID,
// or an unknown org, is rejected.
func (m *stageManager) CreateInitiative(name, orgID string) (models.Initiative, error) {
	id := models.Slugify(name)
	if id == "" {
		return models.Initiative{}, fmt.Errorf("initiative name %q produces an empty id", name)
	}
	if _, found, err := m.store.GetOrganization(orgID); err != nil {
		return models.Initiative{}, err
	} else if !found {
		return models.Initiative{}, fmt.Errorf("organization %q not found", orgID)
	}
	now := m.now()
	init := models.Initiative{
		ID:      id,
		Name:    name,
		OrgID:   orgID,
		Stage:   models.StageIdea,
		Created: now,
		Updated: now,
	}
	if err := m.store.CreateInitiative(init); err != nil {
		return models.Initiative{}, err
	}
	return init, nil
}

func (m *stageManager) ListInitiatives() ([]models.Initiative, error) {
	return m.store.ListInitiatives()
}

func (m *stageManager) GetInitiative(id string) (models.Initiative, error) {
	init, found, err := m.store.GetInitiative(id)
	if err != nil {
		return models.Initiative{}, err
	}
	if !found {
		return models.Initiative{}, fmt.Errorf("initiative %q not found", id)
	}
	return init, nil
}

func (m *stageManager) SetStage(initiativeID string, stage models.Stage) (models.Initiative, error) {
	if !stage.IsValid() {
		return models.Initiative{}, fmt.Errorf("invalid stage %q (want one of Idea, MVP, Launch, Scale)", stage)
	}
	init, found, err := m.store.GetInitiative(initiativeID)
	if err != nil {
		return models.Initiative{}, err
	}
	if !found {
		return models.Initiative{}, fmt.Errorf("initiative %q not found", initiativeID)
	}
	init.Stage = stage
	init.Updated = m.now()
	if err := m.store.UpdateInitiative(init); err != nil {
		return models.Initiative{}, err
	}
	return init, nil
}

// AdvanceStage evaluates the StageGate for the initiative's current stage and
// advances it on a clean pass. See the interface doc for the contract.
func (m *stageManager) AdvanceStage(initiativeID string, opts AdvanceOptions) (AdvanceResult, error) {
	if m.basePath == "" {
		return AdvanceResult{}, fmt.Errorf("evidence root not configured (construct the StageManager with WithEvidenceRoot)")
	}
	// Override is human-only and must be reason-logged (decision D5). Reject an
	// override with no meaningful reason before touching any state.
	if opts.Override && strings.TrimSpace(opts.Reason) == "" {
		return AdvanceResult{}, fmt.Errorf("override requires a non-empty reason")
	}
	// Overrides are human-only (D5): an automation may advance only on a clean pass.
	if opts.Automated && opts.Override {
		return AdvanceResult{}, fmt.Errorf("override is human-only (decision D5); an automation may advance only on a clean pass")
	}

	init, found, err := m.store.GetInitiative(initiativeID)
	if err != nil {
		return AdvanceResult{}, err
	}
	if !found {
		return AdvanceResult{}, fmt.Errorf("initiative %q not found", initiativeID)
	}

	bundle, ok := stageGates[init.Stage]
	if !ok {
		return AdvanceResult{}, fmt.Errorf("no stage gate defined for advancing from %q (gates exist for Idea, MVP, and Launch)", init.Stage)
	}

	// Human-only gates (Launch→Scale, D5) can never be advanced by an automation,
	// even on a clean pass — the decision needs a human in the loop.
	if opts.Automated && bundle.HumanOnly {
		return AdvanceResult{}, fmt.Errorf("advancing %s is human-only (decision D5); an automation cannot advance to %s", bundle.transition(), bundle.To)
	}

	gate := evaluateGate(bundle, gateEval{
		evidenceDir:  m.evidenceDir(initiativeID),
		initiativeID: initiativeID,
		verdicts:     m.verdicts,
		metrics:      m.metrics,
	})

	result := AdvanceResult{
		Initiative: init,
		From:       bundle.From,
		To:         bundle.To,
		Gate:       gate,
	}

	// Blocked with no override: do not mutate the initiative. The caller reports
	// the missing deterministic items and exits non-zero.
	if !gate.Passed && !opts.Override {
		return result, nil
	}

	// Advancing — either a clean pass or a human override of a blocked gate. The
	// Evaluated timestamp is stamped here (not on the blocked-return path) since
	// we only persist the gate when it actually decides an advance.
	overridden := opts.Override && !gate.Passed
	gate.Evaluated = m.now()
	if overridden {
		gate.Overridden = true
		gate.Reason = opts.Reason
	}
	init.Stage = bundle.To
	init.Gate = &gate
	init.Updated = m.now()
	if err := m.store.UpdateInitiative(init); err != nil {
		return AdvanceResult{}, err
	}
	result.Initiative = init
	result.Gate = gate
	result.Advanced = true
	result.Overridden = overridden

	m.emitAdvanceEvents(init, bundle, overridden, opts.Reason)
	return result, nil
}

// emitAdvanceEvents emits the governance events for a successful advance:
// stage.advanced always (with an `overridden` flag), plus stage.override when
// the advance was a human override (carrying the reason). They go to BOTH the
// dev-telemetry event logger (WithEventLogger — preserves the KnownEventTypes
// contract for metrics/dashboards) AND the distinct governance stream
// (WithGovernanceLogger, decision D19) so a compliance/audit reader sees
// governance decisions without the high-volume task/agent telemetry. Emitted via
// raw type strings so core stays free of the observability package (L400 §5);
// each sink is nil-safe.
func (m *stageManager) emitAdvanceEvents(init models.Initiative, bundle gateBundle, overridden bool, reason string) {
	advanced := map[string]interface{}{
		"initiative_id": init.ID,
		"from":          string(bundle.From),
		"to":            string(bundle.To),
		"overridden":    overridden,
	}
	var override map[string]interface{}
	if overridden {
		override = map[string]interface{}{
			"initiative_id": init.ID,
			"from":          string(bundle.From),
			"to":            string(bundle.To),
			"reason":        reason,
		}
	}
	for _, logger := range []EventLogger{m.eventLogger, m.governanceLogger} {
		if logger == nil {
			continue
		}
		logger.Log("stage.advanced", advanced)
		if override != nil {
			logger.Log("stage.override", override)
		}
	}
}
