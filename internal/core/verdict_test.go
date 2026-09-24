package core

import (
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeVerdictSource returns a canned verdict and records the args it was called
// with, for unit-testing the hybrid gate wiring without touching disk.
type fakeVerdictSource struct {
	pass      bool
	available bool
	err       error

	gotTransition, gotItemID, gotEvidenceDir string
	calls                                    int
}

func (f *fakeVerdictSource) Verdict(transition, itemID, evidenceDir string) (Verdict, bool, error) {
	f.calls++
	f.gotTransition, f.gotItemID, f.gotEvidenceDir = transition, itemID, evidenceDir
	return Verdict{Pass: f.pass}, f.available, f.err
}

// TestEvaluateGate_HybridVerdict proves a judgment item now reflects the verdict
// source: a passing verdict is "met" (gate passes), a failing verdict is "failed"
// (gate blocks even with all deterministic evidence), and an unavailable verdict
// or a source error degrades to "pending" (never blocks). The source is queried
// with the transition + item id + evidence dir.
func TestEvaluateGate_HybridVerdict(t *testing.T) {
	dir := t.TempDir()
	// All deterministic evidence present, so only the verdict decides Passed.
	writeEvidence(t, dir, "problem-statement.md", "real pain")
	writeEvidence(t, dir, "target-customer.md", "indie founders")

	t.Run("passing verdict -> met, gate passes", func(t *testing.T) {
		vs := &fakeVerdictSource{available: true, pass: true}
		gs := evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir, verdicts: vs})
		if !gs.Passed {
			t.Errorf("gate should pass with a passing verdict; items=%+v", gs.Items)
		}
		if statusOf(gs, "problem-validation") != models.GateItemMet {
			t.Errorf("judgment = %q, want met", statusOf(gs, "problem-validation"))
		}
		if vs.gotTransition != "Idea->MVP" || vs.gotItemID != "problem-validation" || vs.gotEvidenceDir != dir {
			t.Errorf("verdict source queried with (%q,%q,%q)", vs.gotTransition, vs.gotItemID, vs.gotEvidenceDir)
		}
	})

	t.Run("failing verdict -> failed, gate blocks", func(t *testing.T) {
		gs := evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir, verdicts: &fakeVerdictSource{available: true, pass: false}})
		if gs.Passed {
			t.Error("a failing verdict must block the gate even with all deterministic evidence")
		}
		if statusOf(gs, "problem-validation") != models.GateItemFailed {
			t.Errorf("judgment = %q, want failed", statusOf(gs, "problem-validation"))
		}
	})

	t.Run("unavailable verdict -> pending, gate passes", func(t *testing.T) {
		gs := evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir, verdicts: &fakeVerdictSource{available: false}})
		if !gs.Passed {
			t.Error("an unavailable verdict must degrade to pending (non-blocking)")
		}
		if statusOf(gs, "problem-validation") != models.GateItemPending {
			t.Errorf("judgment = %q, want pending", statusOf(gs, "problem-validation"))
		}
	})

	t.Run("source error -> pending, gate passes", func(t *testing.T) {
		gs := evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir, verdicts: &fakeVerdictSource{available: true, err: errNotFound("boom")}})
		if !gs.Passed {
			t.Error("a verdict source error must degrade to pending, never fail the gate")
		}
		if statusOf(gs, "problem-validation") != models.GateItemPending {
			t.Errorf("judgment = %q, want pending", statusOf(gs, "problem-validation"))
		}
	})
}

// TestRecordedVerdictSource covers the file-backed default: absent file, the two
// real verdicts, an ambiguous contract line, and the traversal guard.
func TestRecordedVerdictSource(t *testing.T) {
	dir := t.TempDir()
	src := NewRecordedVerdictSource()
	const item = "problem-validation"

	// No file recorded yet -> unavailable, no error.
	if _, available, err := src.Verdict("Idea->MVP", item, dir); available || err != nil {
		t.Errorf("absent verdict = (available=%v, err=%v), want (false, nil)", available, err)
	}

	// A real "pass" verdict, in a fenced block like the agent emits.
	writeEvidence(t, dir, item+".verdict.md", "```verdict\nVERDICT: pass\nCONFIDENCE: high\n```\n")
	v, available, err := src.Verdict("Idea->MVP", item, dir)
	if err != nil || !available || !v.Pass {
		t.Errorf("pass verdict = (%+v, available=%v, err=%v)", v, available, err)
	}

	// Flip to "fail".
	writeEvidence(t, dir, item+".verdict.md", "VERDICT: fail\nREASONS:\n- no real pain\n")
	v, available, err = src.Verdict("Idea->MVP", item, dir)
	if err != nil || !available || v.Pass {
		t.Errorf("fail verdict = (%+v, available=%v, err=%v)", v, available, err)
	}

	// The raw contract text ("VERDICT: pass | fail") is ambiguous -> unavailable.
	writeEvidence(t, dir, item+".verdict.md", "VERDICT: pass | fail\n")
	if _, available, err := src.Verdict("Idea->MVP", item, dir); available || err != nil {
		t.Errorf("ambiguous verdict = (available=%v, err=%v), want (false, nil)", available, err)
	}

	// Traversal guard: an item id that would escape the evidence dir errors.
	if _, available, err := src.Verdict("Idea->MVP", "../escape", dir); err == nil || available {
		t.Errorf("traversal item id = (available=%v, err=%v), want an error", available, err)
	}

	// No evidence dir configured -> unavailable, no error.
	if _, available, err := src.Verdict("Idea->MVP", item, ""); available || err != nil {
		t.Errorf("empty evidence dir = (available=%v, err=%v), want (false, nil)", available, err)
	}
}

// TestParseVerdict pins the parser's contract directly.
func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in       string
		wantPass bool
		wantOK   bool
	}{
		{"VERDICT: pass", true, true},
		{"VERDICT: fail", false, true},
		{"verdict: PASS", true, true},    // case-insensitive
		{"# VERDICT: fail", false, true}, // leading markdown/frontmatter
		{"prose\nVERDICT: pass\nmore", true, true},
		{"VERDICT: pass | fail", false, false}, // ambiguous contract text
		{"no verdict here", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		gotPass, gotOK := parseVerdict(c.in)
		if gotPass != c.wantPass || gotOK != c.wantOK {
			t.Errorf("parseVerdict(%q) = (%v,%v), want (%v,%v)", c.in, gotPass, gotOK, c.wantPass, c.wantOK)
		}
	}
}

// sanity: the recorded-verdict artifact name is derived from the item id.
func TestVerdictArtifactName(t *testing.T) {
	if got := verdictArtifact("launch-readiness"); got != filepath.Base("launch-readiness.verdict.md") {
		t.Errorf("verdictArtifact = %q", got)
	}
}
