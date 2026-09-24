package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// This file defines the adversarial-verdict seam for HYBRID StageGates (decision
// D4). A gate's judgment items degrade to "pending" (never block) until a verdict
// is available; a negative verdict BLOCKS the advance like an unmet deterministic
// item. Core stays ignorant of HOW the verdict is produced — a VerdictSource is a
// local interface (defined where it is consumed) with a fake in tests and a
// file-backed default in production.

// Verdict is the outcome of an adversarial judgment for a gate's judgment item.
type Verdict struct {
	// Pass is true when the evidence survived adversarial scrutiny.
	Pass bool
	// Detail is a short, human-readable note surfaced in the gate report.
	Detail string
}

// VerdictSource supplies the adversarial verdict for a judgment gate item.
// available=false means no verdict could be produced (none recorded yet) — the
// judgment then degrades to "pending" and never blocks (the graceful-degradation
// contract, D4). A returned error is treated the same as unavailable: infra
// failure must never itself fail a gate; only a deliberate negative Verdict does.
type VerdictSource interface {
	Verdict(transition, itemID, evidenceDir string) (v Verdict, available bool, err error)
}

// RecordedVerdictSource reads a verdict RECORDED as a file in the initiative's
// evidence directory: <evidenceDir>/<itemID>.verdict.md, holding the output of the
// devils-advocate agent. It parses the machine-readable `VERDICT: pass|fail` line
// that the agent is contracted to emit. Semantics:
//   - file absent            -> unavailable (judgment degrades to pending)
//   - `VERDICT: pass`        -> Pass=true
//   - `VERDICT: fail`        -> Pass=false (blocks the advance)
//   - present but no single, unambiguous VERDICT line (e.g. the raw
//     "VERDICT: pass | fail" contract text) -> unavailable; a malformed record
//     neither passes nor blocks the gate.
type RecordedVerdictSource struct{}

// NewRecordedVerdictSource returns the file-backed verdict source used in production.
func NewRecordedVerdictSource() *RecordedVerdictSource { return &RecordedVerdictSource{} }

// verdictArtifact is the file name a recorded verdict for itemID lives under.
func verdictArtifact(itemID string) string { return itemID + ".verdict.md" }

// Verdict implements VerdictSource by reading and parsing the recorded verdict file.
func (RecordedVerdictSource) Verdict(transition, itemID, evidenceDir string) (Verdict, bool, error) {
	if evidenceDir == "" {
		return Verdict{}, false, nil
	}
	// Defence-in-depth: keep the verdict path inside the evidence directory (itemID
	// is hardcoded in the bundles today, but guard a future dynamic id).
	root := filepath.Clean(evidenceDir)
	name := verdictArtifact(itemID)
	p := filepath.Clean(filepath.Join(root, name))
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return Verdict{}, false, fmt.Errorf("verdict path %q escapes the evidence directory", name)
	}
	content, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Verdict{}, false, nil // no verdict recorded yet
		}
		return Verdict{}, false, fmt.Errorf("reading recorded verdict %s: %w", name, err)
	}
	pass, ok := parseVerdict(string(content))
	if !ok {
		return Verdict{}, false, nil // present but unparseable -> degrade to pending
	}
	return Verdict{Pass: pass}, true, nil
}

// parseVerdict scans the devils-advocate output for a single, unambiguous
// `VERDICT: pass` or `VERDICT: fail` line (case-insensitive, allowing a leading
// markdown/frontmatter '#' or fence noise). A line carrying both tokens (the raw
// contract text "VERDICT: pass | fail") is ambiguous and ignored.
func parseVerdict(s string) (pass bool, ok bool) {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if !strings.HasPrefix(strings.ToUpper(line), "VERDICT:") {
			continue
		}
		val := strings.ToLower(strings.TrimSpace(line[len("VERDICT:"):]))
		switch val {
		case "pass":
			return true, true
		case "fail":
			return false, true
		}
		// e.g. "pass | fail" — ambiguous; keep scanning for a real verdict line.
	}
	return false, false
}
