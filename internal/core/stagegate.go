package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file defines the founder-playbook StageGates DECLARATIVELY. A gate is a
// data-only evidence bundle (gateBundle / gateItem); the evaluation engine
// (evaluateGate) is generic over the item list, so a gate can be added or
// adjusted by editing the data here WITHOUT touching the engine (issue #89 AC).
//
// Two item kinds:
//   - deterministic: an artifact must exist and be non-empty under the
//     initiative's evidence directory. Unmet deterministic items BLOCK the
//     advance and are reported to the user.
//   - judgment: an adversarial verdict that is REPRESENTED but not yet
//     automated. Judgment items are always "pending" and NEVER block, so the
//     gate degrades gracefully until the adversarial agent ships (decision D4).

type gateKind string

const (
	gateDeterministic gateKind = "deterministic"
	gateJudgment      gateKind = "judgment"
	// gateMetric is a deterministic NUMERIC check: a recorded metric node
	// (decision D11) must meet a minimum threshold. Like a deterministic file
	// item, an unmet metric (missing or below threshold) BLOCKS the advance.
	gateMetric gateKind = "metric"
)

// gateItem is one declarative evidence requirement in a stage-gate bundle.
type gateItem struct {
	ID   string
	Desc string
	Kind gateKind
	// Artifact is the path (relative to the initiative's evidence directory) of
	// the file whose presence + non-emptiness satisfies a deterministic item.
	// Ignored for non-deterministic items.
	Artifact string
	// Metric and Threshold apply to a gateMetric item: the initiative's recorded
	// metric named Metric must be >= Threshold. Ignored for other kinds.
	Metric    string
	Threshold float64
}

// MetricSource yields a recorded metric value for a gate's numeric-threshold
// items (decision D11). It is the seam the gate depends on so core stays ignorant
// of storage; an adapter in app.go bridges it to the metric registry. found=false
// (not an error) means the metric has not been recorded yet.
type MetricSource interface {
	Metric(initiative, name string) (value float64, found bool, err error)
}

// gateEval carries the inputs an evaluation needs beyond the bundle itself:
// where the deterministic evidence lives, which initiative is under evaluation
// (for metric lookups), and the optional verdict + metric sources. Bundling them
// keeps evaluateGate's signature stable as new item kinds appear.
type gateEval struct {
	evidenceDir  string
	initiativeID string
	verdicts     VerdictSource
	metrics      MetricSource
}

// gateBundle is the declarative evidence bundle for one stage transition.
type gateBundle struct {
	From  models.Stage
	To    models.Stage
	Items []gateItem
	// HumanOnly marks a transition that an automation may NEVER advance, even on a
	// clean pass (decision D5 — Launch→Scale is human-only). AdvanceStage refuses an
	// automated advance of a human-only gate; humans advance it normally.
	HumanOnly bool
}

func (b gateBundle) transition() string { return string(b.From) + "->" + string(b.To) }

// ideaToMVPBundle is the Idea→MVP (problem–solution fit) evidence bundle. The
// deterministic items are lightweight file checks against the initiative's
// evidence directory; the judgment item stands in for the adversarial
// problem-validation verdict shipped in a later increment.
var ideaToMVPBundle = gateBundle{
	From: models.StageIdea,
	To:   models.StageMVP,
	Items: []gateItem{
		{
			ID:       "problem-statement",
			Desc:     "A written problem statement (who is hurting, and how badly)",
			Kind:     gateDeterministic,
			Artifact: "problem-statement.md",
		},
		{
			ID:       "target-customer",
			Desc:     "A described target customer / segment",
			Kind:     gateDeterministic,
			Artifact: "target-customer.md",
		},
		{
			ID:   "problem-validation",
			Desc: "Adversarial problem-validation verdict (is the problem real and worth solving?)",
			Kind: gateJudgment,
		},
	},
}

// mvpToLaunchBundle is the MVP→Launch (product–market fit) evidence bundle. The
// founder-playbook bar is "Sean Ellis ≥40% + effort test", now enforced as a
// NUMERIC threshold read from metric nodes (decision D11, `adb pmf`) rather than
// mere file presence — this closes the parked #103. The judgment item stands in
// for the adversarial launch-readiness verdict. Editing the thresholds is a data
// edit; the engine is unchanged.
var mvpToLaunchBundle = gateBundle{
	From: models.StageMVP,
	To:   models.StageLaunch,
	Items: []gateItem{
		{
			ID:        "sean-ellis",
			Desc:      "Sean Ellis PMF score ≥ 40% (\"very disappointed\" without the product)",
			Kind:      gateMetric,
			Metric:    "sean-ellis",
			Threshold: 40,
		},
		{
			ID:        "retention",
			Desc:      "Effort/retention test ≥ 40% (users return and invest effort in the product)",
			Kind:      gateMetric,
			Metric:    "retention",
			Threshold: 40,
		},
		{
			ID:   "launch-readiness",
			Desc: "Adversarial launch-readiness verdict (is the product–market fit real, not a false positive?)",
			Kind: gateJudgment,
		},
	},
}

// launchToScaleBundle is the Launch→Scale evidence bundle — the last founder-
// playbook transition. The scale-threshold bars are NUMERIC metric nodes (D11,
// `adb pmf`): net-revenue-retention ≥ 100% (expansion offsets churn — the core
// scaling signal) and a growth-rate floor. The judgment item stands in for the
// adversarial scale-readiness verdict. It is HUMAN-ONLY (decision D5): an
// automation may never advance to Scale, only a human. Editing the thresholds is
// a data edit; the engine is unchanged.
var launchToScaleBundle = gateBundle{
	From:      models.StageLaunch,
	To:        models.StageScale,
	HumanOnly: true,
	Items: []gateItem{
		{
			ID:        "net-revenue-retention",
			Desc:      "Net revenue retention ≥ 100% (expansion offsets churn)",
			Kind:      gateMetric,
			Metric:    "nrr",
			Threshold: 100,
		},
		{
			ID:        "growth-rate",
			Desc:      "Sustained growth-rate ≥ 15% (the business is compounding, not plateaued)",
			Kind:      gateMetric,
			Metric:    "growth",
			Threshold: 15,
		},
		{
			ID:   "scale-readiness",
			Desc: "Adversarial scale-readiness verdict (are the unit economics + ops ready to pour fuel on?)",
			Kind: gateJudgment,
		},
	},
}

// stageGates is the declarative registry of every defined gate, keyed by the
// stage being advanced FROM. Idea→MVP, MVP→Launch, and Launch→Scale are
// implemented; advancing from Scale (the terminal stage) has no entry and is
// refused by AdvanceStage.
var stageGates = map[models.Stage]gateBundle{
	models.StageIdea:   ideaToMVPBundle,
	models.StageMVP:    mvpToLaunchBundle,
	models.StageLaunch: launchToScaleBundle,
}

// DeterministicGateArtifacts returns the evidence-file names a gate requires as
// deterministic items when advancing FROM the given stage (nil if no gate is
// defined for that stage). It reads the same declarative bundle the evaluator
// uses, so callers — and the drift-guard test that asserts the validation pack
// can satisfy the Idea→MVP gate — never hard-code a list that could drift from
// the gate definition. Metric and judgment items are excluded (they aren't files
// scaffolded into the evidence dir).
func DeterministicGateArtifacts(from models.Stage) []string {
	bundle, ok := stageGates[from]
	if !ok {
		return nil
	}
	var artifacts []string
	for _, it := range bundle.Items {
		if it.Kind == gateDeterministic && it.Artifact != "" {
			artifacts = append(artifacts, it.Artifact)
		}
	}
	return artifacts
}

// evaluateGate runs a bundle against the on-disk evidence directory and returns
// the per-item statuses. Passed is true iff every DETERMINISTIC item is met AND
// no judgment item has a FAILED adversarial verdict. Judgment items consult vs
// (the adversarial verdict source): a passing verdict is "met", a failing verdict
// is "failed" (blocks), and no/unavailable verdict degrades to "pending" (never
// blocks) — the graceful-degradation contract (D4). A nil vs means no verdict
// source is wired, so every judgment item degrades to pending.
func evaluateGate(bundle gateBundle, ev gateEval) models.GateState {
	items := make([]models.GateItemState, 0, len(bundle.Items))
	passed := true
	for _, it := range bundle.Items {
		st := models.GateItemState{ID: it.ID, Desc: it.Desc, Kind: string(it.Kind)}
		switch it.Kind {
		case gateJudgment:
			st.Status, st.Detail = judgmentStatus(ev.verdicts, bundle.transition(), it.ID, ev.evidenceDir)
			if st.Status == models.GateItemFailed {
				passed = false
			}
		case gateMetric:
			st.Status, st.Detail = metricStatus(ev.metrics, ev.initiativeID, it.Metric, it.Threshold)
			if st.Status != models.GateItemMet {
				passed = false
			}
		default: // deterministic
			ok, detail := artifactSatisfied(ev.evidenceDir, it.Artifact)
			st.Detail = detail
			if ok {
				st.Status = models.GateItemMet
			} else {
				st.Status = models.GateItemMissing
				passed = false
			}
		}
		items = append(items, st)
	}
	return models.GateState{
		Transition: bundle.transition(),
		Passed:     passed,
		Items:      items,
	}
}

// metricStatus resolves a gateMetric item against the metric source: a recorded
// value >= threshold is "met"; a missing metric, an unavailable source, or a
// below-threshold value is "missing" (which BLOCKS, like an unmet deterministic
// item). The detail line names the metric and the gap so `adb stage advance` can
// tell the user exactly what to record.
func metricStatus(ms MetricSource, initiativeID, name string, threshold float64) (models.GateItemStatus, string) {
	if ms == nil {
		return models.GateItemMissing, fmt.Sprintf("no metric source wired; record %q with `adb pmf`", name)
	}
	v, found, err := ms.Metric(initiativeID, name)
	switch {
	case err != nil:
		return models.GateItemMissing, fmt.Sprintf("metric %q unavailable: %v", name, err)
	case !found:
		return models.GateItemMissing, fmt.Sprintf("no %q metric recorded (need ≥ %g) — record it with `adb pmf record`", name, threshold)
	case v < threshold:
		return models.GateItemMissing, fmt.Sprintf("%q is %g, below the ≥ %g threshold", name, v, threshold)
	default:
		return models.GateItemMet, fmt.Sprintf("%q is %g (≥ %g)", name, v, threshold)
	}
}

// judgmentStatus resolves a judgment item's status from the verdict source. A nil
// source, an unavailable verdict, or a source error all degrade to "pending"
// (never blocking); only a deliberate negative verdict yields "failed".
func judgmentStatus(vs VerdictSource, transition, itemID, evidenceDir string) (models.GateItemStatus, string) {
	if vs == nil {
		return models.GateItemPending, "awaiting adversarial verdict (no verdict source wired)"
	}
	v, available, err := vs.Verdict(transition, itemID, evidenceDir)
	switch {
	case err != nil:
		return models.GateItemPending, fmt.Sprintf("verdict unavailable: %v", err)
	case !available:
		return models.GateItemPending, "awaiting adversarial verdict (none recorded)"
	case v.Pass:
		return models.GateItemMet, verdictDetail("passed", v.Detail)
	default:
		return models.GateItemFailed, verdictDetail("failed", v.Detail)
	}
}

// verdictDetail formats a judgment item's detail line from the verdict outcome.
func verdictDetail(outcome, detail string) string {
	if strings.TrimSpace(detail) == "" {
		return "adversarial verdict: " + outcome
	}
	return "adversarial verdict: " + outcome + " — " + detail
}

// artifactSatisfied reports whether artifact (relative to evidenceDir) exists
// and is a non-empty regular file. The returned detail is a short human
// explanation used in the missing-evidence report.
func artifactSatisfied(evidenceDir, artifact string) (bool, string) {
	if evidenceDir == "" {
		return false, "no evidence directory configured"
	}
	// Defence-in-depth: artifact names are hardcoded in the bundle today, but
	// guard against a future dynamic name escaping the evidence directory via
	// "../". A cleaned path must stay under evidenceDir.
	root := filepath.Clean(evidenceDir)
	p := filepath.Clean(filepath.Join(root, artifact))
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return false, fmt.Sprintf("artifact %q escapes the evidence directory", artifact)
	}
	info, err := os.Stat(p)
	if err != nil {
		return false, fmt.Sprintf("missing artifact %s", artifact)
	}
	if info.IsDir() {
		return false, fmt.Sprintf("%s is a directory, want a file", artifact)
	}
	if info.Size() == 0 {
		return false, fmt.Sprintf("artifact %s is empty", artifact)
	}
	return true, fmt.Sprintf("found %s (%d bytes)", artifact, info.Size())
}
