package foundation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

// TestDoctorInvalidManifestIsNotHealthy is the guard behind the //nolint:nilerr
// in Doctor's manifest branch.
//
// Doctor deliberately returns a nil Go error for a manifest it could not decode
// or validate: the failure is doctor's OUTPUT, carried as an error-severity
// finding with remediation, and collapsing it into a bare error would throw the
// typed report away. That is only defensible for as long as the envelope still
// says something is wrong. A diagnostic that answers "healthy" for a check that
// failed is the one bug it must not have, so the outcome and the recovery flag
// are pinned here rather than left implied by the finding assertions elsewhere in
// this package.
func TestDoctorInvalidManifestIsNotHealthy(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	if err = os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create control directory: %v", err)
	}
	if err = os.WriteFile(
		layout.ManifestPath(),
		[]byte("schema_version: aidb.workspace/v1\nkind: Workspace\nid: \n"),
		0o644,
	); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	service := newTestService(t, nil)
	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor invalid manifest: %v", err)
	}

	if result.Outcome == capability.OutcomeHealthy {
		t.Fatalf(
			"doctor reported %q for an invalid manifest; a failed check must "+
				"never read as healthy",
			result.Outcome,
		)
	}
	if result.Outcome != capability.OutcomeAttention {
		t.Errorf(
			"doctor outcome = %q, want %q",
			result.Outcome,
			capability.OutcomeAttention,
		)
	}
	if !result.Recovery.Required {
		t.Error("doctor did not mark recovery required for an error finding")
	}
	assertFinding(t, result.Data.Findings, "workspace.manifest.invalid", "error")
}
